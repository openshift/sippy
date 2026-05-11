---
description: "Keep documentation up-to-date alongside code changes"
applyTo: "**"
---

* When your change alters setup steps, usage instructions, or architecture,
  update the relevant README (root `README.md`, `sippy-ng/README.md`,
  `mcp/README.md`, etc.) in the same PR.
* When API endpoints are added, removed, or modified, update
  `pkg/api/README.md` (or create it if it doesn't exist).
* When configuration options, environment variables, or CLI flags change,
  update `config/README.md` or the relevant section of the root `README.md`.
* When new conventions, workflows, or tooling are introduced, consider
  adding or updating an `.apm/instructions/` file so that AI coding
  assistants stay aligned with the project's practices.
* Documentation and code belong in the same PR — never treat a docs update
  as a follow-up task.
