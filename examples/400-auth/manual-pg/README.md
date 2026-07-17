# 400-auth/manual-pg — Manual JWT + API Keys

Full-featured manual auth with role hierarchy, rate limiting, CSRF, and security headers.

## Quick Start

```bash
docker compose up --abort-on-container-exit
```

## Files

| File | Purpose |
|------|---------|
| `main.go` | Service + auth validators + handlers |
| `bench_test.go` | 89 integration tests |
| `service.yaml` | Full auth config with CSRF, security headers, rate limits, tenant scoping |
| `run.sh` | Entrypoint: `--test:Name` for specific tests |

## Tenant Scoping

This example demonstrates multi-tenant data isolation using `tenant_scope` + `tenant_field`:

- **org-alfa** (admin, editor): 2 seed products
- **org-beta** (viewer): 1 seed product

The SDK automatically filters all CRUD operations to the authenticated tenant's `org_id` claim.
Tenant A cannot access Tenant B's data — queries include `WHERE tenant_id = <org_id>` automatically.
