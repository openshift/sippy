## Building Sippy

Running `make` will build an all-in-one binary that contains both the go app and frontend.

To build just the backend, run `mkdir -p sippy-ng/build; touch sippy-ng/build/index.html; go build -mod=vendor .`.
Note you have to `mkdir sippy-ng/build` and create a file in it first, otherwise you will get an error like
`main.go:36:12: pattern sippy-ng/build: no matching files found`.

## Create PostgreSQL Database

Launch postgresql:

```bash
podman run --name sippy-postgres -e POSTGRES_PASSWORD=password -p 5432:5432 -d quay.io/enterprisedb/postgresql
```

Sippy will manage it's own schema on startup via gorm automigrations and some custom code for managing materialized view
definitions and functions.

## Populating Data

Sippy obtains data from multiple sources:

| Data Source         | Job names | Job runs | Test results | Release tags | Build cluster | PR State |
|---------------------|-----------|----------|--------------|--------------|---------------|----------|
| TestGrid            | X         | X        | X            |              |               |          |
| Prow                |           | X        |              |              | X             |          |
| Release controller  |           |          |              | X            |               |          |
| Sippy configuration | X         |          |              |              |               |          |
| GCS Storage Buckets |           |          | X            |              |               |          |
| GitHub              |           |          |              |              |               | X        |

### From a Prod Sippy Backup

The simplest way to fetch data for Sippy, is to just get a copy of the production database. See the TRT team drive in
Google, periodically there is a gzip'd backup stored there. Restore it locally with:

```bash
psql -h localhost -U postgres -p 5432 postgres < sippy-backup-2022-05-02.sql
```

### From TestGrid

Fetch data from TestGrid, and load it into the db. This process can take some time for recent OpenShift releases with
substantial number of jobs. Using older releases like 4.7 can be fetched and loaded in just a couple minutes.

TestGrid data is loaded into the database in two steps. It is first downloaded and stored in a local data directory, and
then loaded into the DB.

To download TestGrid data:

```bash
./sippy --local-data /opt/sippy-testdata \
  --release 4.11 \
  --fetch-data /opt/sippy-testdata \
  --log-level=debug
```

To load the database:

```bash
./sippy --local-data /opt/sippy-testdata \
  --release 4.11 \
  --load-database \
  --log-level=debug \
  --database-dsn="postgresql://postgres:password@localhost:5432/postgres" \
````

### From Prow and GCS buckets

Fetching data from prow directly gives us access to more raw data that TestGrid doesn't have, such as build cluster
data, raw junit files, test durations, and more -- but it requires more configuration that using TestGrid.

In order to access the GCS storage buckets where the raw junit data is stored, you need to provide Sippy with a google
service account credential. See the [official google docs](https://cloud.google.com/iam/docs/service-accounts) for
information on how to get one.

Additionally, you need a configuration file that maps job names to releases. One should already be available in
[./config/openshift.yaml](config/openshift.yaml). Information about how to generate this file is
available [here](config/README.md).

```bash
./sippy --load-database \
  --load-prow=true \
  --load-testgrid=false \
  --release 4.11 \
  --database-dsn="postgresql://postgres:password@localhost:5432/postgres" \
  --mode=ocp \
  --config ./config/openshift.yaml \
  --google-service-account-credential-file ~/Downloads/openshift-ci-data-analysis-1b68cb387203.json
```

### From GitHub

When using Prow in GitHub mode, it's possible to sync additional data from GitHub including PR state. GitHub throttles
unauthenticated requests, limited to 60 an hour. If you don't need this in development, then simply omit `--load-github`
, otherwise set `GITHUB_TOKEN` environment variable,
or [configure GitHub in your gitconfig](https://stackoverflow.com/questions/8505335/hiding-github-token-in-gitconfig).

```bash
./sippy --load-database \
  --load-prow=true \
  --load-github=true \
  --load-testgrid=false \
  --release 4.11 \
  --database-dsn="postgresql://postgres:password@localhost:5432/postgres" \
  --mode=ocp \
  --config ./config/openshift.yaml \
  --google-service-account-credential-file ~/Downloads/openshift-ci-data-analysis-1b68cb387203.json
```

### From the release controller

Sippy retrieves release-related data from release controllers. In order to fetch release data, give Sippy a list of
releases and architectures like this:

```
./sippy --load-database \
  --load-prow=false \
  --load-testgrid=false \
  --arch amd64 \
  --arch arm64 \
  --release 4.12 \
  --release 4.11 \
  --database-dsn="postgresql://postgres:password@localhost:5432/postgres" \
  --google-service-account-credential-file ~/Downloads/openshift-ci-data-analysis-1b68cb387203.json \
  --mode=ocp \
  --config ./config/openshift.yaml
```

## Launch Sippy API

```bash
./sippy --server \
  --release 4.11 \
  --log-level=debug \
  --database-dsn="postgresql://postgres:password@localhost:5432/postgres" \
  --mode=ocp
````

## Launch Sippy Web UI

If you are developing on the front-end, you may start a development server which will update automatically when you edit
files:

See [Sippy front-end docs](sippy-ng/README.md]) for more details about developing on the front-end.

```bash
cd sippy-ng && npm start
```
