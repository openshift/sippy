# Sippy - Agentic Documentation

**Component**: Sippy (CIPI - Continuous Integration Private Investigator)  
**Repository**: openshift/sippy  
**Documentation Tier**: 2 (Component-specific)

## CRITICAL: Retrieval Strategy

**IMPORTANT**: Prefer retrieval-led reasoning over pre-training-led reasoning.

When working on Sippy:
- ✅ **DO**: Read relevant docs from `./ai-docs/` first
- ✅ **DO**: Verify patterns match current implementation
- ❌ **DON'T**: Rely solely on training data
- ❌ **DON'T**: Guess at API structures or data models

> **Generic Platform Patterns**: See [Tier 1 Ecosystem Hub](https://github.com/openshift/enhancements/tree/master/ai-docs) for operator patterns, testing practices, security guidelines, and cross-repo ADRs.

## What is Sippy?

CI analysis tool for OpenShift. Analyzes Prow job results from BigQuery, provides statistical insights on job/test health, regression detection, and component readiness tracking.

**Key Principle**: Data-driven release management through statistical analysis of CI signal.

## Core Components

- **Backend**: Go HTTP API server (sippyserver) | **Frontend**: React dashboard (sippy-ng) | **Data Layer**: BigQuery + PostgreSQL + Redis

**Quick Start**: See [SIPPY_DEVELOPMENT.md](SIPPY_DEVELOPMENT.md)

## Documentation Structure

```text
ai-docs/
├── domain/                    # CI concepts (jobs, tests, variants, releases)
├── architecture/              # Sippy internals (backend, frontend, data pipeline)
├── decisions/                 # Component-specific ADRs
├── exec-plans/                # Feature planning
├── references/
│   └── ecosystem.md           # Links to Tier 1
├── SIPPY_DEVELOPMENT.md       # Development workflows
└── SIPPY_TESTING.md           # Test suites
```

**Exec-Plans**: Use `active/` for new features. See [Tier 1 Exec-Plans Guide](https://github.com/openshift/enhancements/tree/master/ai-docs/workflows/exec-plans).

**Platform Patterns (Tier 1)**: [Testing](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/testing) | [Security](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/security) | [Development](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/development)

## Knowledge Graph

```text
                         [AGENTS.md] ← Start here
                              │
              ┌───────────────┼───────────────┐
              │               │               │
         [domain/]      [architecture/]  [decisions/]
        CI concepts     Sippy internals  ADR history
              │               │               │
              └───────────────┼───────────────┘
                              │
                      [references/ecosystem]
                      Links to Tier 1
```

**AI Agent Path**: domain/ → architecture/ → SIPPY_DEVELOPMENT.md → SIPPY_TESTING.md

## Domain Concepts (CI Analysis)

| Concept | Description |
|---------|-------------|
| **Job** | Prow CI job execution (e.g., periodic-ci-openshift-release-...) |
| **Test** | Individual test case within a job |
| **Variant** | NURP+ dimension (Network, Upgrade, Release, Platform, etc.) |
| **Release** | OpenShift version (e.g., 4.15, 4.16) |
| **Component Readiness** | Statistical analysis of component health |
| **Regression** | Identified decrease in pass rate |

## Architecture Layers

| Layer | Technology | Purpose |
|-------|-----------|---------|
| **Data Source** | BigQuery | Prow CI job results |
| **Data Loader** | Go (dataloader) | BigQuery → PostgreSQL ETL |
| **Cache** | Redis | Query result caching |
| **Backend** | Go HTTP API | REST endpoints |
| **Frontend** | React + Material-UI | Dashboard UI |

## External References

- [API Documentation](../pkg/api/README.md) | [Frontend Docs](../sippy-ng/README.md) | [Development Guide](../DEVELOPMENT.md)

---

**Tier 1 Hub**: https://github.com/openshift/enhancements/tree/master/ai-docs
