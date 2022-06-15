## Create PostgreSQL Database

Launch postgresql:

```bash
podman run --name sippy-postgres -e POSTGRES_PASSWORD=password -p 5432:5432 -d quay.io/enterprisedb/postgresql
```

Sippy will manage it's own schema on startup via gorm automigrations and some custom code for managing materialized view definitions and functions.

TODO: podman run postgres as a regular user may require disabling selinux on Fedora 35. Needs investigation.

## Populating Data

### From Scratch

Fetch data from testgrid, and load it into the db. This process can take some time for recent OpenShift releases with substantial number of jobs. Using older releases like 4.7 can be fetched and loaded in just a couple minutes.

```bash
go build -mod=vendor . && ./sippy --local-data /opt/sippy-testdata --release 4.11 --fetch-data /opt/sippy-testdata --log-level=debug
go build -mod=vendor . && ./sippy --release 4.11 --local-data /opt/sippy-testdata --load-database --log-level=debug --database-dsn="postgresql://postgres:password@localhost:5432/postgres" --skip-bug-lookup
````

### From a Prod Sippy Backup

See the TRT team drive in Google, periodically there is a gzip'd backup stored there. Restore it locally with:

```bash
psql -h localhost -U postgres -p 5432 postgres < sippy-backup-2022-05-02.sql
```

## Launch Sippy API

```bash
go build -mod=vendor . && ./sippy --server --local-data /opt/sippy-testdata --release 4.11 --log-level=debug --skip-bug-lookup --database-dsn="postgresql://postgres:password@localhost:5432/postgres" --db-only-mode
````

## Launch Sippy Web UI

```bash
cd sippy-ng && npm start
```

