package parser

import (
	"errors"
	"reflect"
	"slices"

	"vitess.io/vitess/go/mysql/config"
	"vitess.io/vitess/go/vt/sqlparser"
)

var (
	ErrSQLStringEmpty = errors.New("SQL string is empty")
	ErrParserNotReady = errors.New("Parser is not ready. Call New() to create a new parser")
)

const (
	parsedFieldsAreaFrom    parsedFieldsArea = "FROM"
	parsedFieldsAreaWhere   parsedFieldsArea = "WHERE"
	parsedFieldsAreaGroupBy parsedFieldsArea = "GROUP BY"
	parsedFieldsAreaSelect  parsedFieldsArea = "SELECT"
)

type parsedFieldsArea string

// Parser is our SQL parser and configuration
type Parser struct {
	parser *sqlparser.Parser
	config Config
}

// Config defines the configuration for the parser
type Config struct{}

// New creates a new parser
func New(cfg Config) (*Parser, error) {
	p, err := sqlparser.New(sqlparser.Options{
		MySQLServerVersion: config.DefaultMySQLVersion,
		TruncateUILen:      512,
		TruncateErrLen:     0,
	})
	if err != nil {
		return nil, err
	}
	return &Parser{parser: p}, nil
}

// ParseSQL parses the SQL string and returns a ParsedFields struct
func (p *Parser) ParseSQL(sql string) (*ParsedFields, error) {
	if sql == "" {
		return nil, ErrSQLStringEmpty
	} else if p.parser == nil {
		return nil, ErrParserNotReady
	}
	stmt, err := p.parser.Parse(sql)
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

// processStatement processes a sqlparser.Statement and extracts fields from the statement
func processStatement(stmt sqlparser.Statement, f *ParsedFields) {
	switch s := stmt.(type) {
	case *sqlparser.Select:
		if s.SelectExprs != nil {
			// get all fields used in SELECT
			for _, expr := range s.SelectExprs {
				getFieldsFromExpr(parsedFieldsAreaSelect, f, expr, nil)
			}
		}
		// get all fields used in FROM, WHERE, HAVING, GROUP BY used in our SELECT statement
		for _, fromExp := range s.From {
			getFieldsFromExpression(parsedFieldsAreaFrom, f, fromExp)
		}
		if s.Where != nil {
			getFieldsFromExpr(parsedFieldsAreaWhere, f, s.Where.Expr, nil)
		}
		if s.Having != nil {
			getFieldsFromExpr(parsedFieldsAreaWhere, f, s.Having.Expr, nil)
		}
		if s.GroupBy != nil {
			for _, expr := range s.GroupBy.Exprs {
				getFieldsFromExpr(parsedFieldsAreaGroupBy, f, expr, nil)
			}
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

// addTableField adds a field for a table to the provided list of table fields
func addTableField(tableFields map[string][]string, tableName, fieldName string) {
	if tableName == "dual" {
		// ignore dual table, this is a dummy derived table
		return
	}
	if fieldName == "" {
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

// getFieldsFromExpression gets fields from the join table expression, processing table name aliases if present
func getFieldsFromExpression(area parsedFieldsArea, tableFields *ParsedFields, exp sqlparser.TableExpr) {
	switch e := exp.(type) {
	case *sqlparser.JoinTableExpr:
		// join table expression, process left and right expressions and conditions
		if e.LeftExpr != nil {
			getFieldsFromExpression(area, tableFields, e.LeftExpr)
		}
		if e.RightExpr != nil {
			getFieldsFromExpression(area, tableFields, e.RightExpr)
		}
		if e.Condition != nil {
			if e.Condition.On != nil {
				getFieldsFromExpr(area, tableFields, e.Condition.On, nil)
			}
		}
	case *sqlparser.AliasedTableExpr:
		// aliased table expression, process the expression and alias
		if e.Expr != nil {
			if tableFields.DefaultTableName == "" {
				// no default table name set, check if we have a table name defined in this expression
				switch eTyped := e.Expr.(type) {
				case sqlparser.TableName:
					// we have a table name, mark as default table
					tableName := eTyped.Name.String()
					if tableName == "dual" {
						// ignore dual table, this is a dummy derived table
						break
					}
					as := eTyped.Qualifier.String()
					if as != "" {
						// an alias is present
						tableFields.AddTable(tableName, as)
					} else {
						tableFields.AddTable(tableName, "")
					}
					tableFields.DefaultTableName = tableName
				}
			}
			getFieldsFromExpr(area, tableFields, e.Expr, &e.As)
		}
	case *sqlparser.ParenTableExpr:
		// parenthesized table expression, process the expression
		for _, expr := range e.Exprs {
			getFieldsFromExpression(area, tableFields, expr)
		}
	default:
	}
}

// getFieldsFromExpr gets fields from the expression, processing table name aliases if present
func getFieldsFromExpr(area parsedFieldsArea, tableFields *ParsedFields, e interface{}, as *sqlparser.IdentifierCS) {
	if e == nil {
		return
	}
	// check if we have a SELECT field alias
	if area == parsedFieldsAreaSelect {
		switch e := e.(type) {
		case *sqlparser.AliasedExpr:
			if name := e.As.String(); name != "" {
				// we got a SELECT field alias
				if !slices.Contains(tableFields.selectAliases, name) {
					tableFields.selectAliases = append(tableFields.selectAliases, name)
				}
				return
			}
		}
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
			getFieldsFromExpr(area, tableFields, *e, nil)
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
			getFieldsFromExpr(area, tableFields, e, nil)
		}
	case *sqlparser.GroupBy:
		for _, expr := range e.Exprs {
			getFieldsFromExpr(area, tableFields, expr, nil)
		}
	case []*sqlparser.Expr:
		// array of pointers to expressions
		for _, expr := range e {
			if expr != nil {
				getFieldsFromExpr(area, tableFields, expr, nil)
			}
		}
	case []sqlparser.Expr:
		// array of expressions
		for _, expr := range e {
			getFieldsFromExpr(area, tableFields, expr, nil)
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
			for i := range valueOf.Len() {
				if valueOfI := valueOf.Index(i).Interface(); valueOfI != nil {
					getFieldsFromExpr(area, tableFields, valueOfI, nil)
				}
			}
			return
		}
		// check each field in the struct, looking for an expression to process
		for i := range valueOf.NumField() {
			field := valueOf.Field(i)
			if field.CanInterface() {
				fieldTypeName := field.Type().Name()
				if fieldTypeName == "Expr" || fieldTypeName == "*Expr" || fieldTypeName == "Exprs" {
					// found an Expr type
					if fieldI := field.Interface(); fieldI != nil {
						getFieldsFromExpr(area, tableFields, fieldI, nil)
					}
				}
			}
		}
	}
}
