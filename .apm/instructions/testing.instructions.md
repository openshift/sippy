---
description: "Testing guidelines and constraints for Sippy"
applyTo: "**/*_test.go"
---

* Use `go vet` and `go test ./pkg/...` to validate changes before resorting to a full e2e run.
* Run `make e2e 2>&1 | tee e2e-test.log` to verify your changes don't break end-to-end tests. You **MUST** read the log file (`e2e-test.log`) for results. Do not re-run e2e just to grep for different things — all output is already in the log file.
