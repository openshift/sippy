---
description: "React/Material-UI frontend guidelines for Sippy"
applyTo: "sippy-ng/**"
---

* After making changes, always run formatting and linting to maintain consistency:

```bash
npx eslint . --fix
npx prettier --write .
```

* Prefer functional components and React hooks over class components.
* Keep UI elements consistent with Material-UI standards.

The frontend uses `npm`. If you must install or update any dependencies, always use the `--ignore-scripts` flag.
