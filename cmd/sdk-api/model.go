package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type sqlField struct {
	Name string
	Type string
	Tags string
}

func parseCreateTable(sql string) (tableName string, fields []sqlField, err error) {
	lines := strings.Split(sql, "\n")
	inParens := false

	for _, line := range lines {
		processCreateTableLine(line, &tableName, &inParens)
		if !inParens {
			continue
		}
		f := processColumnLine(line)
		if f != nil {
			fields = append(fields, *f)
		}
	}

	if tableName == "" {
		return "", nil, fmt.Errorf("no CREATE TABLE statement found")
	}
	return tableName, fields, nil
}

func processCreateTableLine(line string, tableName *string, inParens *bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if after, ok := strings.CutPrefix(line, "CREATE TABLE"); ok {
		rest := after
		rest = strings.TrimPrefix(rest, " IF NOT EXISTS")
		rest = strings.TrimSpace(rest)
		if before, _, ok := strings.Cut(rest, "("); ok {
			*tableName = strings.TrimSpace(before)
			*tableName = strings.Trim(*tableName, "`\"'")
			*inParens = true
			return
		}
	}
	if strings.Contains(line, "(") {
		*inParens = true
	}
}

func isNonColumnLine(upper string) bool {
	return upper == "" || strings.HasPrefix(upper, "PRIMARY") || strings.HasPrefix(upper, "INDEX") ||
		strings.HasPrefix(upper, "UNIQUE") || strings.HasPrefix(upper, "KEY") ||
		strings.HasPrefix(upper, "CONSTRAINT") || strings.HasPrefix(upper, "FOREIGN")
}

func processColumnLine(line string) *sqlField {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	if strings.Contains(line, ")") {
		line = strings.Replace(line, ")", "", 1)
		line = strings.TrimSpace(line)
		if line == "" || strings.HasSuffix(line, ";") {
			return nil
		}
	}
	upper := strings.ToUpper(line)
	if isNonColumnLine(upper) {
		return nil
	}
	line = strings.TrimSuffix(line, ",")
	return parseColumnDef(line)
}

func parseColumnDef(col string) *sqlField {
	col = strings.TrimSpace(col)
	if col == "" {
		return nil
	}
	parts := strings.Fields(col)
	if len(parts) < 2 {
		return nil
	}

	name := strings.Trim(parts[0], "`\"'")
	sqlType := strings.ToUpper(parts[1])

	goType := sqlToGoType(sqlType)
	tags := "db:\"" + toSnake(name) + "\""

	if strings.Contains(strings.ToUpper(col), "PRIMARY KEY") || strings.Contains(strings.ToUpper(col), "SERIAL") || strings.Contains(strings.ToUpper(col), "AUTO_INCREMENT") {
		tags += ",primary,auto"
	}
	tags += " json:\"" + toSnake(name) + "\""

	return &sqlField{Name: pascalCase(name), Type: goType, Tags: tags}
}

var sqlTypeMap = []struct {
	prefix string
	goType string
}{
	{"BIGINT", "int64"},
	{"BIGSERIAL", "int64"},
	{"INT", "int"},
	{"SMALLINT", "int"},
	{"TINYINT", "int"},
	{"SERIAL", "int64"},
	{"DECIMAL", "float64"},
	{"NUMERIC", "float64"},
	{"FLOAT", "float64"},
	{"DOUBLE", "float64"},
	{"BOOL", "bool"},
	{"TIMESTAMP", "time.Time"},
	{"DATETIME", "time.Time"},
	{"DATE", "time.Time"},
	{"TEXT", "string"},
	{"VARCHAR", "string"},
	{"CHAR", "string"},
	{"JSON", "string"},
}

func sqlToGoType(sqlType string) string {
	for _, m := range sqlTypeMap {
		if strings.HasPrefix(sqlType, m.prefix) {
			return m.goType
		}
	}
	return "string"
}

func singular(s string) string {
	if s == "" || s[len(s)-1] != 's' {
		return s
	}
	if len(s) > 3 && s[len(s)-3:] == "ies" {
		return s[:len(s)-3] + "y"
	}
	if len(s) > 1 && s[len(s)-2:] == "es" {
		return s[:len(s)-2]
	}
	return s[:len(s)-1]
}

func runModelFromSQL(sql string) error {
	tableName, fields, err := parseCreateTable(sql)
	if err != nil {
		return err
	}

	modelName := pascalCase(singular(tableName))
	fmt.Printf("// Model generated from SQL table: %s\n", tableName)
	fmt.Printf("type %s struct {\n", modelName)
	fmt.Printf("\tID %s `db:\"id,primary,auto\" json:\"id\"`\n", "int64")

	for _, f := range fields {
		name := strings.ToUpper(f.Name)
		if name == "ID" || name == "CREATED_AT" || name == "UPDATED_AT" {
			continue
		}
		fmt.Printf("\t%s %s `%s`\n", f.Name, f.Type, f.Tags)
	}

	fmt.Printf("\tCreatedAt time.Time `db:\"created_at\" json:\"created_at\"`\n")
	fmt.Printf("\tUpdatedAt time.Time `db:\"updated_at\" json:\"updated_at\"`\n")
	fmt.Printf("}\n")
	return nil
}

func runModelFromSQLFile(path string) error {
	clean := filepath.Clean(path)
	data, err := os.ReadFile(clean)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	return runModelFromSQL(string(data))
}
