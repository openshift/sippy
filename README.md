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
