# Conventional Commits

This project follows [Conventional Commits](https://www.conventionalcommits.org/) for versioning and changelog generation.

## Format

```
<type>(<scope>): <description>
```

**Scope is required.** Pull requests will be rejected if any commit lacks a scope.

### Examples

```
feat(api): add pagination to list endpoint
fix(db): correct null pointer in product query
docs(readme): update installation guide
test(product): add unit tests for price validation
refactor(server): extract middleware chain into builder
chore(ci): update Go version to 1.27
```

## Allowed Types

| Type     | Description               | Version bump |
|----------|---------------------------|--------------|
| `feat`   | New feature               | minor (or patch in 0.x) |
| `fix`    | Bug fix                   | patch        |
| `docs`   | Documentation             | none         |
| `refactor` | Code refactoring         | none         |
| `test`   | Adding or fixing tests    | none         |
| `chore`  | Build, CI, tooling        | none         |
| `ci`     | CI configuration changes  | none         |
| `perf`   | Performance improvement   | patch        |
| `style`  | Formatting, linting       | none         |
| `build`  | Build system changes      | none         |
| `revert` | Revert a previous commit  | none         |

## Breaking Changes

Add `!` after the type or `BREAKING CHANGE:` in the footer to trigger a major version bump:

```
feat(api)!: change response format

BREAKING CHANGE: new format drops legacy fields
```

## Release Flow

```
1. Push commits to PR
   → CI validates conventional commits format + scope presence
   → CI runs lint + tests + build

2. Merge to main

3. Go to GitHub → Actions → Release → "Run workflow"
   → Select bump: patch / minor / major
   → CI creates tag vX.Y.Z + runs GoReleaser
   → Binaries published to GitHub Releases with checksums
```

## Version Calculation

| Scenario | Example commit | Pre-1.0 bump | Post-1.0 bump |
|----------|---------------|--------------|---------------|
| Bug fix only | `fix(cart): correct total` | 0.1.0 → 0.1.1 | 1.0.0 → 1.0.1 |
| New feature | `feat(api): add login` | 0.1.0 → 0.2.0 | 1.0.0 → 1.1.0 |
| Breaking change | `feat!: change API` | 0.1.0 → 0.2.0 | 1.0.0 → 2.0.0 |

## Local Development

```bash
# Install commitlint for local validation
npm install -g @commitlint/cli @commitlint/config-conventional

# Validate the last commit
echo "feat(api): add login" | commitlint
```
