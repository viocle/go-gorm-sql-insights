package parser

import (
	"slices"
	"strings"
)

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

	// DefaultTableName is the default table name fields are mapped to use when no table name is specified. We're using the first aliased table definition to set this
	DefaultTableName string

	selectAliases []string
}

// Equal compares this ParsedFields to another and returns true if they are equal. Order of fields is not considered
func (f *ParsedFields) Equal(other *ParsedFields) bool {
	if f == nil && other == nil {
		return true
	}
	if f == nil || other == nil {
		return false
	}
	if f.DefaultTableName != other.DefaultTableName {
		return false
	}
	if len(f.FromFields) != len(other.FromFields) {
		return false
	}
	if len(f.WhereFields) != len(other.WhereFields) {
		return false
	}
	if len(f.GroupByFields) != len(other.GroupByFields) {
		return false
	}
	if len(f.TableFields) != len(other.TableFields) {
		return false
	}
	if len(f.AliasMap) != len(other.AliasMap) {
		return false
	}
	for key, value := range f.FromFields {
		otherValue, ok := other.FromFields[key]
		if !ok {
			return false
		}
		for _, fieldName := range value {
			if !slices.Contains(otherValue, fieldName) {
				return false
			}
		}
	}
	for key, value := range f.WhereFields {
		otherValue, ok := other.WhereFields[key]
		if !ok {
			return false
		}
		for _, fieldName := range value {
			if !slices.Contains(otherValue, fieldName) {
				return false
			}
		}
	}
	for key, value := range f.GroupByFields {
		otherValue, ok := other.GroupByFields[key]
		if !ok {
			return false
		}
		for _, fieldName := range value {
			if !slices.Contains(otherValue, fieldName) {
				return false
			}
		}
	}
	for key, value := range f.TableFields {
		otherValue, ok := other.TableFields[key]
		if !ok {
			return false
		}
		for _, fieldName := range value {
			if !slices.Contains(otherValue, fieldName) {
				return false
			}
		}
	}
	for key, value := range f.AliasMap {
		if otherValue, ok := other.AliasMap[key]; !ok || value != otherValue {
			return false
		}
	}
	return true
}

// MapDefaultTableName maps empty table names to the default table name
func (f *ParsedFields) MapDefaultTableName(fieldMap map[string][]string) {
	if f.DefaultTableName == "" {
		return
	}
	if values, ok := fieldMap[""]; ok {
		if len(values) > 0 {
			if _, ok := fieldMap[f.DefaultTableName]; !ok {
				fieldMap[f.DefaultTableName] = make([]string, 0, len(values))
			}
			for _, fieldName := range values {
				// skip if this fieldName is listed in the selectAliases list
				if slices.Contains(f.selectAliases, fieldName) {
					continue
				}
				fieldMap[f.DefaultTableName] = append(fieldMap[f.DefaultTableName], fieldName)
			}
			delete(fieldMap, "")
		}
	}
}

// MergeAliasTables merges the table names where an alias is used
func (f *ParsedFields) MergeAliasTables() {
	if f.DefaultTableName != "" {
		// check if we have any empty tables defined and replace with the default table name
		f.MapDefaultTableName(f.FromFields)
		f.MapDefaultTableName(f.WhereFields)
		f.MapDefaultTableName(f.GroupByFields)
		f.MapDefaultTableName(f.TableFields)
	}
	for alias, tableName := range f.AliasMap {
		// merge fields from alias key to table name key
		for _, alias := range []string{alias, strings.ToLower(alias)} {
			mergeTableFields(f.FromFields, alias, tableName)
			mergeTableFields(f.WhereFields, alias, tableName)
			mergeTableFields(f.GroupByFields, alias, tableName)
			mergeTableFields(f.TableFields, alias, tableName)
		}
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
	if area != parsedFieldsAreaSelect {
		// add field to full list of table fields
		addTableField(f.TableFields, tableName, fieldName)
	}
}

// AddTable adds a table to the list of tables, with an optional alias
func (f *ParsedFields) AddTable(tableName, as string) {
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
