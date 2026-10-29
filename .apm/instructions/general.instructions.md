---
description: "Sippy repository overview and general coding guidelines"
applyTo: "**"
---

### APM context generation

Sources under **`.apm/`** (instructions, prompts, `apm.yml`, etc.) drive generated agent context. After editing those files, run **`make apm`** to regenerate **AGENTS.md**, **CLAUDE.md**, **GEMINI.md**, and the integrated copies under **`.claude/`**, **`.cursor/`**, **`.gemini/`**, and **`.opencode/`**. CI enforces freshness with **`make verify-apm`**.

**Slash / agent commands:** content under **`.apm/skills/*/SKILL.md`** is the single source of truth. Each matching **`.apm/prompts/*.prompt.md`** file is a thin wrapper that preserves slash-command integration while pointing to its skill. **`apm install`** (part of **`make apm`**) deploys the skills and wrappers to editor targets (e.g. **`.claude/commands/`**, **`.claude/skills/`**, **`.agents/skills/`**). Do not add those generated paths by hand or installs will skip them as unmanaged duplicates.

**Codex repository skills:** APM deploys the native skill bundle under **`.apm/skills/`** to **`.agents/skills/`** for Codex. Do not edit the deployed skills directly.

**Sippy (CIPI - Continuous Integration Private Investigator)** is a tool used within the OpenShift engineering organization to analyze CI job results. Its primary goals are to:

* Provide insights into job and test statistics.
* Monitor release health and detect regressions.
* Support release management decisions through statistical analysis (e.g., Component Readiness).

The system consists of:

* A **Go-based API backend**.
* A **React/Material-UI frontend** (located in `sippy-ng`).
* Data sources including **PostgreSQL**, and **BigQuery**
* A **headless daemon** for asynchronous processing

Favor clarity and maintainability over cleverness. Comments should be minimal, helpful, and explain the "why" not the "what".
