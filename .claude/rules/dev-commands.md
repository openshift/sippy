---
paths:
  - "**"
---

### Database migration

Run migrations: `go run ./cmd/sippy migrate --database-dsn $SIPPY_DATABASE_DSN`

If `SIPPY_DATABASE_DSN` is not set, use the dev default: `postgresql://postgres:password@localhost:5432/postgres`

### Linting

Run lint: `CI=true make lint`

`CI=true` makes `hack/go-lint.sh` use the locally installed `golangci-lint` instead of spawning a container.

### Testing

Run unit tests: `make test`

This runs Go tests via gotestsum and sippy-ng Jest tests.
