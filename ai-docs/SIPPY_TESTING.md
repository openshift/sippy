# Sippy - Testing Guide

> **Generic Testing Practices**: See [Tier 1 Testing Practices](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/testing) for test pyramid philosophy (60/30/10), E2E framework patterns, and mock strategies.

This guide covers **Sippy-specific** test organization and execution.

## Test Organization

```text
Test Type    | Location                | Count | Run Time | Purpose
-------------|-------------------------|-------|----------|----------------------------------
Unit (Go)    | pkg/**/*_test.go        | ~150  | ~30s     | Package-level logic
Unit (Jest)  | sippy-ng/src/**/*.test* | ~50   | ~10s     | React component tests
E2E          | test/e2e/               | ~20   | ~5min    | API endpoint validation (BigQuery)
```

**Pyramid ratio**: ~60% unit (Go + Jest), ~30% integration (minimal), ~10% E2E

## Unit Tests (Go)

### Location

Package tests: `pkg/[package]/*_test.go`

### Running

```bash
make test-unit          # All Go tests
go test ./pkg/...       # All packages
go test ./pkg/api/...   # Specific package
go test -v -run TestName ./pkg/api/  # Specific test
```

### Coverage

```bash
go test -coverprofile=coverage.out ./pkg/...
go tool cover -html=coverage.out
```

**Current coverage**: ~65% (goal: 70%)

### Sippy-Specific Patterns

**Variant extraction tests**: `pkg/variantregistry/registry_test.go`
- Test regex patterns against real job names
- Validate variant values

**Database tests**: `pkg/db/*_test.go`
- Use in-memory SQLite for speed
- Avoid real PostgreSQL for unit tests

**API handler tests**: `pkg/api/*_test.go`
- Mock database layer
- Test HTTP responses (status codes, JSON)

## Unit Tests (Jest)

### Location

React component tests: `sippy-ng/src/**/*.test.js`

### Running

```bash
cd sippy-ng
npm test                  # All Jest tests
npm test -- --coverage    # With coverage
```

### Sippy-Specific Patterns

**Component rendering**: Snapshot tests for UI components

**Data fetching**: Mock API responses using MSW (Mock Service Worker)

**Navigation**: Test React Router navigation logic

## E2E Tests

### ⚠️ CRITICAL WARNING

**NEVER run `make e2e` or `go test ./test/e2e/...` more than ONCE per request.**

**Why**: E2E tests query live BigQuery. Expensive. Slow (5+ minutes). Multiple runs waste quota.

**Process**:
1. Run **once**: `make e2e 2>&1 | tee e2e-test.log`
2. Read results: `less e2e-test.log`
3. Grep logs: `grep "FAIL" e2e-test.log`

**DO NOT re-run to grep for different strings. All output is in the log file.**

### Location

E2E tests: `test/e2e/`

### What E2E Tests Do

**Purpose**: Validate API endpoints against real BigQuery data

**Scope**:
- API response structure (JSON schema)
- Data integrity (pass rates, counts)
- Cross-endpoint consistency (jobs → tests → component readiness)

**NOT tested**:
- BigQuery query performance (assume BigQuery works)
- Data loader logic (unit tests cover this)

### Running E2E Tests

```bash
make e2e                          # Run all E2E tests (⚠️ once only)
go test -v ./test/e2e/...         # Same as make e2e
/sippy-dev-tests                  # MCP skill (lint + unit + e2e)
```

**Output**: `e2e-test.log` (persisted)

**Coverage**: `e2e-coverage.out` → `e2e-coverage.html`

### Sippy-Specific E2E Scenarios

| Test | Endpoint | Validates |
|------|----------|-----------|
| **Jobs API** | `/api/jobs?release=4.16` | Job list, filtering, variants |
| **Tests API** | `/api/tests?release=4.16` | Test results, pass rates |
| **Component Readiness** | `/api/componentreadiness?release=4.16` | Aggregations, regressions |
| **Variants** | `/api/variants` | Variant extraction accuracy |

## Test Data

### Local Development

**Recommended**: Use prod backup (see [../DEVELOPMENT.md](../DEVELOPMENT.md#from-a-prod-sippy-backup))

**Advantages**:
- Real data (no mocking)
- Fast setup (no BigQuery credentials)
- Offline development

### BigQuery (Production)

**When needed**:
- E2E tests
- Data loader development
- Variant snapshot updates

**Credentials**: `GOOGLE_APPLICATION_CREDENTIALS` environment variable

## Debugging Test Failures

### Unit Test Failures

**Go tests**:
```bash
go test -v -run TestName ./pkg/api/  # Verbose output
go test -race ./pkg/...               # Race detector
```

**Jest tests**:
```bash
cd sippy-ng
npm test -- --verbose                 # Verbose output
npm test -- --no-cache                # Clear cache
```

### E2E Test Failures

**Read log file**:
```bash
less e2e-test.log
grep "FAIL" e2e-test.log
grep "panic" e2e-test.log
```

**Common issues**:
- BigQuery credentials missing: Check `GOOGLE_APPLICATION_CREDENTIALS`
- Network timeout: BigQuery query too slow (check quota)
- Data mismatch: Release data changed (expected in live data)

## Coverage Targets

| Test Type | Current | Goal |
|-----------|---------|------|
| Go unit | ~65% | 70% |
| Jest | ~50% | 60% |
| E2E | N/A | API endpoints covered |

**Coverage commands**:
```bash
# Go coverage
make test-coverage
go tool cover -html=coverage.out

# Jest coverage
cd sippy-ng && npm test -- --coverage

# E2E coverage
make e2e  # Generates e2e-coverage.html
```

## Test Maintenance

### When to Update Tests

**Variant changes**: Update `pkg/variantregistry/registry_test.go`

**API changes**: Update E2E tests in `test/e2e/`

**Database schema changes**: Update DB tests in `pkg/db/`

**Frontend changes**: Update Jest tests in `sippy-ng/src/`

### Known Flaky Tests

**None currently documented.**

If you encounter flaky tests, document them here with reproduction steps.

## Component-Specific Notes

**E2E tests are expensive**: Never run more than once. See [warning above](#-critical-warning).

**Variant snapshot tests**: `TestVariantsSnapshot` fails if `pkg/variantregistry/snapshot.yaml` is out of date. Run `make update-variants` to fix.

**Database tests use SQLite**: Unit tests don't require PostgreSQL running.

**Jest tests use MSW**: API mocking via Mock Service Worker (see `sippy-ng/src/setupTests.js`).

## See Also

- [SIPPY_DEVELOPMENT.md](SIPPY_DEVELOPMENT.md) - Development workflows
- [architecture/components.md](architecture/components.md) - Sippy internals
- [../CLAUDE.md](../CLAUDE.md#testing) - Claude-specific testing rules
- [Tier 1 Testing Practices](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/testing)
