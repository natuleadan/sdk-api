package db

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type PaginatedResponse[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

type PageCursor struct {
	LastValue string `json:"v"`
	LastID    string `json:"id,omitempty"`
}

func EncodeCursor(cursor PageCursor) string {
	data, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}

func ParseCursor(encoded string) (PageCursor, error) {
	var cursor PageCursor
	data, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return cursor, err
	}
	if err := json.Unmarshal(data, &cursor); err != nil {
		return cursor, err
	}
	return cursor, nil
}

func NewPaginatedResponse[T any](items []T, nextCursor string) PaginatedResponse[T] {
	return PaginatedResponse[T]{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}
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

func (t *Table[T]) QueryPaginated(ctx context.Context, page, size int, orderBy string) ([]T, int64, error) {
	defer logSlowQuery("QueryPaginated", time.Now(), t.tableName)
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

func (t *Table[T]) QueryKeyset(ctx context.Context, cursor string, size int, orderBy string, where map[string]any) ([]T, string, error) {
	defer logSlowQuery("QueryKeyset", time.Now(), t.tableName)
	if size < 1 {
		size = 10
	}
	if orderBy == "" {
		orderBy = t.info.PrimaryKey
	} else if _, err := t.validColumn(orderBy); err != nil {
		return nil, "", err
	}

	orderField := orderBy
	for _, f := range t.info.Fields {
		if f.Column == orderBy {
			orderField = f.GoName
			break
		}
	}

	var b strings.Builder
	b.Grow(128)
	fmt.Fprintf(&b, "SELECT %s FROM %s", t.columnsList(), t.tableName)

	var args []any
	idx := 1
	if cursor != "" {
		fmt.Fprintf(&b, " WHERE %s > $%d", orderBy, idx)
		args = append(args, cursor)
		idx++
	}
	for col, val := range where {
		if _, err := t.validColumn(col); err != nil {
			return nil, "", err
		}
		if idx == 1 {
			fmt.Fprintf(&b, " WHERE %s = $%d", col, idx)
		} else {
			fmt.Fprintf(&b, " AND %s = $%d", col, idx)
		}
		args = append(args, val)
		idx++
	}
	fmt.Fprintf(&b, " ORDER BY %s", orderBy)
	if orderBy != t.info.PrimaryKey {
		b.WriteString(", ")
		b.WriteString(t.info.PrimaryKey)
	}
	fmt.Fprintf(&b, " LIMIT $%d", idx)
	args = append(args, size+1)

	rows, err := t.pool.Query(ctx, b.String(), args...)
	if err != nil {
		return nil, "", fmt.Errorf("db: keyset: %w", err)
	}
	defer rows.Close()

	result, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		return nil, "", fmt.Errorf("db: keyset: %w", err)
	}

	nextCursor := ""
	if len(result) > size {
		v := reflect.ValueOf(result[size-1])
		fv := v.FieldByName(orderField)
		if fv.IsValid() {
			nextCursor = fmt.Sprintf("%v", fv.Interface())
		}
		result = result[:size]
	}
	return result, nextCursor, nil
}
