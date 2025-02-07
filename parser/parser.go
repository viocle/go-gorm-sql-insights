package parser

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	"vitess.io/vitess/go/mysql/config"
	"vitess.io/vitess/go/vt/sqlparser"
)

var (
	ErrSQLStringEmpty = errors.New("SQL string is empty")
)

const (
	parsedFieldsAreaFrom    parsedFieldsArea = "FROM"
	parsedFieldsAreaWhere   parsedFieldsArea = "WHERE"
	parsedFieldsAreaGroupBy parsedFieldsArea = "GROUP BY"
)

type parsedFieldsArea string

type ParsedFields struct {
	// FromFields is a map of all references to table names and their fields from the FROM clause (includes JOINs)
	FromFields map[string][]string

	// WhereFields is a map of all references to table names and their fields from the WHERE clause
	WhereFields map[string][]string

	// GroupByFields is a map of all references to table names and their fields from the GROUP BY clause
	GroupByFields map[string][]string

	// TableFields is a map of all references to table names and their fields
	TableFields map[string][]string

	// AliasMap is a map of table aliases to table names
	AliasMap map[string]string
}

// ParseSQL parses the SQL string and returns a ParsedFields struct
func ParseSQL(sql string) (*ParsedFields, error) {
	if sql == "" {
		return nil, ErrSQLStringEmpty
	}
	// create our new parser and parse it
	sParser, err := sqlparser.New(sqlparser.Options{
		MySQLServerVersion: config.DefaultMySQLVersion,
		TruncateUILen:      512,
		TruncateErrLen:     0,
	})
	if err != nil {
		return nil, err
	}
	stmt, err := sParser.Parse(sql)
	if err != nil {
		return nil, err
	}
	f := &ParsedFields{TableFields: make(map[string][]string, 10), AliasMap: make(map[string]string, 10), FromFields: make(map[string][]string, 10), WhereFields: make(map[string][]string, 10), GroupByFields: make(map[string][]string, 10)}
	processStatement(stmt, f)

	// merge tables where an alias is used back to the actual table name
	f.MergeAliasTables()

	// remove any tables that have no fields
	f.PurgeEmptyTables()

	// return the parsed fields
	return f, nil
}

func processStatement(stmt sqlparser.Statement, f *ParsedFields) {
	switch s := stmt.(type) {
	case *sqlparser.Select:
		// get all fields used in FROM, WHERE, HAVING, GROUP BY used in our SELECT statement
		for _, fromExp := range s.From {
			GetFieldsFromExpression(parsedFieldsAreaFrom, f, fromExp)
		}
		if s.Where != nil {
			GetFieldsFromExpr(parsedFieldsAreaWhere, f, s.Where.Expr, nil)
		}
		if s.Having != nil {
			GetFieldsFromExpr(parsedFieldsAreaWhere, f, s.Having.Expr, nil)
		}
		if s.GroupBy != nil {
			for _, expr := range s.GroupBy.Exprs {
				GetFieldsFromExpr(parsedFieldsAreaGroupBy, f, expr, nil)
			}
		}
	}
}

// MergeAliasTables merges the table names where an alias is used
func (f *ParsedFields) MergeAliasTables() {
	for alias, tableName := range f.AliasMap {
		mergeTableFields(f.FromFields, alias, tableName)
		mergeTableFields(f.WhereFields, alias, tableName)
		mergeTableFields(f.GroupByFields, alias, tableName)
		mergeTableFields(f.TableFields, alias, tableName)
	}
}

// PurgeEmptyTables removes any tables that have no fields
func (f *ParsedFields) PurgeEmptyTables() {
	for tableName, fields := range f.FromFields {
		if len(fields) == 0 {
			delete(f.FromFields, tableName)
		}
	}
	for tableName, fields := range f.WhereFields {
		if len(fields) == 0 {
			delete(f.WhereFields, tableName)
		}
	}
	for tableName, fields := range f.GroupByFields {
		if len(fields) == 0 {
			delete(f.GroupByFields, tableName)
		}
	}
	for tableName, fields := range f.TableFields {
		if len(fields) == 0 {
			delete(f.TableFields, tableName)
		}
	}
}

// mergeTableFields merges fields from an alias key to a table name key
func mergeTableFields(fieldMap map[string][]string, aliasKey, tableKey string) {
	if _, ok := fieldMap[aliasKey]; ok {
		// found an alias, merge into a new/existing key
		if _, ok := fieldMap[tableKey]; !ok {
			// actual table name key does not exist, create the new key
			fieldMap[tableKey] = make([]string, 0, len(fieldMap[aliasKey]))
		}
		// merge fields from alias key to table name key
		for _, fieldName := range fieldMap[aliasKey] {
			if slices.Contains(fieldMap[tableKey], fieldName) {
				// already exists
				continue
			}
			fieldMap[tableKey] = append(fieldMap[tableKey], fieldName)
		}
		// remove the alias key
		delete(fieldMap, aliasKey)
	}
}

// AddTableField adds a field to the list of fields for a table in a specific area
func (f *ParsedFields) AddTableField(area parsedFieldsArea, tableName, fieldName string) {
	// add field to specific area
	switch area {
	case parsedFieldsAreaFrom:
		// field found in FROM clause, includes JOINs
		addTableField(f.FromFields, tableName, fieldName)
	case parsedFieldsAreaWhere:
		// field found in WHERE clause
		addTableField(f.WhereFields, tableName, fieldName)
	case parsedFieldsAreaGroupBy:
		// field found in GROUP BY clause
		addTableField(f.GroupByFields, tableName, fieldName)
	}
	// add field to full list of table fields
	addTableField(f.TableFields, tableName, fieldName)
}

// addTableField adds a field for a table to the provided list of table fields
func addTableField(tableFields map[string][]string, tableName, fieldName string) {
	if tableName == "" || fieldName == "" {
		return
	}
	if _, ok := tableFields[tableName]; !ok {
		tableFields[tableName] = make([]string, 0, 10)
	}
	if slices.Contains(tableFields[tableName], fieldName) {
		return
	}
	tableFields[tableName] = append(tableFields[tableName], fieldName)
}

// AddTable adds a table to the list of tables, with an optional alias
func (f *ParsedFields) AddTable(tableName, as string) {
	if tableName == "dual" {
		fmt.Println("Found dual table")
	}
	if as != "" {
		// add alias
		if _, ok := f.AliasMap[as]; !ok {
			f.AliasMap[as] = tableName
		}
	}
	if _, ok := f.TableFields[tableName]; !ok {
		// add new table to the list of tables
		f.TableFields[tableName] = make([]string, 0, 10)
	}
}

// GetFieldsFromExpression gets fields from the join table expression, processing table name aliases if present
func GetFieldsFromExpression(area parsedFieldsArea, tableFields *ParsedFields, exp sqlparser.TableExpr) {
	switch e := exp.(type) {
	case *sqlparser.JoinTableExpr:
		if e.LeftExpr != nil {
			GetFieldsFromExpression(area, tableFields, e.LeftExpr)
		}
		if e.RightExpr != nil {
			GetFieldsFromExpression(area, tableFields, e.RightExpr)
		}
		if e.Condition != nil {
			if e.Condition.On != nil {
				GetFieldsFromExpr(area, tableFields, e.Condition.On, nil)
			}
		}
	case *sqlparser.AliasedTableExpr:
		GetFieldsFromExpr(area, tableFields, e.Expr, &e.As)
	case *sqlparser.ParenTableExpr:
		for _, expr := range e.Exprs {
			GetFieldsFromExpression(area, tableFields, expr)
		}
	default:
		fmt.Printf("(getFieldsFromExpression) Unknown type: %T\n", e)
	}
}

// GetFieldsFromExpr gets fields from the expression, processing table name aliases if present
func GetFieldsFromExpr(area parsedFieldsArea, tableFields *ParsedFields, e interface{}, as *sqlparser.IdentifierCS) {
	if e == nil {
		return
	}
	switch e := e.(type) {
	case *sqlparser.DerivedTable:
		// derived table, pass back up to process the statement
		if e.Select != nil {
			processStatement(e.Select, tableFields)
		}
	case sqlparser.ColName:
		// column name referenced
		tableFields.AddTableField(area, e.Qualifier.Name.String(), e.Name.String())
	case *sqlparser.ColName:
		// column name referenced
		if e != nil {
			GetFieldsFromExpr(area, tableFields, *e, nil)
		}
	case sqlparser.TableName:
		// we have a table name
		if as != nil {
			// an alias is present
			tableFields.AddTable(e.Name.String(), as.String())
		} else {
			tableFields.AddTable(e.Name.String(), "")
		}
	case *sqlparser.TableName:
		// we have a table name, dereference the pointer and call the function again
		if e != nil {
			GetFieldsFromExpr(area, tableFields, e, nil)
		}
	case *sqlparser.GroupBy:
		for _, expr := range e.Exprs {
			GetFieldsFromExpr(area, tableFields, expr, nil)
		}
	case []*sqlparser.Expr:
		// array of pointers to expressions
		for _, expr := range e {
			if expr != nil {
				GetFieldsFromExpr(area, tableFields, expr, nil)
			}
		}
	case []sqlparser.Expr:
		// array of expressions
		for _, expr := range e {
			GetFieldsFromExpr(area, tableFields, expr, nil)
		}
	case *sqlparser.Literal, sqlparser.Literal, *sqlparser.NullVal, sqlparser.NullVal, *sqlparser.BoolVal, sqlparser.BoolVal, *sqlparser.ListArg, sqlparser.ListArg, *sqlparser.Scope, sqlparser.Scope, *sqlparser.BinaryExprOperator, sqlparser.BinaryExprOperator, *sqlparser.UnaryExprOperator, sqlparser.UnaryExprOperator:
		// explicitly ignore
	default:
		// all other types. Look specifically for Expr types and process
		// get our model value
		valueOf := reflect.ValueOf(e)
		// if the value is a pointer, dereference it
		if valueOf.Kind() == reflect.Ptr {
			valueOf = valueOf.Elem()
		}
		if !valueOf.CanInterface() {
			// non-interfaceable value, skip
			return
		}
		if valueOf.Kind() == reflect.Slice {
			// slice of values, process each one
			for i := 0; i < valueOf.Len(); i++ {
				if valueOfI := valueOf.Index(i).Interface(); valueOfI != nil {
					GetFieldsFromExpr(area, tableFields, valueOfI, nil)
				}
			}
			return
		}
		// check each field in the struct, looking for an expression to process
		for i := 0; i < valueOf.NumField(); i++ {
			field := valueOf.Field(i)
			if field.CanInterface() {
				fieldTypeName := field.Type().Name()
				if fieldTypeName == "Expr" || fieldTypeName == "*Expr" || fieldTypeName == "Exprs" {
					// found an Expr type
					if fieldI := field.Interface(); fieldI != nil {
						GetFieldsFromExpr(area, tableFields, fieldI, nil)
					}
				}
			}
		}
	}
}
