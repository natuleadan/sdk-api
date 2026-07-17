package db

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLTable[T any] struct {
	db        *sql.DB
	info      *TableInfo
	tableName string
	columns   []string
}

func MySQLOpen(url string) (*sql.DB, error) {
	db, err := sql.Open("mysql", url)
	if err != nil {
		return nil, fmt.Errorf("db: mysql open: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
	return db, nil
}

func NewMySQLTable[T any](db *sql.DB, tableName string) (*MySQLTable[T], error) {
	info, err := parseStruct[T]()
	if err != nil {
		return nil, err
	}
	cols := make([]string, 0, len(info.Fields))
	for _, f := range info.Fields {
		if f.Skip {
			continue
		}
		cols = append(cols, f.Column)
	}
	return &MySQLTable[T]{db: db, info: info, tableName: tableName, columns: cols}, nil
}

func NewMySQLTableFromURL[T any](url string, tableName string) (*MySQLTable[T], error) {
	db, err := MySQLOpen(url)
	if err != nil {
		return nil, err
	}
	return NewMySQLTable[T](db, tableName)
}

func (t *MySQLTable[T]) columnsList() string { return strings.Join(t.columns, ", ") }

func (t *MySQLTable[T]) columnField(column string) *FieldInfo {
	for i, f := range t.info.Fields {
		if f.Column == column {
			return &t.info.Fields[i]
		}
	}
	return nil
}

func (t *MySQLTable[T]) AutoInit(ctx context.Context) error {
	var parts []string
	var indexes []string

	for _, f := range t.info.Fields {
		if f.Skip {
			continue
		}
		col := t.buildColumnDef(f)
		parts = append(parts, col)
		if f.Index || f.Unique {
			idxType := "INDEX"
			if f.Unique {
				idxType = "UNIQUE INDEX"
			}
			idxName := fmt.Sprintf("idx_%s_%s", t.tableName, f.Column)
			indexes = append(indexes, fmt.Sprintf("CREATE %s IF NOT EXISTS %s ON %s (%s)", idxType, idxName, t.tableName, f.Column))
		}
	}

	var query strings.Builder
	query.Grow(128)
	query.WriteString("CREATE TABLE IF NOT EXISTS ")
	query.WriteString(t.tableName)
	query.WriteString(" (\n  ")
	query.WriteString(strings.Join(parts, ",\n  "))
	query.WriteString("\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4")
	if _, err := t.db.ExecContext(ctx, query.String()); err != nil {
		return fmt.Errorf("db: mysql migrate: %w", err)
	}
	for _, idx := range indexes {
		var sb strings.Builder
		sb.WriteString(idx)
		if _, err := t.db.ExecContext(ctx, sb.String()); err != nil {
			// index may already exist (MySQL < 8 / MariaDB lacks IF NOT EXISTS)
			fmt.Printf("db: mysql index warn: %v\n", err)
		}
	}
	return nil
}

func (t *MySQLTable[T]) buildColumnDef(f FieldInfo) string {
	var parts []string
	parts = append(parts, "`"+f.Column+"`")

	if f.Auto {
		parts = append(parts, "BIGINT UNSIGNED AUTO_INCREMENT")
		if f.Primary {
			parts = append(parts, "PRIMARY KEY")
		}
	} else {
		switch f.FieldType.Kind() {
		case reflect.Int, reflect.Int64:
			parts = append(parts, "BIGINT")
		case reflect.Float64:
			parts = append(parts, "DOUBLE")
		case reflect.String:
			parts = append(parts, "VARCHAR(255)")
		case reflect.Bool:
			parts = append(parts, "TINYINT(1)")
		default:
			switch {
			case f.FieldType.Kind() == reflect.Slice || f.FieldType.Kind() == reflect.Map:
				parts = append(parts, "JSON")
			case f.FieldType.Name() == "Time":
				parts = append(parts, "DATETIME(3)")
			default:
				parts = append(parts, "VARCHAR(255)")
			}
		}
		if f.Required {
			parts = append(parts, "NOT NULL")
		} else {
			parts = append(parts, "NULL")
		}
		if f.Primary {
			parts = append(parts, "PRIMARY KEY")
		}
		if f.Default != "" {
			parts = append(parts, "DEFAULT "+f.Default)
		}
	}
	return strings.Join(parts, " ")
}

func (t *MySQLTable[T]) scanRow(row *sql.Row, entity *T) error {
	v := reflect.ValueOf(entity).Elem()
	ptrs := make([]any, 0, len(t.columns))
	for _, col := range t.columns {
		fi := t.columnField(col)
		if fi == nil {
			ptrs = append(ptrs, new(any))
			continue
		}
		fv := v.FieldByName(fi.GoName)
		if !fv.IsValid() || !fv.CanInterface() {
			ptrs = append(ptrs, new(any))
			continue
		}
		ptrs = append(ptrs, fv.Addr().Interface())
	}
	return row.Scan(ptrs...)
}

func (t *MySQLTable[T]) scanRows(rows *sql.Rows) ([]T, error) {
	var result []T
	for rows.Next() {
		var entity T
		v := reflect.ValueOf(&entity).Elem()
		ptrs := make([]any, 0, len(t.columns))
		for _, col := range t.columns {
			fi := t.columnField(col)
			if fi == nil {
				ptrs = append(ptrs, new(any))
				continue
			}
			fv := v.FieldByName(fi.GoName)
			if !fv.IsValid() || !fv.CanInterface() {
				ptrs = append(ptrs, new(any))
				continue
			}
			ptrs = append(ptrs, fv.Addr().Interface())
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("db: mysql scan: %w", err)
		}
		result = append(result, entity)
	}
	return result, rows.Err()
}

func (t *MySQLTable[T]) List(ctx context.Context) ([]T, error) {
	var b strings.Builder
	b.Grow(128)
	b.WriteString("SELECT ")
	b.WriteString(t.columnsList())
	b.WriteString(" FROM ")
	b.WriteString(t.tableName)
	b.WriteString(" ORDER BY ")
	b.WriteString(t.info.PrimaryKey)
	query := b.String()
	rows, err := t.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("db: mysql list: %w", err)
	}
	defer func() { if err := rows.Close(); err != nil { fmt.Printf("close error: %v\n", err) } }()
	return t.scanRows(rows)
}

func (t *MySQLTable[T]) ListScoped(ctx context.Context, tenantField string, tenantID string) ([]T, error) {
	if t.columnField(tenantField) == nil {
		return nil, fmt.Errorf("db: mysql list scoped: invalid column %q", tenantField)
	}
	var b strings.Builder
	b.Grow(128)
	b.WriteString("SELECT ")
	b.WriteString(t.columnsList())
	b.WriteString(" FROM ")
	b.WriteString(t.tableName)
	b.WriteString(" WHERE ")
	b.WriteString(tenantField)
	b.WriteString(" = ? ORDER BY ")
	b.WriteString(t.info.PrimaryKey)
	rows, err := t.db.QueryContext(ctx, b.String(), tenantID)
	if err != nil {
		return nil, fmt.Errorf("db: mysql list scoped: %w", err)
	}
	defer func() { if err := rows.Close(); err != nil { fmt.Printf("close error: %v\n", err) } }()
	return t.scanRows(rows)
}

func (t *MySQLTable[T]) QueryKeyset(ctx context.Context, cursor string, size int, orderBy string, where map[string]any) ([]T, string, error) {
	if orderBy == "" {
		orderBy = t.info.PrimaryKey
	}
	var b strings.Builder
	b.Grow(128)
	b.WriteString("SELECT ")
	b.WriteString(t.columnsList())
	b.WriteString(" FROM ")
	b.WriteString(t.tableName)

	var args []any
	if cursor != "" {
		b.WriteString(" WHERE ")
		b.WriteString(orderBy)
		b.WriteString(" > ?")
		args = append(args, cursor)
	}
	for col, val := range where {
		if cursor != "" || len(args) > 0 {
			b.WriteString(" AND ")
			b.WriteString(col)
			b.WriteString(" = ?")
		} else {
			b.WriteString(" WHERE ")
			b.WriteString(col)
			b.WriteString(" = ?")
		}
		args = append(args, val)
	}
	b.WriteString(" ORDER BY ")
	b.WriteString(orderBy)
	if orderBy != t.info.PrimaryKey {
		b.WriteString(", ")
		b.WriteString(t.info.PrimaryKey)
	}
	b.WriteString(" LIMIT ?")
	args = append(args, size+1)

	rows, err := t.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, "", fmt.Errorf("db: mysql keyset: %w", err)
	}
	defer func() { if err := rows.Close(); err != nil { fmt.Printf("close error: %v\n", err) } }()

	result, err := t.scanRows(rows)
	if err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if len(result) > size {
		v := reflect.ValueOf(result[size-1])
		for _, f := range t.info.Fields {
			if f.Column == orderBy {
				fv := v.FieldByName(f.GoName)
				if fv.IsValid() {
					nextCursor = fmt.Sprintf("%v", fv.Interface())
				}
				break
			}
		}
		result = result[:size]
	}
	return result, nextCursor, nil
}

func (t *MySQLTable[T]) Get(ctx context.Context, id any) (*T, error) {
	var b strings.Builder
	b.Grow(128)
	b.WriteString("SELECT ")
	b.WriteString(t.columnsList())
	b.WriteString(" FROM ")
	b.WriteString(t.tableName)
	b.WriteString(" WHERE ")
	b.WriteString(t.info.PrimaryKey)
	b.WriteString(" = ?")
	query := b.String()
	row := t.db.QueryRowContext(ctx, query, id)
	var entity T
	if err := t.scanRow(row, &entity); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("db: mysql get: %w", err)
	}
	return &entity, nil
}

func (t *MySQLTable[T]) Create(ctx context.Context, entity *T) error {
	v := reflect.ValueOf(entity).Elem()
	var cols []string
	var vals []any
	for _, f := range t.info.Fields {
		if f.Skip || f.Auto {
			continue
		}
		fv := v.FieldByName(f.GoName)
		if !fv.IsValid() {
			continue
		}
		if f.Default != "" && fv.IsZero() {
			continue
		}
		cols = append(cols, "`"+f.Column+"`")
		vals = append(vals, fv.Interface())
	}

	var b strings.Builder
	b.Grow(128)
	b.WriteString("INSERT INTO ")
	b.WriteString(t.tableName)
	b.WriteString(" (")
	b.WriteString(strings.Join(cols, ", "))
	b.WriteString(") VALUES (")
	b.WriteString(t.placeholder(len(cols)))
	b.WriteString(")")
	query := b.String()
	res, err := t.db.ExecContext(ctx, query, vals...)
	if err != nil {
		return fmt.Errorf("db: mysql create: %w", err)
	}

	if t.info.PrimaryKey != "" {
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("db: mysql lastid: %w", err)
		}
		for _, f := range t.info.Fields {
			if f.Primary {
				v.FieldByName(f.GoName).SetInt(id)
				break
			}
		}
	}
	return nil
}

func (t *MySQLTable[T]) Update(ctx context.Context, id any, patch map[string]any) (*T, error) {
	if len(patch) == 0 {
		return nil, fmt.Errorf("db: mysql update: no fields")
	}
	var sets []string
	var args []any
	for col, val := range patch {
		sets = append(sets, "`"+col+"` = ?")
		args = append(args, val)
	}
	args = append(args, id)

	var b strings.Builder
	b.Grow(128)
	b.WriteString("UPDATE ")
	b.WriteString(t.tableName)
	b.WriteString(" SET ")
	b.WriteString(strings.Join(sets, ", "))
	b.WriteString(" WHERE ")
	b.WriteString(t.info.PrimaryKey)
	b.WriteString(" = ?")
	query := b.String()
	if _, err := t.db.ExecContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("db: mysql update: %w", err)
	}

	return t.Get(ctx, id)
}

func (t *MySQLTable[T]) Delete(ctx context.Context, id any) error {
	var b strings.Builder
	b.Grow(64)
	b.WriteString("DELETE FROM ")
	b.WriteString(t.tableName)
	b.WriteString(" WHERE ")
	b.WriteString(t.info.PrimaryKey)
	b.WriteString(" = ?")
	res, err := t.db.ExecContext(ctx, b.String(), id)
	if err != nil {
		return fmt.Errorf("db: mysql delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *MySQLTable[T]) GetScoped(ctx context.Context, id any, tenantField string, tenantID string) (*T, error) {
	if t.columnField(tenantField) == nil {
		return nil, fmt.Errorf("db: mysql get scoped: invalid column %q", tenantField)
	}
	var b strings.Builder
	b.Grow(128)
	b.WriteString("SELECT ")
	b.WriteString(t.columnsList())
	b.WriteString(" FROM ")
	b.WriteString(t.tableName)
	b.WriteString(" WHERE ")
	b.WriteString(t.info.PrimaryKey)
	b.WriteString(" = ? AND ")
	b.WriteString(tenantField)
	b.WriteString(" = ?")
	row := t.db.QueryRowContext(ctx, b.String(), id, tenantID)
	var entity T
	if err := t.scanRow(row, &entity); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("db: mysql get scoped: %w", err)
	}
	return &entity, nil
}

func (t *MySQLTable[T]) CreateScoped(ctx context.Context, entity *T, tenantField string, tenantID string) error {
	if t.columnField(tenantField) == nil {
		return fmt.Errorf("db: mysql create scoped: invalid column %q", tenantField)
	}
	v := reflect.ValueOf(entity).Elem()
	for _, f := range t.info.Fields {
		if f.Column == tenantField {
			fv := v.FieldByName(f.GoName)
			if fv.IsValid() && fv.CanSet() {
				fv.SetString(tenantID)
			}
			break
		}
	}
	return t.Create(ctx, entity)
}

func (t *MySQLTable[T]) UpdateScoped(ctx context.Context, id any, patch map[string]any, tenantField string, tenantID string) (*T, error) {
	if t.columnField(tenantField) == nil {
		return nil, fmt.Errorf("db: mysql update scoped: invalid column %q", tenantField)
	}
	if len(patch) == 0 {
		return nil, fmt.Errorf("db: mysql update scoped: no fields")
	}
	var sets []string
	var args []any
	for col, val := range patch {
		sets = append(sets, "`"+col+"` = ?")
		args = append(args, val)
	}
	args = append(args, id, tenantID)

	var b strings.Builder
	b.Grow(128)
	b.WriteString("UPDATE ")
	b.WriteString(t.tableName)
	b.WriteString(" SET ")
	b.WriteString(strings.Join(sets, ", "))
	b.WriteString(" WHERE ")
	b.WriteString(t.info.PrimaryKey)
	b.WriteString(" = ? AND ")
	b.WriteString(tenantField)
	b.WriteString(" = ?")
	if _, err := t.db.ExecContext(ctx, b.String(), args...); err != nil {
		return nil, fmt.Errorf("db: mysql update scoped: %w", err)
	}
	return t.GetScoped(ctx, id, tenantField, tenantID)
}

func (t *MySQLTable[T]) DeleteScoped(ctx context.Context, id any, tenantField string, tenantID string) error {
	if t.columnField(tenantField) == nil {
		return fmt.Errorf("db: mysql delete scoped: invalid column %q", tenantField)
	}
	var b strings.Builder
	b.Grow(64)
	b.WriteString("DELETE FROM ")
	b.WriteString(t.tableName)
	b.WriteString(" WHERE ")
	b.WriteString(t.info.PrimaryKey)
	b.WriteString(" = ? AND ")
	b.WriteString(tenantField)
	b.WriteString(" = ?")
	res, err := t.db.ExecContext(ctx, b.String(), id, tenantID)
	if err != nil {
		return fmt.Errorf("db: mysql delete scoped: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *MySQLTable[T]) placeholder(n int) string {
	ps := make([]string, n)
	for i := range ps {
		ps[i] = "?"
	}
	return strings.Join(ps, ", ")
}

func (t *MySQLTable[T]) Close() error {
	return t.db.Close()
}

func (t *MySQLTable[T]) DB() *sql.DB {
	return t.db
}
