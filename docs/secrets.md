# Secrets Management

## ${VAR} Expansion (built-in)

All YAML config values support `${VAR}` substitution. The SDK replaces `${VAR}` placeholders with environment variable values at load time.

```yaml
databases:
  - name: primary
    driver: postgres
    url: "${DATABASE_URL}"        # ← reads from env DATABASE_URL

stream:
  - name: default
    driver: nats
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

## ${VAR:default} Syntax

Provide a default value if the environment variable is not set:

```yaml
databases:
  - name: primary
    url: "${DATABASE_URL:postgres://localhost:5432/mydb}"
    max_conns: "${DB_MAX_CONNS:10}"
```

If no default is provided and the variable is not set, the SDK returns an error at startup listing all missing variables.

## .env File Loading

The SDK loads `.env` from the current working directory automatically at startup. Variables in `.env` do not override already-set environment variables.

```bash
# .env
DATABASE_URL=postgres://localhost:5432/dev
NATS_URL=nats://localhost:4222
```

## SOPS/age Integration

For encrypting entire config files at rest (in git), use SOPS with age keys.

### Setup

```bash
# Install sops
brew install sops

# Generate an age key
mkdir -p ~/.config/sops/age
age-keygen -o ~/.config/sops/age/key.txt

# Create .sops.yaml in your project root
sdk-api init --sops --age-key "$(cat ~/.config/sops/age/key.txt | grep 'public key')"
```

### Encrypt a Config File

```bash
make encrypt-config FILE=service.yaml
# Creates service.enc.yaml — safe to commit to git
```

### Decrypt at Startup

The SDK automatically detects SOPS-encrypted files (those containing a `sops:` field) and decrypts them using the `sops` binary at load time. No code changes needed.

```bash
# Run with encrypted config — SDK decrypts transparently
CONFIG_PATH=service.enc.yaml go run .
```

### Makefile Targets

```bash
make decrypt-config FILE=service.enc.yaml    # Decrypt to stdout
make encrypt-config FILE=service.yaml         # Encrypt to .enc.yaml
```

## Best Practices

1. **Use environment variables** for all secrets (`${VAR}` syntax)
2. **Never commit `.yaml` files containing plaintext secrets** to git
3. **Use SOPS/age** for encrypting configs in repositories
4. **Prefer short-lived credentials** (tokens with expiry, not static keys)
