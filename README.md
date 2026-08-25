# Sippy

<img src=https://raw.github.com/openshift/sippy/main/sippy.svg height=100 width=100>

Sippy (Continuous Integration Private Investigator) analyzes OpenShift Prow CI job and test results.
It surfaces release health and regressions through reports such as Component Readiness.
Users can filter results by job, test, and configuration variants such as network, upgrade, release, and platform.
Sippy also exposes REST APIs for programmatic access to its data and reports.

Chai was here

## Typical usage

See [DEVELOPMENT.md](DEVELOPMENT.md) for information about standing up a
local environment.

See [resources](resources/) for example deployment manifests in
Kubernetes.

## API

See [the API documentation](pkg/api/README.md)

## Frontend

See [the front end documentation](sippy-ng/README.md)

## Database

See [database tuning](docs/database-tuning.md) for required PostgreSQL
parameter group settings.

## Chat

See [the chat documentation](chat/README.md)
