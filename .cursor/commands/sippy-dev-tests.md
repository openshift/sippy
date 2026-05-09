---
description: Run lint, unit tests, and e2e in order (full local CI suite)
---

# Sippy dev — full test suite

Run these three steps in order. Stop if any step fails.

1. **Lint** — run directly:

   ```bash
   make lint
   ```

2. **Unit tests** — run directly:

   ```bash
   make test
   ```

3. **E2e** — run directly:

   ```bash
   make e2e
   ```

   Works both on the host (starts its own PostgreSQL/Redis containers via Podman) and inside the devcontainer (creates a temporary `sippy_e2e` database on the existing PostgreSQL).