---
description: "How Sippy configuration files work, especially release/job config"
applyTo: "config/**"
---

### Configuration files

The `config/` directory contains YAML files that define which CI jobs Sippy tracks for each release.

#### `config/openshift.yaml` — generated, do not edit

This file is **generated** by `sippy-config-generator` from [openshift/ci-tools](https://github.com/openshift/ci-tools/) and will be overwritten. Manual edits will be lost.

The generator reads Prow job definitions and release controller configuration from the [openshift/release](https://github.com/openshift/release/) repo to produce the full list of jobs per OCP release. For periodic jobs to appear here, they must:

1. Have the release version in their job name (e.g., `4.18`).
2. Have a `job-release` label set via the `release` configuration option.
3. Be allowlisted in the ci-tools configuration.

See [Configuration for periodic jobs](https://docs.ci.openshift.org/how-tos/naming-your-ci-jobs/#configuration-for-periodic-jobs) for details.

#### `config/openshift-customizations.yaml` — manually maintained

This file is an **overlay** applied on top of the generated config. Use it to:

* Add **pseudoreleases** (e.g., `Presubmits`, `aro-integration`, `aro-stage`).
* Add jobs that are **not** part of an OCP release (and therefore not picked up by the generator).
* **Exclude** a generated job by setting it to `false`.
* Configure `componentReadiness` overrides (e.g., `variantJunitTableOverrides`).

**Adding a new release** requires two steps beyond the config change:

1. The release must be registered in the Sippy central database — config alone is not enough.
2. A Jira ticket must be filed in the **TRT** project with the release details (name, product, etc.) so the team can set it up.

#### Other config files

* `config/views.yaml` / `config/qe-views.yaml` / `config/seed-views.yaml` — Component Readiness view definitions.
* `config/e2e-openshift.yaml` — configuration used during e2e testing.
