
## One Time Setup

Install goose for db migrations:

```
$ go install github.com/pressly/goose/v3/cmd/goose@latest
```

## Create and Populate PostgreSQL Database

Launch postgresql:

```bash
podman run --name sippy-postgres -e POSTGRES_PASSWORD=password -p 5432:5432 -d quay.io/enterprisedb/postgresql
```

TODO: podman run postgres as a regular user may require disabling selinux on Fedora 35. Needs investigation.

## Populating Data

### From Scratch

Populate db schema with goose, may need to run periodically if new migrations come in, fetch data from testgrid, and load it into the db. This process can take some time for recent OpenShift releases with substantial number of jobs. Using older releases like 4.7 can be fetched and loaded in just a couple minutes.

```bash
goose --dir dbmigration postgres "user=postgres password=password dbname=postgres sslmode=disable" up
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

## Modifying Database Schema

```bash
goose -dir dbmigration create test
```

Edit the resulting file and add your schema to migrate "up" to, and "down" from.

In the case of materialized views I believe you will need to drop the old view, recreate with the entire
new schema. We should maintain the old schema in the "down" section of the migration to
be able to roll back if ever needed. This will be verbose, but I believe correct.

We do not issue a refresh after matview changes as this would run on every change, and they can be very slow.
Normally this is handled in the background refresh run when we start the API.


