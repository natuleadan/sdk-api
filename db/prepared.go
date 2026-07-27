package db

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const preparedStmtPrefix = "sdk_"

type PreparedTable[T any] struct {
	*Table[T]
	pool    *pgxpool.Pool
	mu      sync.RWMutex
	stmts   map[string]string
	prepped map[string]bool
}

func NewPreparedTable[T any](pool *pgxpool.Pool, tableName string) (*PreparedTable[T], error) {
	tbl, err := NewTable[T](pool, tableName)
	if err != nil {
		return nil, err
	}
	return &PreparedTable[T]{
		Table:   tbl,
		pool:    pool,
		stmts:   make(map[string]string),
		prepped: make(map[string]bool),
	}, nil
}

func (pt *PreparedTable[T]) stmtName(op string) string {
	return preparedStmtPrefix + pt.tableName + ":" + op
}

func (pt *PreparedTable[T]) buildSQL(op string) (string, error) {
	switch op {
	case "list":
		return fmt.Sprintf("SELECT %s FROM %s ORDER BY %s LIMIT %d",
			pt.columnsList(), pt.tableName, pt.info.PrimaryKey, defaultListLimit), nil
	case "get":
		return fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1",
			pt.columnsList(), pt.tableName, pt.info.PrimaryKey), nil
	case "count":
		return fmt.Sprintf("SELECT COUNT(*) FROM %s", pt.tableName), nil
	case "exists":
		return fmt.Sprintf("SELECT 1 FROM %s WHERE %s = $1 LIMIT 1",
			pt.tableName, pt.info.PrimaryKey), nil
	case "delete":
		return fmt.Sprintf("DELETE FROM %s WHERE %s = $1",
			pt.tableName, pt.info.PrimaryKey), nil
	default:
		return "", fmt.Errorf("prepared: unknown op %q", op)
	}
}

func (pt *PreparedTable[T]) prepare(ctx context.Context, op string) error {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pt.prepped[op] {
		return nil
	}

	sql, err := pt.buildSQL(op)
	if err != nil {
		return err
	}

	conn, err := pt.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("prepared: acquire: %w", err)
	}
	defer conn.Release()

	_, err = conn.Conn().Prepare(ctx, pt.stmtName(op), sql)
	if err != nil {
		return fmt.Errorf("prepared: prepare %s: %w", op, err)
	}

	pt.stmts[op] = sql
	pt.prepped[op] = true
	return nil
}

func (pt *PreparedTable[T]) List(ctx context.Context) ([]T, error) {
	if err := pt.prepare(ctx, "list"); err != nil {
		return pt.Table.List(ctx)
	}
	return queryWithStmt[T](ctx, pt.pool, pt.stmtName("list"))
}

func (pt *PreparedTable[T]) Get(ctx context.Context, id any) (*T, error) {
	if err := pt.prepare(ctx, "get"); err != nil {
		return pt.Table.Get(ctx, id)
	}
	return queryRowWithStmt[T](ctx, pt.pool, pt.stmtName("get"), id)
}

func (pt *PreparedTable[T]) Count(ctx context.Context) (int64, error) {
	if err := pt.prepare(ctx, "count"); err != nil {
		return 0, err
	}
	return queryCountWithStmt(ctx, pt.pool, pt.stmtName("count"))
}

func (pt *PreparedTable[T]) Exists(ctx context.Context, column string, value any) (bool, error) {
	if err := pt.prepare(ctx, "exists"); err != nil {
		return pt.Table.Exists(ctx, column, value)
	}
	if column != pt.info.PrimaryKey {
		return pt.Table.Exists(ctx, column, value)
	}
	return queryExistsWithStmt(ctx, pt.pool, pt.stmtName("exists"), value)
}

func (pt *PreparedTable[T]) Delete(ctx context.Context, id any) error {
	if err := pt.prepare(ctx, "delete"); err != nil {
		return pt.Table.Delete(ctx, id)
	}
	return execWithStmt(ctx, pt.pool, pt.stmtName("delete"), id)
}

func queryWithStmt[T any](ctx context.Context, pool *pgxpool.Pool, stmtName string) ([]T, error) {
	rows, err := pool.Query(ctx, stmtName)
	if err != nil {
		return nil, fmt.Errorf("db: %s: %w", stmtName, err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[T])
}

func queryRowWithStmt[T any](ctx context.Context, pool *pgxpool.Pool, stmtName string, args ...any) (*T, error) {
	rows, err := pool.Query(ctx, stmtName, args...)
	if err != nil {
		return nil, fmt.Errorf("db: %s: %w", stmtName, err)
	}
	defer rows.Close()
	entity, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[T])
	if err != nil {
		return nil, fmt.Errorf("db: %s: %w", stmtName, err)
	}
	return &entity, nil
}

func queryCountWithStmt(ctx context.Context, pool *pgxpool.Pool, stmtName string) (int64, error) {
	var count int64
	err := pool.QueryRow(ctx, stmtName).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("db: %s: %w", stmtName, err)
	}
	return count, nil
}

func queryExistsWithStmt(ctx context.Context, pool *pgxpool.Pool, stmtName string, id any) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, stmtName, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("db: %s: %w", stmtName, err)
	}
	return exists, nil
}

func execWithStmt(ctx context.Context, pool *pgxpool.Pool, stmtName string, args ...any) error {
	_, err := pool.Exec(ctx, stmtName, args...)
	if err != nil {
		return fmt.Errorf("db: %s: %w", stmtName, err)
	}
	return nil
}
