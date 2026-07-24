---
paths:
  - "**/*.go"
---

* When adding or updating APIs, **use HATEOAS** in responses to support discoverability and consistent client interaction.
* Follow idiomatic Go practices.
* Use `k8s.io/apimachinery/pkg/util/sets` (e.g. `sets.New[string]()`) to deduplicate or collect 
  unique strings. Do not use `map[string]bool` as a hand-rolled set.
* Prefer structured logging where it makes sense; particularly for names and IDs, and often for
  counts, a log.WithField() call is preferred over formatting values into a string.
* After making changes, always run `gofmt -w` on modified files to ensure proper formatting.
* When modifying any data provider (BigQuery or PostgreSQL), ensure **parity between both implementations**. Changes to query logic, filtering, or returned data in one provider must be reflected in the other.
