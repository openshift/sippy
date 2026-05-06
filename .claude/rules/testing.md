---
paths:
  - "**/*_test.go"
---

* Unit tests (`make test`, `go test ./pkg/...`) and e2e tests with the postgres provider are cheap — run them freely to validate changes.
* Use `go vet` and `go test` (for unit tests) to validate changes before resorting to a full e2e run.
