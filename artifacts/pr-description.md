TRT-3016: Archive old resolved GA triages in listings

Default triage listings now exclude triages resolved strictly more than 90 elapsed days ago when every associated regression release has reached its known GA date. Eligibility is derived at query time across all regressions, including associations outside the requested view. Missing or deleted release metadata and missing or future GA dates keep the entire triage visible.

`include_archived=true` provides historical REST access while retaining view and field filters. Reopening or adding a non-GA association automatically restores ordinary visibility. Records, associations, CRUD access, and historical potential-match access are preserved. API documentation describes the option and caller audit.

Coverage includes the exact 90-day boundary, unresolved triages, GA and mixed-release associations, missing metadata, reactivation, preservation, cross-release view reuse, REST defaults and historical access, field selection, and archived-record updates.

Validation:
- `make test`: passed with writable caches/artifact paths and local-listener access, including Go (16,921 tests, 32 skipped), frontend, and MCP (53 passed). Initial sandbox-only attempt failed because HTTP tests could not open local listeners.
- `go vet ./pkg/...`: passed.
- Go lint: 0 issues; ESLint: passed. Full `make lint` fails in audit-ci with "Cannot convert undefined or null to object".
- Integration test compiles but cannot start: PostgreSQL container runtime socket unavailable.
- Updated e2e test package compiles (`go test ... -run '^$'`). Full e2e execution blocked by unavailable PostgreSQL/services.
- `git diff --check`: passed.

Scope is medium (shared query semantics, REST propagation, cross-release edge cases). Implementation, serial review with corrections, and available validation were performed directly because the named workflow skills were unavailable. No push or PR creation performed; pipeline handles publication. Representative production query-plan validation remains the TRT-3053 follow-up; no indexes or persisted state added.
