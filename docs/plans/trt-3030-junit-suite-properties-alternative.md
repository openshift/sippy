**TRT-3030 alternative: additive JUnit suite-selection properties**

**Status:** Proposed alternative. Implementation has not started.

**Jira:** [TRT-3030](https://redhat.atlassian.net/browse/TRT-3030)

This is a separate design from [the suite-name replacement plan](trt-3030-junit-suite-names.md). It keeps existing JUnit testsuite names and adds execution metadata through standard JUnit properties. The original plan remains unchanged; the two documents describe alternatives, not successive implementation phases.

**Problem and recommended approach**

TRT-3030 needs visibility into which origin/OTE suites a job executes, including the proposed candidate, active, stable, and minimal suites. Jobs can execute multiple suites, such as upgrade followed by conformance. The metadata must describe the execution observed at the time of the run, so that later suite promotion or shard rebalancing does not rewrite history.

Retain testsuite.name as openshift-tests for ordinary origin execution and openshift-tests-upgrade for upgrade execution. Add a suite-level selected-suite property describing the actual invocation. Carry that metadata into Sippy as a new test-execution attribute. Extending ci-cloud-functions and BigQuery to capture it is optional, pending additional requirements gathering. Changes to ci-tools aggregation are also optional, depending on whether consumers need the metadata in aggregated results.

The stable identity inputs stay intact. Both ci-test-mapping and the cloud JUnit parser use the current suite name in ID generation. Keeping that name avoids introducing compatibility namespaces, changing mapping keys, or creating aliases for every new lifecycle suite and shard. Preserve testcase names and existing rename handling as well.

Sources: [stable ownership IDs](https://github.com/openshift-eng/ci-test-mapping/blob/21407df0b29d5638cadeaed8e9f75f84b01fe6b5/pkg/util/tests.go#L65); [raw BigQuery test IDs](https://github.com/openshift-eng/ci-cloud-functions/blob/b8380a4cb5397514ab690f8a1d679a9d6cf10719/ci-to-bigquery/gcs_finalize_event.py#L1313).

This alternative changes the storage interpretation proposed in the Jira: existing suite/suite_id values continue describing the legacy JUnit suite. Queries for the actual selected execution suite use new metadata fields or relations.

**Review baseline**

The source assessment uses the 2026-11-10 snapshots below. Links are pinned to those revisions; implementation should recheck affected code against the then-current branches.

| Repository | Reviewed commit |
| --- | --- |
| openshift/origin | [b019a84de55c1b8fa8cdb91eef194217ad24b7b7](https://github.com/openshift/origin/commit/b019a84de55c1b8fa8cdb91eef194217ad24b7b7) |
| openshift/sippy | [bfb72a3c644162c6ddfd0d333c939cd978bd2796](https://github.com/openshift/sippy/commit/bfb72a3c644162c6ddfd0d333c939cd978bd2796) |
| openshift-eng/ci-test-mapping | [21407df0b29d5638cadeaed8e9f75f84b01fe6b5](https://github.com/openshift-eng/ci-test-mapping/commit/21407df0b29d5638cadeaed8e9f75f84b01fe6b5) |
| openshift/ci-tools | [d5d01ecf301b98438187bae06654550f9d389a1a](https://github.com/openshift/ci-tools/commit/d5d01ecf301b98438187bae06654550f9d389a1a) |
| openshift-eng/ci-cloud-functions | [b8380a4cb5397514ab690f8a1d679a9d6cf10719](https://github.com/openshift-eng/ci-cloud-functions/commit/b8380a4cb5397514ab690f8a1d679a9d6cf10719) |

The test-suite [enhancement PR 2080](https://github.com/openshift/enhancements/pull/2080) was open at revision [371956b9b12099e00f4140894285cec26374d6eb](https://github.com/openshift/enhancements/blob/371956b9b12099e00f4140894285cec26374d6eb/enhancements/testing/test-suites.md). Its lifecycle and shard examples are proposed behavior, not a claim that the suite registry already contains them.

**Property contract**

Use one resolved execution context to generate these properties:

| Property | Meaning | Example |
| --- | --- | --- |
| openshift-tests:selected-suite | Exact resolved suite name selected for this invocation | openshift/conformance/parallel/active-02 |
| openshift-tests:suite-stage | Lifecycle stage of the selected execution suite, when applicable | active |
| openshift-tests:shard | Shard identifier within that suite family, mode, and stage, when applicable | 02 |
| openshift-tests:command | Origin execution command, which disambiguates generic selections | run or run-upgrade |

Example:

~~~xml
<testsuite name="openshift-tests" tests="1" skipped="0"
           failures="0" time="1">
  <properties>
    <property name="openshift-tests:selected-suite"
              value="openshift/conformance/parallel/active-02"/>
    <property name="openshift-tests:suite-stage" value="active"/>
    <property name="openshift-tests:shard" value="02"/>
    <property name="openshift-tests:command" value="run"/>
  </properties>
  <testcase name="[sig-example] existing test name" time="1"/>
</testsuite>
~~~

The example illustrates the reporting contract, not new lifecycle suite registration or a change to test selection.

selected-suite means the exact registered or resolved invocation name. It does not mean just candidate, active, stable, or minimal. Those values belong in suite-stage. Retaining the full selection avoids reconstructing a canonical name from separate fields and supports dedicated and extension-defined suites with their own naming conventions.

Expose shard separately for convenient filtering and reporting even when it appears in the exact selected-suite string. Store it as a string so identifiers such as 02 are preserved. A shard value is not globally unique and must be interpreted with its suite family, execution mode, and stage. Omit stage and shard when they do not apply; do not assign a default stage to a dedicated suite or an older result.

The enhancement explicitly embeds shard identifiers in registered suite names and discusses rebalancing by changing those names. It does not define a JUnit reporting schema or require shard information to be encoded only in one reporting field. The separate shard property is a reporting convenience, not a replacement for the enhancement's suite registry or membership model. See [Declaring suites](https://github.com/openshift/enhancements/blob/371956b9b12099e00f4140894285cec26374d6eb/enhancements/testing/test-suites.md#declaring-suites) and [Shards](https://github.com/openshift/enhancements/blob/371956b9b12099e00f4140894285cec26374d6eb/enhancements/testing/test-suites.md#shards).

Generate all properties from the same resolved suite context. Prefer explicit suite metadata when the lifecycle-suite implementation supplies it. If classification initially uses names, restrict it to the registered lifecycle naming convention and cover it with tests; do not infer a stage or shard from an arbitrary extension name ending in a number. selected-suite remains authoritative, and missing or inconsistent structured fields must not be silently invented.

No identity-suite compatibility property or identity_suite database column is needed in this alternative.

**Execution context versus test membership**

The suite property describes the suite that origin selected. Loaders can associate it with every testcase emitted for that execution, including monitor and synthetic cases, as provenance.

That does not prove that each testcase is declared as a member of that lifecycle stage. An umbrella selection can include multiple child suites, and monitor tests can accompany several selections. If reporting declared per-test membership is later required, introduce a separate testcase-level contract sourced from resolved membership metadata. Do not use a suite-level property to claim that membership.

OTE's existing lifecycle attribute continues to mean blocking or informing. suite-stage is a separate field; do not overload lifecycle.

The value is attached to an observation, not the static tests row or test ownership record. A test's selected execution suite can change between runs without changing its identity.

**Origin implementation**

1. Resolve the reporting context after suite selection, using Suite.Name and the command mode already known at the entry point. Keep the hardcoded legacy JUnit suite names passed by the current ordinary and upgrade commands. Sources: [ordinary runner](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/cmd/openshift-tests/run/options.go#L95); [upgrade runner](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/cmd/openshift-tests/run-upgrade/options.go#L121).

2. Extend report generation to accept this context and append its properties alongside TestVersion. Preserve testcase names, statuses, timing, source attributes, retry/flake representation, and OTE lifecycle values. Origin's [JUnit types](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/test/ginkgo/junitapi/types.go#L22) already support standard suite properties; the [main report generator](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/test/ginkgo/junit.go#L69) already writes TestVersion through them.

3. Pass the same context into monitor serialization. Main and monitor JUnit files for one invocation must identify the same selection. The monitor writer currently initializes Properties to nil and needs explicit propagation. Sources: [monitor call](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/test/ginkgo/cmd_runsuite.go#L858); [monitor report writer](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/monitor/monitor.go#L232); [monitor interface](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/monitor/types.go#L12).

4. Keep generic selections honest:

   - Upgrade suite names currently include all, platform, and none. Record that exact selected name and command=run-upgrade; the existing testsuite.name remains openshift-tests-upgrade. No upgrade reporting-name prefix migration is needed.
   - File-only selection resolves to files. Record that value with no invented stage or shard.
   - A named suite restricted with --file or --run still reports the named selection. If needed, add a separate indication that the invocation was filtered; the property does not promise full-suite coverage.
   - An umbrella invocation reports the umbrella selection. Do not report every child membership suite as though it were a separate invocation.
   - Extension-defined suite names remain valid without requiring an openshift/ prefix.

   Sources: [upgrade suites](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/testsuites/upgrade_suites.go#L20); [suite selection](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/clioptions/suiteselection/suite_flags.go#L65); [extension suite discovery](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/testsuites/standard_suites.go#L37).

5. Keep artifact filenames and discovery paths intact. Existing consumers may ignore the additional properties while continuing to process ordinary test results.

6. Do not attach selected-suite metadata to auxiliary readiness analysis that has no selected execution suite. Leave those reports unclassified unless they receive their own explicit context. [Readiness analysis](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/e2eanalysis/e2e_analysis.go#L91) independently emits openshift-tests.

**Sippy ingestion and storage**

The suite import allowlist and existing suite ownership joins continue to work because testsuite.name stays unchanged. Do not change the meaning of existing suite_id, stable IDs, or ownership mappings to carry the new metadata.

Required work:

- Correct standard suite-property decoding. Sippy currently models the properties container as the individual property element and loses name/value data from standard properties/property XML. Add coverage for new metadata plus existing unrelated properties. Source: [suite property model](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/apis/junit/types.go#L40).
- Extract the suite context before flattening and deduplicating testcases. Define how nested suites inherit context, with an explicit child value overriding the inherited value. Distinguish suite-level properties from testcase properties. Treat absent metadata as unknown.
- Carry the context through testcase extraction and persistence without using it as an input to the legacy suite import decision.
- Expose selected suites, stage, and shard as execution attributes in query/API output. A result can have more than one selection context, so model that cardinality explicitly.

The current extraction key is (legacy suite name, test name). Main/monitor reports or several ordinary invocations can therefore contribute to one stored result. A single selected_suite field on that deduplicated row cannot always represent all contributing executions. Silently retaining the first or last property would lose information. Source: [testcase extraction](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/dataloader/prowloader/prow.go#L1069).

Recommended storage shape:

| Record | Purpose |
| --- | --- |
| Suite execution observation | Job-run identity, source artifact and suite occurrence, legacy JUnit suite, selected suite, optional stage/shard, and command |
| Test-result-to-suite-execution association | Connects the existing stored test result to every execution context that contributed to it |

An extensible test attribute framework may be considered as a substitute for the Test-result-to-suite-execution association shape. Designing such a framework is beyond the scope of the current planning.

Use the repository's release/timestamp partition conventions for references to job runs and test results. Give observations a deterministic source key so reprocessing the same artifact does not create duplicate observations. Keep observation metadata immutable for a given source.

This normalized shape makes selected-suite a logical test-execution attribute through the association. It also preserves multiple contexts without expanding the existing metric key. A scalar selected_suite projection can be provided when there is exactly one context; expose a set when there are several.

Retain the existing result/flake aggregation policy. Provenance links do not by themselves provide a separate per-suite pass rate when the underlying result combined several invocations. If per-suite status analysis becomes a requirement, preserve occurrence-level status before deduplication and design those statistics explicitly.

Record a suite observation when the report contains no executable testcases or only skipped testcases. This permits job-suite discovery without manufacturing synthetic test results or changing Sippy's current skipped-test policy. A run that fails before writing a report remains unknown; JUnit alone cannot report an invocation for which no artifact exists.

The existing [test and suite persistence](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/dataloader/prowloader/pgwriter/pgwriter.go#L340) and summary keys remain authoritative for existing metrics. The new observation/association tables require an additive migration and writer changes. The initial visibility feature does not require adding selected suite to daily or cumulative summary keys.

The synthetic [sig-sippy] openshift-tests should work test keeps its name and sippy suite. Verify its status and spot-check behavior are identical with and without the metadata. Metadata-only failures must not redefine this synthetic result.

**ci-cloud-functions and BigQuery (optional, pending requirements)**

This entire work package is optional. Additional requirements gathering must establish whether consumers need selected-suite metadata in BigQuery, rather than only in origin artifacts and Sippy. It is not a prerequisite for delivering the origin and Sippy changes. Without this extension, existing cloud ingestion continues processing legacy suite names and IDs; it does not capture the new properties.

Before including this work, determine:

- Which consumers and queries require selected-suite, stage, or shard data from BigQuery.
- Whether they need per-test metadata, per-run suite discovery, empty/all-skipped suite visibility, or aggregate provenance.
- Which destination tables and deployments need the data, and their retention requirements.

Use those requirements to choose the required subset of the possible changes below. Nullable testcase fields and a companion observation table are proposed options, not mandatory deliverables of the core plan. Defer the entire extension if no requirement justifies it.

If the extension is included:

The Python parser currently copies testsuite.name and derives raw IDs from that name and the testcase name. Preserve this code path and existing explicit ID overrides.

If per-test metadata is required, add nullable selected_suite, suite_stage, suite_shard, and suite_command fields to JUnitTestRecord and the relevant junit and junit_pr destination schemas. Populate them from suite context for each raw testcase record. Historical records and reports without properties retain NULL metadata. Update all supported deployment destinations before writing the extra columns.

Sources: [JUnitTestRecord](https://github.com/openshift-eng/ci-cloud-functions/blob/b8380a4cb5397514ab690f8a1d679a9d6cf10719/ci-to-bigquery/gcs_finalize_event.py#L343); [JUnitHandler](https://github.com/openshift-eng/ci-cloud-functions/blob/b8380a4cb5397514ab690f8a1d679a9d6cf10719/ci-to-bigquery/gcs_finalize_event.py#L1300).

Implement the metadata path for both SAX and ElementTree modes. Scope it to the active suite, reset it between sibling suites, and use a context stack for nested metadata inheritance. Preserve the existing legacy identity behavior while adding metadata parsing; unrelated parser corrections belong in separate changes.

The added field must not participate in raw test_id generation, flake keys, or existing result deduplication by default. Multiple observed selections remain provenance. If exact outcome attribution by selected suite is required, preserve invocation-scoped observations and review retry/flake semantics as a separate extension rather than silently changing existing metrics.

If requirements include empty/all-skipped suite visibility in BigQuery, persist suite-level observations in an additive BigQuery companion table. Use a deterministic observation identity containing the job run, artifact path, and suite occurrence, with the ingestion path's established retry/deduplication policy. Testcase records can refer to that identity for correlation. This also provides a stable place for invocation provenance when testcase records are deduplicated.

Do not use this change to rewrite existing testsuite fields, raw IDs, source metadata, or historical rows. No bulk historical reload is required.

**ci-test-mapping**

No functional changes are required for stable IDs, suite allowlists, component ownership, or the existing mapping joins. TestInfo.Suite continues receiving openshift-tests or openshift-tests-upgrade, so stage promotion and shard moves do not create new ownership identities.

Keep this new execution metadata out of StableID and the current mapping key. No identity namespace field, compatibility aliases, or shard-specific mapping rows are needed.

Sources: [StableID](https://github.com/openshift-eng/ci-test-mapping/blob/21407df0b29d5638cadeaed8e9f75f84b01fe6b5/pkg/util/tests.go#L65); [suite allowlist](https://github.com/openshift-eng/ci-test-mapping/blob/21407df0b29d5638cadeaed8e9f75f84b01fe6b5/config/openshift-eng.yaml#L1); [mapping defaults](https://github.com/openshift-eng/ci-test-mapping/blob/21407df0b29d5638cadeaed8e9f75f84b01fe6b5/pkg/components/component.go#L88).

Validate unchanged generated IDs, owners, capabilities, and rename behavior for legacy JUnit suite names. If new columns enter a BigQuery view read by this repository, verify that the query still selects its existing fields explicitly and that the view remains compatible. Mapping execution metadata into ownership is outside this alternative.

**ci-tools aggregation (optional, pending requirements)**

This entire work package is optional. Additional requirements gathering must establish whether consumers need selected-suite metadata from aggregated results, rather than the original origin reports and Sippy observations. It is not a prerequisite for delivering the origin and Sippy changes. With this extension deferred, existing aggregation behavior and historical baseline keys remain intact, but aggregated output does not retain the new suite-selection metadata.

Before including this work, determine:

- Which consumers need suite-selection visibility directly from aggregated results.
- Whether job-level selection sets are sufficient or individual combined tests need attribution to source execution contexts.
- Whether source job reports already satisfy those consumers, or aggregate provenance must be preserved and ingested separately.

Use those requirements to choose the necessary subset of the proposed changes below. A companion provenance artifact is an option to evaluate, not a mandatory core-plan deliverable. Defer the entire extension if no requirement justifies it.

Keeping the old suite names preserves:

- anyRealTestsRan recognition.
- Exact upgrade-check suite selection.
- Test-and-suite historical baseline keys.
- The historical test-summary query's legacy suite filter.

Sources: [real-suite predicate](https://github.com/openshift/ci-tools/blob/d5d01ecf301b98438187bae06654550f9d389a1a/pkg/jobrunaggregator/jobrunaggregatoranalyzer/analyzer.go#L340); [upgrade check](https://github.com/openshift/ci-tools/blob/d5d01ecf301b98438187bae06654550f9d389a1a/pkg/jobrunaggregator/jobruntestcaseanalyzer/analyzer.go#L43); [baseline keys](https://github.com/openshift/ci-tools/blob/d5d01ecf301b98438187bae06654550f9d389a1a/pkg/jobrunaggregator/jobrunaggregatoranalyzer/pass_fail.go#L170); [historical summary caller](https://github.com/openshift/ci-tools/blob/d5d01ecf301b98438187bae06654550f9d389a1a/pkg/jobrunaggregator/jobrunhistoricaldataanalyzer/analyzer.go#L107).

Metadata preservation is still required when consumers need selected-suite visibility from aggregated jobs. The [suite combiner](https://github.com/openshift/ci-tools/blob/d5d01ecf301b98438187bae06654550f9d389a1a/pkg/jobrunaggregator/jobrunaggregatoranalyzer/junit.go#L93) creates suites by their legacy names, merges tests into them, and does not propagate suite properties. Existing testcase property propagation is also a first-populated copy, which is insufficient for multiple selection contexts.

Do not rename or split aggregate suites to work around this, because that would reintroduce the compatibility impacts this design avoids. Preserve existing combined metrics and legacy names.

If requirements justify the extension, the proposed aggregate metadata representation is:

- Preserve the scalar selected-suite/stage/shard/command properties only when all contributing contexts agree.
- Record the complete set of distinct contexts in a companion machine-readable provenance artifact when they differ, including source run/artifact references and associations with combined testcase identities.
- Use a documented schema shared with the observation loaders. Define its discovery and ingestion paths before enabling metadata visibility for aggregated jobs. Copying a union of selected suite strings alone is sufficient for job-level visibility but is not sufficient for per-test attribution.
- Omit an ambiguous scalar property on multi-context aggregates; consumers must not confuse its absence with the absence of a provenance artifact.
- Preserve existing pass/fail, retry, lifecycle, and historical-baseline behavior.

If a provenance artifact is needed, finalize its filename, schema, discovery, and ingestion paths as part of this optional work package. It is an additional metadata path, not a replacement JUnit identity format.

**Query and product behavior**

The query functionality in scope is searching for tests associated with the new selected-suite, suite-stage, shard, and command properties, and returning their recorded execution context. The metadata comes from original origin reports. Visibility from aggregated output is conditional on the optional ci-tools extension; when deferred, aggregate metadata coverage remains unknown and must not be inferred from one contributing run.

Component Readiness and Variant Registry changes are outside the scope of the current plan. A separate discussion must review what functionality is needed beyond searching for tests associated with the new suite properties. This plan does not commit to changes in either product area.

- Search for tests by the new suite properties within a bounded job/run/time selection and return their associated execution contexts.
- Support stage/shard filters for that search without changing existing test identity or Component Readiness filtering and comparison behavior.
- Distinguish unknown metadata from non-lifecycle suites: a dedicated suite has a selected_suite but no stage; an old report has neither.
- Retain suite observations with no non-skipped tests at the storage layer; additional product workflows for displaying those observations require the separate functionality discussion.
- Avoid joins that duplicate existing result counts when a result has multiple context associations. Use distinct or existence-based filtering at the correct result grain.
- Deliver initial visibility through Sippy/PostgreSQL. If requirements include the optional BigQuery extension, provide equivalent semantics for the supported metadata queries. Until then, selected-suite metadata is unavailable from BigQuery; do not imply that backend already supports it.
- Update affected API documentation when the endpoints or response fields are implemented.

The separate discussion should identify the users and questions that require more than test search, then determine whether Component Readiness comparisons, statistical filters, summary dimensions, or Variant Registry job-level displays are needed. It must review historical semantics, missing metadata, and the distinction between per-run observations and a latest-known job configuration before choosing a design.

Raw observation metadata alone does not establish accurate historical stage-specific summary statistics. This plan preserves existing Component Readiness comparison/grouping behavior and the current Suite variant meaning; it does not prescribe a new variant, display, or default grouping. Any agreed functionality belongs in a separate plan.

**Impact compared with replacing testsuite.name**

| Concern | This additive alternative |
| --- | --- |
| Existing JUnit import allowlists | Continue matching legacy names |
| Raw BigQuery test IDs | Existing generation remains valid |
| Stable ownership IDs and mapping aliases | Existing generation and mappings remain valid |
| Upgrade detection and real-test checks | Existing suite selectors remain valid |
| ci-tools historical baselines | Existing suite keys remain valid |
| New identity compatibility protocol | Unnecessary |
| Selected-suite visibility | Requires new parsing, storage, and queries |
| PostgreSQL changes | New execution observations and test-result associations |
| BigQuery changes | Optional, pending requirements; nullable metadata fields and suite observations only where needed |
| Aggregated metadata | Optional, pending requirements; preserve selection sets or per-test provenance only where needed |
| Component Readiness and Variant Registry changes | Outside this plan; require a separate discussion of functionality beyond test search |
| Lifecycle/shard-specific statistics | Outside this plan; define requirements and query/summary semantics separately |

This trades identity migration risk for additive metadata plumbing. It still requires explicit handling of cardinality and aggregation; adding an XML property alone will not make the value available in databases.

**Delivery sequence**

1. Finalize the property names, exact-selection/stage/shard meanings, inheritance rules, and observation keys. Finalize an aggregate provenance schema only if requirements include that optional extension.
2. Add PostgreSQL observation and association migrations.
3. Implement metadata readers and writers in Sippy, including tests for legacy reports. Readers must tolerate missing properties.
4. Add the properties to origin's main and monitor reports. Old readers can ignore them while retaining ordinary results; deploying new readers first avoids losing initial metadata visibility.
5. Pilot ordinary conformance, a lifecycle shard, a dedicated/extension suite, file selection, and upgrade followed by conformance. Compare results against identical metadata-free fixtures.
6. Gather requirements for the optional ci-tools aggregation extension. If needed, define its scope and implement and validate metadata preservation and its discovery/ingestion path before claiming selected-suite visibility from aggregated output. Deferring it does not block the origin/Sippy rollout.
7. Enable forward-release reporting and query/API visibility. Keep older records unknown and preserve old-release output. Monitor parser errors, metadata coverage, invalid stage/shard classifications, and count differences.
8. Gather requirements for the optional ci-cloud-functions and BigQuery extension. If needed, define its scope, deploy the relevant additive schemas before writers, and implement and validate that path separately. Deferring it does not block the origin/Sippy rollout.
9. Hold a separate discussion of functionality beyond searching for tests associated with the new suite properties. Component Readiness, Variant Registry, and lifecycle-specific statistical changes require their own requirements and plan; they are not rollout dependencies or implementation deliverables here.

Rollback disables producer properties or metadata consumption. Compatible readers, additive schema, and already recorded observations can remain. Existing IDs and suite names require no reverse migration.

**Acceptance and validation**

| Scenario | Required outcome |
| --- | --- |
| Legacy XML without properties | Existing results, IDs, ownership, and aggregation unchanged; metadata unknown |
| Active -> stable -> minimal or a shard move | Updated execution metadata; identical test identity |
| Stage/shard fields alongside a full selected name | Values come from one resolved context and agree |
| Dedicated or extension-defined suite | Exact selection retained; no invented lifecycle stage or shard |
| Upgrade all followed by ordinary conformance | Two execution contexts; existing upgrade/ordinary IDs and suite names preserved |
| all, files, or a filtered named invocation | Honest selection context, without claiming full-suite coverage |
| Main and monitor reports | Same invocation context, with existing testcase results unchanged |
| Nested and sibling suites | Correct metadata inheritance/reset, without cross-suite leakage |
| Multiple selections contribute to one test result | Every context retained; legacy result counted once |
| Same-suite retry fail+pass | Existing flake behavior retained |
| Empty/all-skipped report | Suite observation retained without adding a fake test result |
| Failure before any report is written | Coverage remains unknown; no inferred invocation |
| ci-tools aggregation extension deferred | Existing aggregation and baseline behavior unchanged; suite-selection metadata from aggregate output remains unknown |
| Optional aggregate metadata with one context | Scalar metadata retained and result behavior unchanged |
| Optional aggregate metadata with several contexts | Required selection sets or per-test provenance retained; no first/last-value attribution |
| Existing component mappings and renames | IDs, owners, capabilities, and matching remain unchanged |
| BigQuery extension deferred | Origin/Sippy visibility works; existing BigQuery results and IDs remain unchanged, with selected-suite metadata unavailable there |
| Optional BigQuery extension included | Equivalent selection sets and unknown-value semantics for the supported metadata queries |
| Historical base versus new sample | Existing comparison grouping and stable IDs remain valid |

Use focused unit tests for property generation/classification, XML decoding, parser state, and metadata unions. Use database integration tests where supported for persistence, retry idempotency, associations, and duplicate-safe queries. BigQuery-specific metadata validation is required only if the optional extension is included. ci-tools-specific metadata validation is required only if the optional aggregation extension is included; use its existing unit-test structure for those changes. Compare unchanged legacy metrics across old/new metadata fixtures instead of asserting only that the new field is populated.

This document records design and source analysis. It does not claim implementation tests or live database validation have passed.

**Work packages**

- [ ] Agree on the property contract and observation semantics, including the exact role of selected-suite, stage, and shard; define aggregate provenance only if that optional extension is included.
- [ ] Implement origin reporting context and main/monitor properties.
- [ ] Fix Sippy standard suite-property decoding.
- [ ] Implement PostgreSQL suite observations, test-result associations, and visibility queries.
- [ ] Gather additional requirements and decide whether to include the optional ci-cloud-functions and BigQuery extension.
- [ ] Optional, if requirements justify it: implement the agreed BigQuery metadata fields, observation storage, and parser changes.
- [ ] Verify ci-test-mapping behavior remains unchanged without new identity logic.
- [ ] Gather additional requirements and decide whether to include the optional ci-tools aggregation extension.
- [ ] Optional, if requirements justify it: preserve the agreed aggregate metadata while retaining legacy suite names and baseline keys.
- [ ] Implement Sippy/PostgreSQL test search by the new suite properties, associated context output, and documentation; add equivalent BigQuery support for those queries only if that optional extension is included.
- [ ] Run the compatibility, multi-context, and empty-suite acceptance matrix.
- [ ] Pilot and roll out metadata reporting and test search.
- [ ] Discuss necessary functionality beyond test search separately; leave Component Readiness and Variant Registry implementation to a separate plan.

**Implementation decisions still to finalize**

Determine whether any BigQuery consumer requires the new metadata and which parts of that optional extension satisfy the need. Independently determine whether any consumer needs metadata from ci-tools aggregate output, and whether job-level selection sets or per-test provenance are required. Keep the core origin/Sippy delivery independent of both decisions.

Review requirements beyond test search in a separate discussion before planning any Component Readiness or Variant Registry changes. The current scope does not include their implementation.

Property names and database table names in this document are proposed. Finalize the partitioned observation/association schema, deterministic source keys and retry behavior, and invalid-property handling before implementing the core readers. Finalize aggregate provenance artifact schema/discovery only if the optional ci-tools work is included and requires that artifact.

The recommended contract keeps the exact selected suite as well as separate stage and shard fields. A stage-only variant of this alternative is possible, but would need additional suite-family and execution-mode fields to recover the actual invocation and would provide less direct linkage to origin's registry. Neither reporting layout changes the enhancement's registered suite names.
