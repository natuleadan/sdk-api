# 400-auth/ory-stack — Ory Kratos (identity) + Ory Keto (authorization)

Same resource API as `manual-pg`, but **identity comes from Ory Kratos** and
**authorization is 100% Ory Keto** (ReBAC). No manual login: the service
validates the Kratos session and asks Keto for every `roles`/`permissions` gate.

## How it works

- **Identity — Ory Kratos.** The service validates a **Kratos session**
  (Bearer session token or cookie) through `/sessions/whoami`. Sessions are
  created by the Kratos API login flow (`/self-service/login/api`).
- **Authorization — Ory Keto.** The driver `ory` checks Keto tuples on every
  gate:

  | Gate | Check | Tuple |
  |------|-------|-------|
  | `entry.roles: [admin]` | `assignee` of `roles:admin` | `roles:admin#assignee@user:<id>` |
  | `entry.permissions: [users:manage]` | `perform` on `users:manage` | `roles:admin#assignee` is `users:manage#perform` |

  Keto has separate read (:4466) and write (:4467) APIs; the SDK takes
  `keto_read_url` and `keto_write_url` (or a single `keto_url`).
- **Arbitrary roles.** Role names are free-form and unlimited; permissions are
  explicit (`auth.role_permissions` or `entry.permissions`). The role →
  permission subject sets are seeded automatically at startup.
- **API keys.** `auth_modes: [session, apikey]`: an API key is a machine
  subject whose roles are resolved by Keto (`sk-` prefix, sent raw).

## Bootstrap (no manual Ory setup)

The service **creates the Kratos identity** with a password at startup through
the Kratos Admin API (idempotent: reuses it if it exists) and writes
`.runtime/ory.json` for the tests:

- `KRATOS_ADMIN_URL` → Kratos admin API (e.g. `http://localhost:14434`).
- `KRATOS_PUBLIC_URL` → Kratos public API (e.g. `http://localhost:14433`).
- `KETO_READ_URL` / `KETO_WRITE_URL` → Keto read/write APIs.

The Keto namespaces (`roles`, `products`, `users`, `documents`) are declared in
`keto.yml`.

## Run

```bash
docker compose up -d --build    # postgres + valkey + kratos(+pg) + keto + app
```

Locally against a working copy of the SDK, add a temporary replace and run:

```bash
go mod edit -replace github.com/natuleadan/sdk-api=../../..
go build -o /tmp/svc ./cmd/ && go test -c -o /tmp/tester .
```

The tests use the bootstrapped identity and the Keto APIs; set `KRATOS_URL`,
`KETO_READ_URL` and `KETO_WRITE_URL` for the test process.
