---
paths:
  - "**/*.go"
---

* Follow idiomatic Go practices.
* Choose names carefully using concise but appropriately descriptive words; in the scope of a
  package, a name need only describe its function relative to that package. Provide docstring for
  every package-level name explaining _why_ it exists and describing any parameter whose purpose is
  not completely obvious from name and context.
* Keep packages, structs, and methods focused on a single clear conceptual "chunk":
  a package should represent one cohesive concept, a struct should represent a single entity
  in the scope of its package, and a method should operate on a single level of abstraction.
  Prefer descriptive names that evoke a specific concept (e.g., "TestResultAggregator" not
  "Manager"). Avoid generic names like "Manager", "Handler", "Util" without specific context,
  and method names that don't indicate what they do (e.g., "Process", "Handle", "Do").
  Structs with more than about 7 top-level fields should be refactored into focused sub-types.
* When adding or updating APIs, **use HATEOAS** in responses to support discoverability and consistent client interaction.
* Use `k8s.io/apimachinery/pkg/util/sets` (e.g. `sets.New[string]()`) to deduplicate or collect 
  unique strings. Do not use `map[string]bool` as a hand-rolled set.
* Prefer structured logging where it makes sense; particularly for names and IDs, and often for
  counts, a log.WithField() call is preferred over formatting values into a string.
* When modifying any data provider (BigQuery or PostgreSQL), ensure **parity between both implementations**. Changes to query logic, filtering, or returned data in one provider must be reflected in the other.
* **Timestamps and dates**: Use proper types, never epoch integers.
  - PostgreSQL columns: use `TIMESTAMP WITH TIME ZONE` for timestamps and `DATE` for date-only values. All timestamps are UTC.
  - Go structs: use `time.Time` for timestamps. Use `civil.Date` (`cloud.google.com/go/civil`) for date-only values (e.g., GA dates, development start dates). Do not use `string` for date or timestamp fields; let JSON marshaling produce the correct format.
  - API responses: timestamps serialize as RFC 3339 strings, dates as `YYYY-MM-DD` strings. Never return epoch millisecond integers. Never manually format with `time.Format()` into a string field.
  - Materialized views: project timestamp columns directly from the source table. Do not convert to epoch with `EXTRACT(epoch FROM ...)`.
  - GORM model tags: include `gorm:"type:date"` on date-only columns so GORM and migrations use the correct PostgreSQL type.
* Check `pkg/util/` for existing helper functions before adding inline utility logic.
  Avoid calling the same utility function multiple times with identical arguments in the
  same code path.
* Never ignore returned errors as `_` without clear justification. Errors should be wrapped
  with context using `fmt.Errorf` with `%w`. Avoid `panic()` except in `init()` or fatal
  conditions. Check for nil before dereferencing pointers.
* **Never** concatenate or format SQL queries with values directly from user input. Always use
  placeholders for parameters in queries, preferably named (`@Name`).
* Structs used with GORM that have fields not backed by database columns (such as computed or
  API-only fields) must include the `gorm:"-"` tag to explicitly exclude them from GORM operations.
  BigQuery struct fields must have `bigquery:"column_name"` tags that exactly match the BigQuery
  query result schema names, whether from table columns or aliases (give computed fields aliases).
* After making changes, always run `gofmt -w` on modified files to ensure proper formatting.
