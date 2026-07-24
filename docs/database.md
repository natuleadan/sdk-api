# Database

sdk-api supports **PostgreSQL** (primary driver, via pgx), **MySQL** (via go-sql-driver), **Turso** (SQLite-compatible, via tursogo), and **MongoDB** (via mongo-driver).

## Configuration

```yaml
databases:
  - name: pg-main
    driver: postgres
    url: "${DATABASE_URL}"
    pool:
      max_conns: 10
      min_conns: 2
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
| `mongo` | `string` (URI) | — | `NewMongoCRUDProvider` |

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
| `type=...` | Override SQL type (e.g. `type=DECIMAL(10,2)`, `type=JSONB`, `type=TEXT[]`) |
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

## AutoInit

`AutoInit()` creates the table on startup if it doesn't exist:

```go
table.AutoInit(ctx)
```

- Creates `CREATE TABLE IF NOT EXISTS ...` with columns from struct tags
- Creates indexes for `index` and `unique` fields
- Applies table-level constraints declared via `TableConstraints` interface (composite UNIQUE, INDEX, CHECK)
- Does NOT run migrations (ALTER TABLE). Schema changes must be manual.

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
