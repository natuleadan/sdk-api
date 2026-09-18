# 400-auth/openfga-zitadel — Zitadel (identity) + OpenFGA (authorization)

Same resource API as `manual-pg`, but **identity comes from Zitadel** and
**authorization is 100% OpenFGA** (ReBAC). No manual login: the service validates
Zitadel credentials and asks OpenFGA for every `roles`/`permissions` gate.

## How it works

- **Identity — Zitadel.** Zitadel issues **opaque access tokens** by default,
  so the service validates them with **RFC 7662 introspection**
  (`auth.oauth.introspection_url`) using a client secret. (JWKS would only work
  for JWT-shaped tokens, which Zitadel does not issue for service accounts.)
- **Authorization — OpenFGA.** The driver `openfga-zitadel` derives the
  authorization model from the entry YAML (each resource gets `can_<action>`
  relations) and checks role membership / inherited permissions on every gate:

  | Gate | Check | Tuple |
  |------|-------|-------|
  | `entry.roles: [admin]` | `member` of `role:admin` | `user:<id> member role:admin` |
  | `entry.permissions: [users:manage]` | `can_manage` on `users:manage` | `role:admin#member can_manage users:manage` |

- **Arbitrary roles.** Role names are free-form and unlimited; permissions are
  explicit (`auth.role_permissions` or `entry.permissions`). No fixed set.
- **API keys.** `auth_modes: [oauth, apikey]`: an API key is a machine subject
  whose roles are, again, resolved by OpenFGA (`sk-` prefix, sent raw).

## Bootstrap (no manual Zitadel setup)

The service **creates the Zitadel project + API app** it needs at startup, using
the machine key that `start-from-init` writes to the mounted path:

- `ZITADEL_MACHINEKEY` → path to `machinekey.json` (JWT profile).
- It writes `ZITADEL_INTROSPECTION_URL/CLIENT_ID/CLIENT_SECRET` into the
  environment (consumed by the YAML) and `.runtime/zitadel.json` for the tests.

If `ZITADEL_MACHINEKEY` is unset, the bootstrap is skipped and the introspection
client must be provided via the environment instead.

## Run

```bash
docker compose up -d --build    # postgres + valkey + openfga(+pg) + zitadel(+pg) + app
```

Locally against a working copy of the SDK, add a temporary replace and run:

```bash
go mod edit -replace github.com/natuleadan/sdk-api=../../..
go build -o /tmp/svc ./cmd/ && go test -c -o /tmp/tester .
```

The tests use the machine key and the bootstrapped identifiers; set
`ZITADEL_MACHINEKEY` and `OPENFGA_URL` for the test process.
