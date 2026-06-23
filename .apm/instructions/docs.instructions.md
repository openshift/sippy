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
* When your change affects a feature documented in `docs/features/`,
  update the relevant feature doc in the same PR. Feature docs describe
  purpose, data flow, and key code locations; keep them accurate as
  the implementation evolves. If you add a new major feature, create a
  new doc in `docs/features/`.
* Documentation and code belong in the same PR; never treat a docs update
  as a follow-up task.
* Do not use em dashes when writing docs. Use commas, parentheses, or periods instead.
