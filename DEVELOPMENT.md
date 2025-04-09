## Building Sippy

Running `make` will build an all-in-one binary that contains both the go app and frontend.

To build just the backend, run `make sippy` (or `make sippy-daemon` if
testing the PR commenter).

## Create PostgreSQL Database

Launch postgresql:

```bash
podman run --name sippy-postgres -e POSTGRES_PASSWORD=password -p 5432:5432 -d quay.io/enterprisedb/postgresql
```

Migrate the database with sippy:

```
./sippy migrate
```

## Populating Data

Sippy obtains data from multiple sources:

| Data Source         | Job names | Job runs | Test results | Release tags | Build cluster | PR State |
|---------------------|-----------|----------|--------------|--------------|---------------|----------|
| Prow                |           | X        |              |              | X             |          |
| Release controller  |           |          |              | X            |               |          |
| Sippy configuration | X         |          |              |              |               |          |
| GCS Storage Buckets |           |          | X            |              |               |          |
| GitHub              |           |          |              |              |               | X        |

**Note**: Test results are only stored from [known suites](pkg/db/suites.go).

### From a Prod Sippy Backup

The simplest way to fetch data for Sippy, is to just get a copy of the production database. TRT stores gzip'd backups in S3 periodically, see the staff slack channel for links or reach out to a team member for more information. Restore it locally with a command like this if you are using the plain .sql file:

```bash
psql -h localhost -U postgres -p 5432 postgres < sippy-backup-2022-05-02.sql
```

or with a command like this if you are using the custom format file (you can ignore errors regarding missing roles):

```bash
pg_restore -h localhost -U postgres -p 5432 --verbose -Fc -d postgres ./sippy-backup-2022-10-20.dump
```

### From Prow and GCS buckets

In order to access the GCS storage buckets where the raw junit data is stored, you need to provide Sippy with a google
service account credential. See the [official google docs](https://cloud.google.com/iam/docs/service-accounts) for
information on how to get one.

Additionally, you need a configuration file that maps job names to releases. One should already be available in
[./config/openshift.yaml](config/openshift.yaml). Information about how to generate this file is
available [here](config/README.md).

```bash
./sippy load \
  --loader prow \
  --release 4.11 \
  --database-dsn="postgresql://postgres:password@localhost:5432/postgres" \
  --mode=ocp \
  --config ./config/openshift.yaml \
  --load-openshift-ci-bigquery \
  --google-service-account-credential-file ~/Downloads/openshift-ci-data-analysis-1b68cb387203.json
```

### From GitHub

When using Prow in GitHub mode, it's possible to sync additional data from GitHub including PR state. GitHub throttles
unauthenticated requests, limited to 60 an hour. If you don't need this in development, then simply omit `--load-github`
, otherwise set `GITHUB_TOKEN` environment variable,
or [configure GitHub in your gitconfig](https://stackoverflow.com/questions/8505335/hiding-github-token-in-gitconfig).

```bash
./sippy load \
  --loader prow \
  --loader github \
  --release 4.11 \
  --database-dsn="postgresql://postgres:password@localhost:5432/postgres" \
  --mode=ocp \
  --config ./config/openshift.yaml \
  --load-openshift-ci-bigquery \
  --google-service-account-credential-file ~/Downloads/openshift-ci-data-analysis-1b68cb387203.json
```

### From the release controller

Sippy retrieves release-related data from release controllers. In order to fetch release data, give Sippy a list of
releases and architectures like this:

```
./sippy load \
  --loader releases \
  --arch amd64 \
  --arch arm64 \
  --release 4.12 \
  --release 4.11 \
  --database-dsn="postgresql://postgres:password@localhost:5432/postgres" \
  --google-service-account-credential-file ~/Downloads/openshift-ci-data-analysis-1b68cb387203.json \
  --mode=ocp \
  --load-openshift-ci-bigquery \
  --config ./config/openshift.yaml
```

## Launch Sippy API

If you are *not* loading a backup for your data, you will need to
initialize and/or update the database schema:

```bash
./sippy migrate
```

Then to launch the API server:
```bash
./sippy serve \
  --log-level=debug \
  --database-dsn="postgresql://postgres:password@localhost:5432/postgres" \
  --google-service-account-credential-file ~/google-service-account-credential-file.json \
  --mode=ocp
````

If you'd like to launch just Component Readiness, you can run:

```
./sippy component-readiness \
    --google-service-account-credential-file ~/google-service-account-credential-file.json \
    --redis-url="redis://192.168.1.215:6379"
```

When providing BigQuery credentials (`--google-service-account-credential-file`), your service account or personal token needs access to the project and datasets that are being used.
The defaults are visible in `--help`. For component readiness, you need to have access to the storage API as well
with the permission `bigquery.readsessions.create`.

## Launch Sippy Web UI

If you are developing on the front-end, you may start a development server which will update automatically when you edit
files:

See [Sippy front-end docs](sippy-ng/README.md) for more details about developing on the front-end.

```bash
cd sippy-ng && npm start
```

## Caching

For particularly slow API's, such as those that need to fetch data from
BigQuery, Sippy supports using a redis key/value store by specifying
this argument on the command line:

```
--redis-url="redis://localhost:6379"
```

In development, you can start a Redis cache using Podman or Docker:

```
podman run --name sippy-redis -p 6379:6379 -d redis
```

## Run Sippy comment processing

If you want to run Sippy PR Commenting you likely want to first load data so that you have the PR commenting table populated.
See [Populating Data](DEVELOPMENT.md) in this document.  By default, comment processing dry run is set to true to prevent
unintended commenting on PRs, the code will run the full path up until the point PRs are modified due to commenting.
**Do Not** enable commenting unless you understand the PRs that will be affected and intend for that to happen.

Additionally, you will need a GITHUB_TOKEN environment variable to be configured both when loading the data and running 
the comment processing as described in [From GitHub](DEVELOPMENT.md) also within this document

```
./sippy-daemon \
  --database-dsn="postgresql://postgres:password@localhost:5432/postgres" \
  --google-service-account-credential-file ~/Downloads/openshift-ci-data-analysis-1b68cb387203.json \
  --comment-processing \
  --include-repo-commenting=origin
```

## Run E2E Tests

Sippy has a currently basic/minimal set of e2e tests which run a temporary postgres container, load the database with an
older release with fewer runs, launch the API, and run a few tests against it to verify things are working.
This runs as a presubmit on the repo, but developers can also run locally if they have a GCS service account JSON credential file.

```bash
GCS_SA_JSON_PATH=~/creds/openshift-ci-data-analysis.json make e2e
```

## Running the sippy e2e tests

The sippy e2e tests run in
[prow](https://prow.ci.openshift.org/job-history/gs/origin-ci-test/pr-logs/directory/pull-ci-openshift-sippy-main-e2e)
as part of CI using the [scripts](e2e-scripts) in this repo.

You can also run them locally using your own Kubernetes cluster.  These Kubernetes types have been tested:

* [Minikube](https://minikube.sigs.k8s.io/docs/) on MacOS and Linux
* [K3s](https://k3s.io/) on Linux
* [Redhat Openshift Local](https://developers.redhat.com/products/openshift-local/overview)

Setup you Kubernetes cluster, login, set context to your Kubernetes cluster, and run the
[run-e2e.sh](e2e-scripts/run-e2e.sh) script like this:

```
  SIPPY_IMAGE=quay.io/username/sippy GCS_CRED=/path/to/cred.json e2e-scripts/run-e2e.sh
```

Include and set the `DOCKERCONFIGJSON` variable appropriately if using a private container registry.

To skip the `docker build` and `docker push` steps, set `SKIP_BUILD=1' like this:

```
  SIPPY_IMAGE=quay.io/username/sippy GCS_CRED=/path/to/cred.json SKIP_BUILD=1 e2e-scripts/run-e2e.sh
```
