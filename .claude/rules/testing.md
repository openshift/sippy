---
paths:
  - "**/*_test.go"
---

* **Never run `make e2e` more than once per request.** E2e tests issue expensive BigQuery queries and take several minutes. Run `make e2e` only when explicitly asked, capture the output on that single run, and read the log file (`e2e-test.log`) for results. **Do not** re-run e2e just to grep for different things - all output is already in the log file.
* The same applies to `go test ./test/e2e/...` - never run it repeatedly.
* Use `go vet` and `go test` (for unit tests) to validate changes before resorting to a full e2e run.
