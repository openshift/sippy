---
paths:
  - "pkg/**/query/**"
---

* BigQuery and SQL query-building code should have inline comments explaining the
  purpose of each major query section (CTEs, JOINs, window functions, WHERE clauses).
* Abbreviations like CTE, matview, etc. should be expanded on first use.
* Functions that construct queries must stay under 200 lines; extract sub-queries or
  CTEs into helper functions when they grow beyond that.
