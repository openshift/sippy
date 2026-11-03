---
description: "Run the Component Readiness Playwright BDD suite"
---

# Sippy dev: Component Readiness BDD

Run the browser suite from the repo root:

```bash
make test-bdd
```

That installs Playwright's Chromium and runs `npm run test:bdd` in `sippy-ng`. If a dev server is already on port 3000, the suite reuses it. Do not stop that server first. A run where `CI` is already set starts its own server and will not reuse one.

Report the pass/fail summary. If the run fails, include the failing scenario names and the error text. Do not rerun unless asked.
