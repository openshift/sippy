---
description: "Guidelines for BigQuery and SQL query-building code"
applyTo: "pkg/**/query/**"
---

* Abbreviations should be expanded on first use, for example Common Table Expression (CTE) and
  Materialized View (matview). "SQL" is exempt from this rule.
* BigQuery and SQL query-building code should have inline comments explaining the
  purpose of each major query section (CTEs, JOINs, window functions, WHERE clauses).
* Functions that construct queries must stay under 200 lines; extract sub-queries or
  CTEs into helper functions when they grow beyond that.
* When constructing SQL queries, prefer using a format string and/or multi-line string rather than
  concatenation of short strings; and always use placeholders for parameters.
  
