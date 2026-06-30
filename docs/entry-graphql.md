# GraphQL Entry Type

The `type: graphql` entry auto-generates a GraphQL schema from registered models and CRUD providers. Built on `github.com/graphql-go/graphql`.

## Configuration

```yaml
entry:
  - type: graphql
    path: /graphql
```

A single `POST /graphql` endpoint is registered. The schema is built from registered models.

## Registration

Models must be registered with the service:

```go
type Product struct {
    ID    int64   `db:"id,primary,auto"`
    Name  string  `db:"name,required"`
    Price float64 `db:"price"`
}

svc.RegisterModel("Product", (*Product)(nil))
svc.WithCRUD("Product", provider)
```

The `CRUDProvider` for each model provides the resolver logic.

## Schema

Queries and mutations are auto-generated:

### Queries

```graphql
type Query {
    Products: [Product]
    Product(id: ID!): Product
}
```

### Mutations

```graphql
type Mutation {
    createProduct(input: ProductInput!): Product
    updateProduct(id: ID!, input: ProductInput!): Product
    deleteProduct(id: ID!): ID
}
```

### Types

```graphql
type Product {
    Id: ID!
    Name: String!
    Price: Float
}

input ProductInput {
    Name: String!
    Price: Float
}
```

Field names are CamelCase (matching Go struct field names).

## Usage

```bash
# Query
curl -X POST http://localhost:8080/api/v1/graphql \
  -H "Content-Type: application/json" \
  -d '{"query":"{ Products { Id Name } }"}'

# Mutation
curl -X POST http://localhost:8080/api/v1/graphql \
  -H "Content-Type: application/json" \
  -d '{"query":"mutation { createProduct(input: {Name: \"test\"}) { Id Name } }"}'

# Introspection
curl -X POST http://localhost:8080/api/v1/graphql \
  -H "Content-Type: application/json" \
  -d '{"query":"{ __schema { types { name } } }"}'
```

## Limitations

- Only CRUD resolvers are auto-generated (no custom queries/mutations yet)
- Input types exclude primary key and auto-increment fields
- Pagination via `Products(page: Int, size: Int)` (optional)
