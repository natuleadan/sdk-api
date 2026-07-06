package db

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"

	_ "turso.tech/database/tursogo"
)

type TursoTable[T any] struct {
	db        *sql.DB
	info      *TableInfo
	tableName string
	columns   []string
}

func NewTursoTable[T any](url string, tableName string) (*TursoTable[T], error) {
	db, err := TursoOpen(url)
	if err != nil {
		return nil, err
	}
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
	return &TursoTable[T]{db: db, info: info, tableName: tableName, columns: cols}, nil
}

func TursoOpen(url string) (*sql.DB, error) {
	db, err := sql.Open("turso", url)
	if err != nil {
		return nil, fmt.Errorf("db: turso open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

func NewTursoTableFrom[T any](db *sql.DB, tableName string, info *TableInfo) (*TursoTable[T], error) {
	if info == nil {
		var err error
		info, err = parseStruct[T]()
		if err != nil {
			return nil, err
		}
	}
	cols := make([]string, 0, len(info.Fields))
	for _, f := range info.Fields {
		if f.Skip { continue }
		cols = append(cols, f.Column)
	}
	return &TursoTable[T]{db: db, info: info, tableName: tableName, columns: cols}, nil
}

func (t *TursoTable[T]) columnsList() string { return strings.Join(t.columns, ", ") }

func (t *TursoTable[T]) placeholder(n int) string {
	// SQLite uses ? placeholders, not $1
	ps := make([]string, n)
	for i := range ps {
		ps[i] = "?"
	}
	return strings.Join(ps, ", ")
}

func (t *TursoTable[T]) AutoInit(ctx context.Context) error {
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

	query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n  %s\n)", t.tableName, strings.Join(parts, ",\n  "))
	if _, err := t.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("db: turso migrate: %w", err)
	}
	for _, idx := range indexes {
		if _, err := t.db.ExecContext(ctx, idx); err != nil {
			return fmt.Errorf("db: turso index: %w", err)
		}
	}
	return nil
}

func (t *TursoTable[T]) buildColumnDef(f FieldInfo) string {
	var parts []string
	parts = append(parts, f.Column)

	if f.Auto { parts = append(parts, "INTEGER PRIMARY KEY AUTOINCREMENT") }
	if !f.Auto {
		switch f.FieldType.Kind() {
		case reflect.Int, reflect.Int64: parts = append(parts, "INTEGER")
		case reflect.Float64: parts = append(parts, "REAL")
		case reflect.String: parts = append(parts, "TEXT")
		case reflect.Bool: parts = append(parts, "INTEGER")
		default: parts = append(parts, "TEXT")
		}
		if f.Required { parts = append(parts, "NOT NULL") }
		if f.Default != "" { parts = append(parts, "DEFAULT "+f.Default) }
	}
	if f.Primary && !f.Auto { parts = append(parts, "PRIMARY KEY") }
	return strings.Join(parts, " ")
}

func (t *TursoTable[T]) scanRow(row *sql.Row, entity *T) error {
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

func (t *TursoTable[T]) scanRows(rows *sql.Rows) ([]T, error) {
	var result []T
	for rows.Next() {
		var entity T
		v := reflect.ValueOf(&entity).Elem()
		ptrs := make([]any, 0, len(t.columns))
		for _, col := range t.columns {
			fi := t.columnField(col)
			if fi == nil { ptrs = append(ptrs, new(any)); continue }
			fv := v.FieldByName(fi.GoName)
			if !fv.IsValid() || !fv.CanInterface() {
				ptrs = append(ptrs, new(any)); continue
			}
			ptrs = append(ptrs, fv.Addr().Interface())
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("db: turso scan: %w", err)
		}
		result = append(result, entity)
	}
	return result, rows.Err()
}

func (t *TursoTable[T]) columnField(column string) *FieldInfo {
	for i, f := range t.info.Fields {
		if f.Column == column { return &t.info.Fields[i] }
	}
	return nil
}

func (t *TursoTable[T]) List(ctx context.Context) ([]T, error) {
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
	if err != nil { return nil, fmt.Errorf("db: turso list: %w", err) }
	defer func() { if err := rows.Close(); err != nil { fmt.Printf("close error: %v\n", err) } }()
	return t.scanRows(rows)
}

func (t *TursoTable[T]) Get(ctx context.Context, id any) (*T, error) {
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
		if err == sql.ErrNoRows { return nil, ErrNotFound }
		return nil, fmt.Errorf("db: turso get: %w", err)
	}
	return &entity, nil
}

func (t *TursoTable[T]) Create(ctx context.Context, entity *T) error {
	v := reflect.ValueOf(entity).Elem()
	var cols []string
	var vals []any
	for _, f := range t.info.Fields {
		if f.Skip || f.Auto { continue }
		fv := v.FieldByName(f.GoName)
		if !fv.IsValid() { continue }
		if f.Default != "" && fv.IsZero() { continue }
		cols = append(cols, f.Column)
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
	if err != nil { return fmt.Errorf("db: turso create: %w", err) }

	id, err := res.LastInsertId()
	if err != nil { return fmt.Errorf("db: turso lastid: %w", err) }

	v.FieldByName(t.info.Fields[0].GoName).SetInt(id)
	return nil
}

func (t *TursoTable[T]) Update(ctx context.Context, id any, patch map[string]any) (*T, error) {
	if len(patch) == 0 { return nil, fmt.Errorf("db: turso update: no fields") }
	var sets []string
	var args []any
	for col, val := range patch {
		sets = append(sets, fmt.Sprintf("%s = ?", col))
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
	b.WriteString(" = ? RETURNING ")
	b.WriteString(t.columnsList())
	query := b.String()
	// SQLite supports RETURNING since 3.35.0
	row := t.db.QueryRowContext(ctx, query, args...)
	var entity T
	if err := t.scanRow(row, &entity); err != nil {
		if err == sql.ErrNoRows { return nil, ErrNotFound }
		return nil, fmt.Errorf("db: turso update: %w", err)
	}
	return &entity, nil
}

func (t *TursoTable[T]) Delete(ctx context.Context, id any) error {
	res, err := t.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s = ?", t.tableName, t.info.PrimaryKey), id)
	if err != nil { return fmt.Errorf("db: turso delete: %w", err) }
	n, _ := res.RowsAffected()
	if n == 0 { return ErrNotFound }
	return nil
}

func (t *TursoTable[T]) Close() error { return t.db.Close() }
