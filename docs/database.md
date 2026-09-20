# Database

sdk-api supports **PostgreSQL** (primary driver, via pgx), **MySQL** (via go-sql-driver), **Turso/libSQL** (4 variants, see below), and **MongoDB** (via mongo-driver).

## Turso / libSQL driver variants

| Driver name | Package | Use case | CGO | Notes |
|-------------|---------|----------|-----|-------|
| `turso` | `turso.tech/database/tursogo` | Local/embedded database + sync | no | Turso engine, MVCC concurrent writes, `NewTursoSyncDb` push/pull (Turso Cloud only) |
| `turso-serverless` | `turso.tech/database/tursogo-serverless` | Remote Turso/libSQL over HTTP | **no** | Pure Go, no native libs; ideal for serverless/edge/Docker |
| `libsql` | `github.com/tursodatabase/libsql-client-go` | Remote libSQL (hrana wire protocol) | no | Pure Go, works with Turso Cloud and Bunny |
| `go-libsql` | `github.com/tursodatabase/go-libsql` | Embedded replicas (reads local, writes to cloud) | **yes** | Build with `-tags golibsql`; replaces libsql-client-go (same driver name) |

> **Mutual exclusion**: `go-libsql` and `libsql-client-go` both register the
> driver name `libsql`. Build with `-tags golibsql` to use go-libsql; the
> default build provides libsql-client-go.

### Remote variant config

```yaml
databases:
  - name: remote-main
    driver: turso-serverless     # or libsql, or go-libsql
    url: "${TURSO_DATABASE_URL}" # libsql://... (Turso Cloud or Bunny)
    auth_token: "${TURSO_AUTH_TOKEN}"
    turso:
      mode: remote
```

### Local variant config

## Configuration

```yaml
databases:
  - name: pg-main
    driver: postgres
    url: "${DATABASE_URL}"
    read_url: "${DATABASE_READ_URL}"       # optional read replica
    pool:
      max_conns: 20
      min_conns: 5
      max_conn_lifetime: 30m
      max_conn_idle_time: 5m
      health_check_period: 1m
      reserved_conns: 10
      statement_timeout: 30s               # per-query timeout
  - name: mongo-main
    driver: mongo
    url: "${MONGO_URI}"
    database: shorturl
    pool:
      max_conns: 100
      min_conns: 10
  - name: local-turso
    driver: turso
    url: "${DATABASE_URL}"
    pool:
      max_conns: 500
    turso:
      mode: local
      busy_timeout: 30000
```

Multiple databases = multiple entries in the `databases:` array. Each is referenced by name via `entry[].db` or `exit[].db`.

## Drivers

| Driver (required) | Connection | Table Type | CRUD Provider |
|-------------------|-----------|------------|---------------|
| `postgres` / `pg` (default) | `*pgxpool.Pool` | `db.Table[T]` | `NewCRUDProvider[T]` |
| `mysql` | `*sql.DB` | `db.MySQLTable[T]` | `NewMySQLCRUDProvider[T]` |
| `turso` | `*sql.DB` | `db.TursoTable[T]` | `NewTursoCRUDProvider[T]` |
| `turso-serverless` | `*sql.DB` | `db.TursoTable[T]` | `NewTursoCRUDProvider[T]` |
| `libsql` | `*sql.DB` | `db.TursoTable[T]` | `NewTursoCRUDProvider[T]` |
| `go-libsql` (build `-tags golibsql`) | `*sql.DB` | `db.TursoTable[T]` | `NewTursoCRUDProvider[T]` |
| `mongo` | `string` (URI) | — | `NewMongoCRUDProvider` |

### Resolving a driver via YAML (YAML-driven)

Drivers are **declared in `service.yaml`** under `databases:`. The runtime
resolves the pool from the `driver:` field; application code never opens a
connection directly:

```yaml
databases:
  - name: main
    driver: turso-serverless     # turso | turso-serverless | libsql | go-libsql
    url: "${TURSO_DATABASE_URL}" # libsql://... (Turso Cloud or Bunny)
    auth_token: "${TURSO_AUTH_TOKEN}"
    turso:
      mode: remote
```

Then register CRUD against the pool **by name** — the runtime owns the
connection:

```go
runtime.TursoMustRegister[Product](svc, "Product", "main", "products", nil)
```

The pool is available to handlers via `svc.Pool("main")` (a `*sql.DB` for all
Turso/libSQL variants). `initTursoServerless`/`initLibsql`/`initGoLibsql` in
the runtime open the connection from the YAML config; app code stays
configuration-driven and never calls `sql.Open` or the driver packages
directly.

> **YAML-driven rule**: always declare the database in `service.yaml` and let
> the runtime resolve it. Do not open connections in application code.

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
| `default=...` | DEFAULT value; `now()` and `gen_random_uuid()` are normalized per driver (Turso omits the uuid default and the app sets the id) |
| `unique` | UNIQUE INDEX |
| `index` | INDEX |
| `type=...` | Override SQL type (e.g. `type=DECIMAL(10,2)`, `type=JSONB`, `type=TEXT[]`); translated per driver (`JSONB`/`TEXT[]` → `JSON` on MySQL, `TEXT` on Turso; `DECIMAL` → `NUMERIC`; `timestamptz` → `DATETIME(3)`/`TEXT`; `UUID` → `CHAR(36)`/`TEXT`) |
| `fk=table.col` | Foreign key reference (e.g. `fk=users.id` generates `REFERENCES users(id)`) |
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
table, _ := db.NewTursoTable[Product]("file://bench.db?_busy_timeout=30000", "products")
svc.WithCRUD("Product", runtime.NewTursoCRUDProvider(table, &ProductHooks{}))
```

Or via YAML with `turso:` block:
```yaml
databases:
  - driver: turso
    url: "${DATABASE_URL}"
    pool:
      max_conns: 500
    turso:
      mode: local        # local | remote — remote skips PRAGMAs (Turso Cloud)
      busy_timeout: 30000
```

### MongoDB

```go
runtime.MongoMustRegister(svc, "Product", "mongo-main", "mydb", "products", "_id")
```

Pool config is set via YAML (`pool.max_conns` → `maxPoolSize`, `maxConnecting` = `max_conns / 10`, capped at 10):
```yaml
databases:
  - driver: mongo
    url: "${MONGO_URI}"
    database: mydb
    pool:
      max_conns: 100
      min_conns: 10
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

### Read Replicas

PostgreSQL databases support a separate read replica pool via `read_url`:

```yaml
databases:
  - name: pg-main
    driver: postgres
    url: "${DATABASE_URL}"
    read_url: "${DATABASE_READ_URL}"
```

Two pools are created: `pg-main` (write) and `pg-main-read` (read-only).
Use `PoolPGRead(name)` to get the read pool with automatic fallback to write:
```go
pool := svc.PoolRead("pg-main")          // returns read pool or write pool
pool := runtime.PoolPGRead(svc.Pools(), "pg-main")  // same, from pools map
```

### Prepared Statements (PostgreSQL)

`PreparedTable[T]` wraps `Table[T]` with explicit pgx statement caching:

```go
table := db.NewTable[Product](pool, "products")
pt := db.NewPreparedTable[Product](pool, "products")
items, _ := pt.List(ctx)   // uses prepared statement
item, _ := pt.Get(ctx, id)  // uses prepared statement
```

Supported operations: `List`, `Get`, `Count`, `Exists`, `Delete`.
Falls back to `Table[T]` methods if preparation fails.

Additionally, `QueryWhere` uses `$N` bind parameters for LIMIT/OFFSET,
enabling pgx's internal statement cache even with dynamic pagination.

### AfterConnect Hooks (PostgreSQL)

Each new PostgreSQL connection runs these session setup commands:

- `SET statement_timeout = '<value>'` — per-query timeout
- `SET application_name = '<name>'` — session identifier

Configured via `pool.statement_timeout` and the database name.
Prevents hung queries and provides visibility in `pg_stat_activity`.

> **Default List Limit:** All `List()` and `ListScoped()` calls apply a limit of 1000
> to prevent accidental full-table scans. This applies to PostgreSQL, Turso, and MySQL.

## AutoInit

`AutoInit()` creates the table on startup if it doesn't exist:

```go
table.AutoInit(ctx)
```

- Creates `CREATE TABLE IF NOT EXISTS ...` with columns from struct tags
- Creates indexes for `index` and `unique` fields
- Applies table-level constraints declared via `TableConstraints` interface (composite UNIQUE, INDEX, CHECK)
- Translates `type=` overrides and `default=` values per driver, so the same model targets PostgreSQL, MySQL and Turso/libSQL (see the dialect notes under **DB Tags**)
- Does NOT run migrations (ALTER TABLE). Use the migration runner below for schema changes.

MongoDB has no DDL. Its AutoInit equivalent is ensuring indexes at startup:
`db.IndexFields[Model]()` derives the `primary`/`unique`/`index` columns and the
extra fields are passed to the Mongo registration:

```go
fields, _ := db.IndexFields[Widget]()
runtime.MongoMustRegister(svc, "Widget", "mongo", "app", "widgets", "slug", fields...)
```

The lookup field (`slug`) gets a **unique** index; the extra fields get plain
(non-unique) indexes via `mon.EnsureIndexField(ctx, field, unique)`. Call
`mon.Disconnect(url)` on graceful shutdown (or test cleanup) so the driver's
background monitors stop.

#### TableConstraints

For composite constraints (UNIQUE across multiple columns), implement the optional interface:

```go
type OAuthSession struct {
    Signature string `db:"signature,required"`
    Type      string `db:"type,required"`
}

func (OAuthSession) Constraints() []db.Constraint {
    return []db.Constraint{
        {Type: "UNIQUE", Columns: []string{"signature", "type"}},
    }
}
```

Supported constraint types: `UNIQUE`, `INDEX`, `CHECK`.

## Migrations

`AutoInit` creates a table that does not exist, but it never evolves one that
does. For schema changes (a new column, a new index, a data backfill) use the
versioned migration runner, so dev, staging and production converge to the same
schema instead of diverging silently.

### Files

Migrations live in a directory (default `migrations/`), one file per version:

```
migrations/
  0001_auth_baseline.sql
  0002_add_users_email.sql
  0002_add_users_email.down.sql   # optional rollback companion
```

The version is the leading integer, so order is explicit and never depends on
directory listing. A `.down.sql` file shares the version of its up migration and
is never treated as a version of its own.

### CLI

```bash
# Connection from service.yaml (first database, or --db <name>)
sdk-api migrate status
sdk-api migrate up
sdk-api migrate down

# Explicit connection (overrides --service)
sdk-api migrate up --driver postgres --dsn "$DATABASE_URL"
sdk-api migrate up --driver turso-serverless --dsn "$TURSO_URL" --auth-token "$TURSO_TOKEN"

# Custom directory
sdk-api migrate up --dir db/migrations
```

- `status` lists every file as `applied`, `pending`, or `MODIFIED` (a file that
  changed after being applied).
- `up` applies pending migrations in order, each in its own transaction.
- `down` rolls back the highest applied migration using its `.down.sql`.

### Library

```go
conn, _ := db.OpenForDriver("turso-serverless", url, token)
mig, _ := db.NewMigrator(conn, "migrations")
defer mig.Close()

applied, err := mig.Up(ctx)      // versions applied in this run
status, err := mig.Status(ctx)   // applied / pending / modified
version, err := mig.Down(ctx)    // rolled back version (0 if nothing)
```

### Guarantees

- A `schema_migrations` control table records the applied version, name and a
  SHA-256 checksum of the file.
- A migration runs **exactly once** per environment; re-running `up` is a no-op.
- If an applied file's checksum changes, `up` **fails** instead of guessing: the
  schema and the file would diverge.
- Each migration runs in a transaction, so a failure leaves neither the schema
  nor the control table half-updated.
- File access is confined to the migrations directory with `os.Root`, so a name
  can never escape it.

## Helpers

```go
runtime.Pool(pools, name)       // any
runtime.PoolPG(pools, name)     // *pgxpool.Pool
runtime.PoolSQL(pools, name)    // *sql.DB
runtime.TableFor[T](pools, poolName, tableName)  // *db.Table[T]
```

## Model Generation from SQL

You can generate Go structs from existing SQL `CREATE TABLE` statements using the CLI:

```bash
# From a file
sdk-api model from-sql schema.sql

# From stdin
cat schema.sql | sdk-api model from-sql -

# From MongoDB collection
sdk-api model from-mongo --uri "mongodb://localhost:27017" --db mydb --collection products
```

```sql
CREATE TABLE products (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    price DECIMAL(10,2) NOT NULL DEFAULT 0
);
```

Generates:

```go
type Product struct {
    ID        int64     `db:"id,primary,auto" json:"id"`
    Name      string    `db:"name" json:"name"`
    Price     float64   `db:"price" json:"price"`
    CreatedAt time.Time `db:"created_at" json:"created_at"`
    UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
```

Supports PostgreSQL, MySQL, and SQLite DDL syntax. Column types mapped to Go types (BIGINT→int64, VARCHAR→string, DECIMAL→float64, BOOLEAN→bool, TIMESTAMP→time.Time, etc.).
