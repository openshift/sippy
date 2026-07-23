# Sippy

<img src=https://raw.github.com/openshift/sippy/main/sippy.svg height=100 width=100>

CIPI (Continuous Integration Private Investigator) aka Sippy -- a tool
to analyze prow job results.

Reports on job and test statistics, sliced by various filters including
name, suite, or NURP+ variants (network, upgrade, release, platform, etc).

## Typical usage

See [DEVELOPMENT.md](DEVELOPMENT.md) for information about standing up a
local environment.

See [resources](resources/) for example deployment manifests in
Kubernetes.

## API

See [the API documentation](pkg/api/README.md)

## Frontend

See [the front end documentation](sippy-ng/README.md)

## Database Operations

After cloning or restoring the production database to staging, query
performance will be degraded until PostgreSQL has fresh planner statistics.
Run `scripts/analyze-db.sh` to execute `ANALYZE VERBOSE` on the database
via a one-shot pod:

```bash
./scripts/analyze-db.sh
```

The pod runs detached, so your local machine does not need to stay
connected. Use `--wait` to block until completion instead. The script
defaults to the `sippy` namespace and `postgres-aws` secret. Use
`--namespace` and `--db-secret` to override, and `--dry-run` to preview
the command without executing.

## Chat

See [the chat documentation](chat/README.md)
