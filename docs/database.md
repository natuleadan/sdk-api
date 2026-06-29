# Database

sdk-api supports **PostgreSQL** (primary driver, via pgx), **MySQL** (via go-sql-driver), and **Turso** (SQLite-compatible, via tursogo).

## Configuration

```yaml
databases:
  - name: pg-main
    driver: postgres
    url: "${DATABASE_URL}"
    pool:
      max_conns: 10
      min_conns: 2
```

Multiple databases = multiple entries in the `databases:` array. Each is referenced by name via `entry[].db` or `exit[].db`.

## Drivers

| Driver | Connection | Table Type | CRUD Provider |
|--------|-----------|------------|---------------|
| `postgres` / `pg` | `*pgxpool.Pool` | `db.Table[T]` | `NewCRUDProvider[T]` |
| `mysql` | `*sql.DB` | `db.MySQLTable[T]` | `NewMySQLCRUDProvider[T]` |
| `turso` | `*sql.DB` | `db.TursoTable[T]` | `NewTursoCRUDProvider[T]` |

## Model Definition

Models are Go structs with `db:""` and `json:""` tags:

```go
type Product struct {
    ID        int64     `db:"id,primary,auto"    json:"id"`
    Name      string    `db:"name,required"      json:"name"`
    Price     float64   `db:"price"              json:"price"`
    Stock     int       `db:"stock,default=0"    json:"stock"`
    CreatedAt time.Time `db:"created_at"         json:"createdAt"`
}
```

### DB Tags

| Tag | Description |
|-----|-------------|
| `primary` | Primary key field |
| `auto` | Auto-increment (PostgreSQL serial, MySQL AUTO_INCREMENT, Turso AUTOINCREMENT) |
| `required` | NOT NULL constraint |
| `default=...` | DEFAULT constraint value |
| `unique` | UNIQUE INDEX |
| `index` | INDEX |
| `-` | Skip this field |

The `db` and `json` tags are independent. DB tags control column names, JSON tags control API serialization.

## CRUD Operations

Three implementations of the `CRUDProvider` interface:

### PostgreSQL

```go
pgPool := svc.Pool("pg-main").(*pgxpool.Pool)
table, _ := db.NewTable[Product](pgPool, "products")
svc.WithCRUD("Product", runtime.NewCRUDProvider(table, &ProductHooks{}))
```

### MySQL

```go
sqlDB := runtime.PoolSQL(nil, "mysql-main")
table, _ := db.NewMySQLTable[Product](sqlDB, "products")
svc.WithCRUD("Product", runtime.NewMySQLCRUDProvider(table, &ProductHooks{}))
```

### Turso

```go
table, _ := db.NewTursoTable[Product]("file://bench.db", "products")
svc.WithCRUD("Product", runtime.NewTursoCRUDProvider(table, &ProductHooks{}))
```

## Pool Sizing

When `pool.max_conns` is 0 or not set, pool size is auto-calculated:

```
max(1, (PG_SERVER_MAX_CONNS - reserved_conns) / REPLICA_COUNT)
```

| Env var | Default | Description |
|---------|---------|-------------|
| `PG_SERVER_MAX_CONNS` | `100` | PostgreSQL `max_connections` |
| `REPLICA_COUNT` | `1` | Number of service replicas |

`reserved_conns` is set per-database in YAML (default: `10`).

## AutoInit

`AutoInit()` creates the table on startup if it doesn't exist:

```go
table.AutoInit(ctx)
```

- Creates `CREATE TABLE IF NOT EXISTS ...` with columns from struct tags
- Creates indexes for `index` and `unique` fields
- Does NOT run migrations (ALTER TABLE). Schema changes must be manual.

## Helpers

```go
runtime.Pool(pools, name)       // any
runtime.PoolPG(pools, name)     // *pgxpool.Pool
runtime.PoolSQL(pools, name)    // *sql.DB
runtime.TableFor[T](pools, poolName, tableName)  // *db.Table[T]
```
