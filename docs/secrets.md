# Secrets Management

## ${VAR} Expansion (built-in)

All YAML config values support `${VAR}` substitution. The SDK uses `os.ExpandEnv()` to replace `${VAR}` placeholders with environment variable values at load time.

```yaml
databases:
  - name: primary
    driver: postgres
    url: "${DATABASE_URL}"        # ← reads from env DATABASE_URL

nats:
  - name: default
    url: "${NATS_URL}"

entry:
  - type: file
    path: /files/upload
    handler: onUpload
    storage:
      mode: s3
      access_key: "${S3_ACCESS_KEY}"
      secret_key: "${S3_SECRET_KEY}"
```

**Never hardcode secrets in YAML.** If the SDK detects a value that looks like a plaintext secret (password, secret, key, token, auth), it logs a warning at startup.

## Best Practices

1. **Use environment variables** for all secrets (`${VAR}` syntax)
2. **Never commit `.yaml` files containing plaintext secrets** to git
3. **Use SOPS/age** for encrypting secrets in repositories (see sdk-ops)
4. **Prefer short-lived credentials** (tokens with expiry, not static keys)

## SOPS/age Integration (sdk-ops)

For encrypted secrets at rest, use SOPS with age keys:

```bash
# Generate age key
sdk-ops secrets init

# Encrypt a config file
sdk-ops secrets encrypt service.yaml > service.enc.yaml

# Decrypt at deploy time
sdk-ops secrets decrypt service.enc.yaml > service.yaml
```

See `sdk-ops` documentation for details.
