# AGENTS.md

This file provides guidance for working with code in this repository,
for use by coding assistants.

## Repository Overview

**Sippy (CIPI – Continuous Integration Private Investigator)** is a tool
used within the OpenShift engineering organization to analyze CI job
results. Its primary goals are to:

* Provide insights into job and test statistics.
* Monitor release health and detect regressions.
* Support release management decisions through statistical analysis (e.g., Component Readiness).

The system consists of:

* A **Go-based API backend**.
* A **React/Material-UI frontend** (located in `sippy-ng`).
* Data sources including **PostgreSQL**, and **BigQuery**

## Backend Guidelines

* Written in **Golang**.
* When adding or updating APIs, **use HATEOAS** in responses to support discoverability and consistent client interaction.
* Follow idiomatic Go practices.

## Frontend Guidelines

* The frontend code lives entirely under the `sippy-ng` directory.
* After making changes, always run formatting and linting to maintain consistency:

```bash
npx eslint . --fix
npx prettier --write .
```

* Prefer functional components and React hooks over class components.
* Keep UI elements consistent with Material-UI standards.

### Making API Calls

* **Always use `apiFetch()` from `helpers.js` instead of direct `fetch()` calls** when making API requests to Sippy's backend.
* `apiFetch()` automatically prepends `REACT_APP_API_URL` to relative URLs, making the code cleaner and handling development/production environments correctly.

```javascript
// ✅ Good - uses apiFetch
import { apiFetch } from '../helpers'

apiFetch('/api/releases')
  .then(response => response.json())
  .then(data => console.log(data))

// ❌ Bad - manual concatenation
fetch(process.env.REACT_APP_API_URL + '/api/releases')
  .then(response => response.json())
  .then(data => console.log(data))
```

## General Notes

* Favor clarity and maintainability over cleverness.
* There should not be an excessive amount of comments. They should helpful and explain non-obvious details. Explain the "why" and not the "what".
