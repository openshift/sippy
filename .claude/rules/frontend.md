---
paths:
  - "sippy-ng/**"
---

* The frontend uses `npm`. If you must install or update any dependencies, always use the `--ignore-scripts` flag.
* After making changes, always run formatting and linting to maintain consistency:

  ```bash
  npx eslint . --fix
  npx prettier --write .
  ```

* Prefer functional components and React hooks over class components.
* Keep UI elements consistent with Material-UI standards.
* Avoid nested ternary expressions in JSX. When there are more than two
  branches, use if/else if chains, early returns, or a lookup object instead.
  A single ternary is fine; nesting ternaries makes code hard to follow.
* Before adding date/time formatting, duration calculations, or string
  utilities inline, check `sippy-ng/src/helpers.js` for existing functions
  like `relativeDuration`, `safeEncodeURIComponent`, etc. Prefer reusing
  existing helpers over reimplementing similar logic.
* React components with extensive inline CSS should use the `useStyles` pattern.
  Inline style objects with more than 3-4 properties should be extracted to
  `useStyles()` or styled components. Inline styles are acceptable for simple,
  dynamic values (e.g., width based on props).
* **Timestamps and dates from the API**:
  - Timestamps arrive as RFC 3339 strings (e.g., `"2024-06-27T15:30:00Z"`), not epoch millisecond integers. Use `new Date(value)` or `Temporal.Instant.from(value)` to parse them.
  - Dates arrive as `YYYY-MM-DD` strings (e.g., `"2024-06-27"`). Use `Temporal.PlainDate.from(value)` for date arithmetic.
  - For MUI DataGrid timestamp columns, use `type: 'date'` with a `valueGetter` that returns a `Date` object. Do not return epoch milliseconds from `valueGetter`.
  - For filter values sent to the API, use ISO 8601 strings (e.g., `new Date(...).toISOString()`), not epoch millisecond integers.
  - For day-level bucketing or date arithmetic, prefer `Temporal.PlainDate` over `Date` with manual millisecond math.
* Non-trivial modifications to frontend logic must include unit test coverage.
  If a function is hard to test, consider refactoring to separate pure logic
  from side effects. 
