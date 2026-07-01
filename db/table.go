package db

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/infra/logx"
)

type Table[T any] struct {
	pool      *pgxpool.Pool
	info      *TableInfo
	tableName string
	columns   []string
}

func NewTable[T any](pool *pgxpool.Pool, tableName string) (*Table[T], error) {
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
	return &Table[T]{
		pool:      pool,
		info:      info,
		tableName: tableName,
		columns:   cols,
	}, nil
}

func (t *Table[T]) columnsList() string {
	return strings.Join(t.columns, ", ")
}

func (t *Table[T]) insertColumns() []string {
	var cols []string
	for _, f := range t.info.Fields {
		if f.Skip || f.Auto {
			continue
		}
		cols = append(cols, f.Column)
	}
	return cols
}

type ColumnValue struct {
	col string
	val any
}

func Col(col string, val any) ColumnValue {
	return ColumnValue{col: col, val: val}
}

func (t *Table[T]) List(ctx context.Context) ([]T, error) {
	query := fmt.Sprintf("SELECT %s FROM %s ORDER BY %s",
		t.columnsList(), t.tableName, t.info.PrimaryKey)
	rows, err := t.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("db: list: %w", err)
	}
	defer rows.Close()

	result, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		return nil, fmt.Errorf("db: list: %w", err)
	}
	return result, nil
}

func (t *Table[T]) Get(ctx context.Context, id any) (*T, error) {
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1",
		t.columnsList(), t.tableName, t.info.PrimaryKey)
	rows, err := t.pool.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("db: get: %w", err)
	}
	defer rows.Close()

	entity, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[T])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("db: get: %w", err)
	}
	return &entity, nil
}

func (t *Table[T]) FindBy(ctx context.Context, column string, value any) (*T, error) {
	if _, err := t.validColumn(column); err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1", t.columnsList(), t.tableName, column)
	rows, err := t.pool.Query(ctx, query, value)
	if err != nil {
		return nil, fmt.Errorf("db: find by %s: %w", column, err)
	}
	defer rows.Close()

	entity, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[T])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("db: find by %s: %w", column, err)
	}
	return &entity, nil
}

func (t *Table[T]) Create(ctx context.Context, entity *T) error {
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
		cols = append(cols, f.Column)
		vals = append(vals, fv.Interface())
	}

	placeholders := make([]string, len(cols))
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING %s",
		t.tableName,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
		t.columnsList())

	rows, err := t.pool.Query(ctx, query, vals...)
	if err != nil {
		return fmt.Errorf("db: create: %w", err)
	}
	defer rows.Close()

	created, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[T])
	if err != nil {
		return fmt.Errorf("db: create: %w", err)
	}
	*entity = created
	return nil
}

func (t *Table[T]) Update(ctx context.Context, id any, patch map[string]any) (*T, error) {
	if len(patch) == 0 {
		return nil, fmt.Errorf("db: update: no fields")
	}

	idx := 1
	var sets []string
	args := make([]any, 0, len(patch)+1)
	for col, val := range patch {
		if _, err := t.validColumn(col); err != nil {
			return nil, err
		}
		sets = append(sets, fmt.Sprintf("%s = $%d", col, idx))
		args = append(args, val)
		idx++
	}
	args = append(args, id)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s = $%d RETURNING %s",
		t.tableName,
		strings.Join(sets, ", "),
		t.info.PrimaryKey,
		idx,
		t.columnsList())

	rows, err := t.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("db: update: %w", err)
	}
	defer rows.Close()

	updated, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[T])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("db: update: %w", err)
	}
	return &updated, nil
}

func (t *Table[T]) Delete(ctx context.Context, id any) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE %s = $1", t.tableName, t.info.PrimaryKey)
	tag, err := t.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("db: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *Table[T]) validColumn(col string) (string, error) {
	for _, f := range t.info.Fields {
		if f.Column == col {
			return col, nil
		}
	}
	return "", fmt.Errorf("db: invalid column %q", col)
}

func (t *Table[T]) ResolveColumn(jsonKey string) string {
	for _, f := range t.info.Fields {
		tag := f.Tags.Get("json")
		if tag == "" {
			if f.Column == jsonKey {
				return f.Column
			}
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == jsonKey {
			return f.Column
		}
	}
	return jsonKey
}

func (t *Table[T]) ResolvePatch(patch map[string]any) map[string]any {
	resolved := make(map[string]any, len(patch))
	for k, v := range patch {
		resolved[t.ResolveColumn(k)] = v
	}
	return resolved
}

func (t *Table[T]) TableInfo() *TableInfo {
	return t.info
}

func (t *Table[T]) PrimaryKey() string {
	return t.info.PrimaryKey
}

func (t *Table[T]) Count(ctx context.Context, where ...ColumnValue) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", t.tableName)
	var args []any
	if len(where) > 0 {
		if _, err := t.validColumn(where[0].col); err != nil {
			return 0, err
		}
		query += " WHERE " + where[0].col + " = $1"
		args = append(args, where[0].val)
	}
	var count int64
	err := t.pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("db: count: %w", err)
	}
	return count, nil
}

func (t *Table[T]) Exists(ctx context.Context, column string, value any) (bool, error) {
	if _, err := t.validColumn(column); err != nil {
		return false, err
	}
	query := fmt.Sprintf("SELECT 1 FROM %s WHERE %s = $1 LIMIT 1", t.tableName, column)
	rows, err := t.pool.Query(ctx, query, value)
	if err != nil {
		return false, fmt.Errorf("db: exists: %w", err)
	}
	defer rows.Close()
	return rows.Next(), nil
}

func (t *Table[T]) Increment(ctx context.Context, id any, column string, amount int64) error {
	if _, err := t.validColumn(column); err != nil {
		return err
	}
	query := fmt.Sprintf("UPDATE %s SET %s = %s + $1 WHERE %s = $2",
		t.tableName, column, column, t.info.PrimaryKey)
	tag, err := t.pool.Exec(ctx, query, amount, id)
	if err != nil {
		return fmt.Errorf("db: increment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *Table[T]) BatchInsert(ctx context.Context, entities []T) error {
	if len(entities) == 0 {
		return nil
	}
	cols := t.insertColumns()
	v := reflect.ValueOf(entities)
	rows := make([][]any, v.Len())
	for i := range v.Len() {
		elem := v.Index(i)
		row := make([]any, 0, len(cols))
		for _, col := range cols {
			fi := t.columnField(col)
			if fi == nil {
				continue
			}
			if fi.Default != "" {
				fv := elem.FieldByName(fi.GoName)
				if fv.IsValid() && fv.IsZero() {
					row = append(row, nil)
					continue
				}
			}
			fv := elem.FieldByName(fi.GoName)
			if fv.IsValid() {
				row = append(row, fv.Interface())
			}
		}
		rows[i] = row
	}
	_, err := t.pool.CopyFrom(
		ctx,
		pgx.Identifier{t.tableName},
		cols,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("db: batch insert: %w", err)
	}
	return nil
}

func (t *Table[T]) columnField(column string) *FieldInfo {
	for i, f := range t.info.Fields {
		if f.Column == column {
			return &t.info.Fields[i]
		}
	}
	return nil
}

func (t *Table[T]) Transaction(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: tx begin: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			logx.Errorf("db: tx rollback: %v", err)
		}
	}()
	if err := fn(tx); err != nil {
		return fmt.Errorf("db: tx: %w", err)
	}
	return tx.Commit(ctx)
}

func (t *Table[T]) ExecRaw(ctx context.Context, sql string, args ...any) (int64, error) {
	tag, err := t.pool.Exec(ctx, sql, args...)
	if err != nil {
		return 0, fmt.Errorf("db: exec: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (t *Table[T]) QueryPaginated(ctx context.Context, page, size int, orderBy string) ([]T, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if orderBy == "" {
		orderBy = t.info.PrimaryKey
	} else if _, err := t.validColumn(orderBy); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * size

	total, err := t.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf("SELECT %s FROM %s ORDER BY %s LIMIT $1 OFFSET $2",
		t.columnsList(), t.tableName, orderBy)
	rows, err := t.pool.Query(ctx, query, size, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("db: paginated: %w", err)
	}
	defer rows.Close()

	result, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		return nil, 0, fmt.Errorf("db: paginated: %w", err)
	}
	return result, total, nil
}

func (t *Table[T]) Upsert(ctx context.Context, entity *T, conflictColumn string) error {
	if _, err := t.validColumn(conflictColumn); err != nil {
		return err
	}
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
		cols = append(cols, f.Column)
		vals = append(vals, fv.Interface())
	}
	var placeholders []string
	for i := range cols {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+1))
	}
	var updates []string
	for _, c := range cols {
		if c != conflictColumn {
			updates = append(updates, fmt.Sprintf("%s = EXCLUDED.%s", c, c))
		}
	}
	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s RETURNING %s",
		t.tableName,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
		conflictColumn,
		strings.Join(updates, ", "),
		t.columnsList(),
	)
	rows, err := t.pool.Query(ctx, query, vals...)
	if err != nil {
		return fmt.Errorf("db: upsert: %w", err)
	}
	defer rows.Close()
	created, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[T])
	if err != nil {
		return fmt.Errorf("db: upsert: %w", err)
	}
	*entity = created
	return nil
}

func (t *Table[T]) QueryWhere(ctx context.Context, where map[string]any, orderBy string, limit, offset int) ([]T, error) {
	if orderBy == "" {
		orderBy = t.info.PrimaryKey
	} else if _, err := t.validColumn(orderBy); err != nil {
		return nil, err
	}
	var query strings.Builder
	query.WriteString(fmt.Sprintf("SELECT %s FROM %s", t.columnsList(), t.tableName))
	var args []any
	idx := 1
	for col, val := range where {
		if _, err := t.validColumn(col); err != nil {
			return nil, err
		}
		if idx == 1 {
			query.WriteString(" WHERE " + col + " = $" + fmt.Sprintf("%d", idx))
		} else {
			query.WriteString(" AND " + col + " = $" + fmt.Sprintf("%d", idx))
		}
		args = append(args, val)
		idx++
	}
	query.WriteString(" ORDER BY " + orderBy)
	if limit > 0 {
		query.WriteString(fmt.Sprintf(" LIMIT %d", limit))
	}
	if offset > 0 {
		query.WriteString(fmt.Sprintf(" OFFSET %d", offset))
	}
	rows, err := t.pool.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("db: where: %w", err)
	}
	defer rows.Close()
	result, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		return nil, fmt.Errorf("db: where: %w", err)
	}
	return result, nil
}
