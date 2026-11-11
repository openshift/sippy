**TRT-3030: JUnit suite names and consumer compatibility plan**

**Status:** Proposed. Implementation has not started.

Reviewed 2026-11-10 against the following default-branch snapshots. Source links throughout this plan are pinned to these revisions.

| Repository | Reviewed commit |
| --- | --- |
| openshift/origin | [b019a84de55c1b8fa8cdb91eef194217ad24b7b7](https://github.com/openshift/origin/commit/b019a84de55c1b8fa8cdb91eef194217ad24b7b7) |
| openshift/sippy | [bfb72a3c644162c6ddfd0d333c939cd978bd2796](https://github.com/openshift/sippy/commit/bfb72a3c644162c6ddfd0d333c939cd978bd2796) |
| openshift-eng/ci-test-mapping | [21407df0b29d5638cadeaed8e9f75f84b01fe6b5](https://github.com/openshift-eng/ci-test-mapping/commit/21407df0b29d5638cadeaed8e9f75f84b01fe6b5) |
| openshift/ci-tools | [d5d01ecf301b98438187bae06654550f9d389a1a](https://github.com/openshift/ci-tools/commit/d5d01ecf301b98438187bae06654550f9d389a1a) |
| openshift-eng/ci-cloud-functions | [b8380a4cb5397514ab690f8a1d679a9d6cf10719](https://github.com/openshift-eng/ci-cloud-functions/commit/b8380a4cb5397514ab690f8a1d679a9d6cf10719) |

The issue is [TRT-3030](https://redhat.atlassian.net/browse/TRT-3030). It asks to expose the selected suite in JUnit and future run records, preserve old results, and optionally expose the suite through the variant registry. Its listed consumers are openshift/sippy, openshift-eng/ci-test-mapping, openshift/ci-tools, and openshift-eng/ci-cloud-functions. The linked [enhancement PR 2080](https://github.com/openshift/enhancements/pull/2080) was open when reviewed at [371956b9b12099e00f4140894285cec26374d6eb](https://github.com/openshift/enhancements/blob/371956b9b12099e00f4140894285cec26374d6eb/enhancements/testing/test-suites.md), so its candidate/active/stable/minimal and shard names should be treated as proposed examples.

**Recommendation**

Make the JUnit suite name describe the invocation, and keep test identity in its existing namespace. These are separate concerns:

| Field | Meaning | Example |
| --- | --- | --- |
| JUnit testsuite.name; BigQuery testsuite; PostgreSQL suites.name | Selected execution suite | openshift/conformance/parallel/stable-05 |
| Compatibility identity namespace | Existing test identity family | openshift-tests |
| Stable ownership ID | Existing namespace plus existing stable-name hash | openshift-tests:existing-hash |
| Raw BigQuery test_id | Existing namespace plus existing raw-name hash, or existing explicit override | openshift-tests:existing-raw-hash |

The raw BigQuery ID and ownership ID have different existing normalization rules; they are not guaranteed to equal one another. Preserve each algorithm's old result under a suite-only change.

Keep ordinary tests under the openshift-tests namespace and run-upgrade tests under openshift-tests-upgrade. Do not create new test identities for lifecycle stages or shards. A new suite can have its own PostgreSQL suite row/ID without changing a test's stable ID. This preserves the existing distinction between the same named test run normally and during an upgrade.

Deploy compatible consumers before enabling renamed output. Historical JUnit and historical run rows retain their old suite names. Do not infer and rewrite old runs from today's suite membership.

**Origin implementation plan**

1. Introduce one reporting context, computed after suite selection, containing the selected suite, reported name, legacy identity namespace, and command mode. The current entry points already have the resolved TestSuite and separately pass hardcoded JUnit names: [run/options.go](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/cmd/openshift-tests/run/options.go#L95) and [run-upgrade/options.go](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/cmd/openshift-tests/run-upgrade/options.go#L121). Use the resolved suite, rather than test-name tags, binary names, or guesses from Prow job names.

2. Define the naming contract before switching output:
   - For ordinary named runs, report the exact resolved suite name, including the shard when present.
   - For run-upgrade, use an unambiguous reported name such as openshift/upgrade/all, openshift/upgrade/platform, or openshift/upgrade/none. These are proposed reporting names, not existing CLI suite names. The actual current names are all/platform/none, so simply substituting Suite.Name would make ordinary all and upgrade all ambiguous. Keep the selected CLI name separately as metadata. Reserve the upgrade reporting prefix against ordinary/extension suite collisions.
   - File-only selection currently creates the suite named files. A named suite combined with --file or --run remains that named suite with a selection filter. Report this honestly; do not claim it represents the entire registered suite. Optional selection metadata can describe the restricted run without changing the testcase name.
   - Origin also accepts extension-defined suites whose names need not start with openshift/. Do not make the protocol or compatibility classification depend solely on that prefix.

   Sources: [upgrade suite definitions](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/testsuites/upgrade_suites.go#L20), [suite selection](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/clioptions/suiteselection/suite_flags.go#L65), [extension suite discovery](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/testsuites/standard_suites.go#L37).

3. Add standard suite properties, with names finalized as a shared protocol. Suggested properties:
   - openshift-tests:selected-suite = the exact resolved CLI suite name.
   - openshift-tests:identity-suite = openshift-tests or openshift-tests-upgrade.
   - openshift-tests:command = run or run-upgrade.

   Example of proposed ordinary-run output:

   ~~~xml
   <testsuite name="openshift/conformance/parallel/stable-05">
     <properties>
       <property name="openshift-tests:selected-suite"
                 value="openshift/conformance/parallel/stable-05"/>
       <property name="openshift-tests:identity-suite"
                 value="openshift-tests"/>
       <property name="openshift-tests:command" value="run"/>
     </properties>
     <testcase name="[sig-example] existing test name" time="1"/>
   </testsuite>
   ~~~

   Existing suite properties are already supported in origin's [JUnit schema](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/test/ginkgo/junitapi/types.go#L22). Preserve TestVersion and all existing testcase names, source metadata, OTE lifecycle attributes, timing, failures, skips, and retry representation. OTE's blocking/informing lifecycle field remains separate from candidate/active/stable/minimal suite membership.

4. Apply the same context to the main junit_e2e report and e2e-monitor-tests report. The runner already passes one JUnit name to both. A change only in the main JUnit generator would leave the monitors under the old name and give a misleading mixture of suites. Update the monitor interface/serialization to carry the properties too; it currently constructs a suite with Properties:nil. Sources: [monitor serialization call](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/test/ginkgo/cmd_runsuite.go#L858), [main report creation](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/test/ginkgo/cmd_runsuite.go#L982), [monitor JUnit writer](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/monitor/monitor.go#L232), [monitor interface](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/monitor/types.go#L12).

5. Keep JUnit artifact filenames and discovery paths stable. A suite name with slashes belongs in XML, not in a filename. Origin's generator already accepts the name as data, so the main mechanical change is small. Upgrade followed by conformance should produce two suite names from two invocations, not one concatenated testsuite.name. Concatenation belongs only in a job-level display.

6. Inventory auxiliary JUnit emitters separately. [e2e readiness analysis](https://github.com/openshift/origin/blob/b019a84de55c1b8fa8cdb91eef194217ad24b7b7/pkg/e2eanalysis/e2e_analysis.go#L91) also emits openshift-tests but has no selected execution suite. Leave that as a legacy auxiliary report, or give it an explicitly designed analysis identity in separate work. Do not falsely label it as whichever suite happened to run later. Suite-discovery queries must distinguish these auxiliary results from selected execution suites.

7. Stage the producer change: first emit the new metadata with legacy names, then enable selected names through a temporary opt-in/reporting mode on validation jobs, then make it the default on the forward development branch after consumers pass. Older release branches keep their current output. The temporary mode provides a concrete rollback without rewriting stored results.

**Consumer review: openshift-eng/ci-test-mapping**

Risk: new results can be excluded, or every lifecycle/shard movement can create a new stable test ID.

- config/openshift-eng.yaml explicitly includes openshift-tests and openshift-tests-upgrade. The current inclusion patterns cover layered-product suites, not new origin suite names. [suite configuration](https://github.com/openshift-eng/ci-test-mapping/blob/21407df0b29d5638cadeaed8e9f75f84b01fe6b5/config/openshift-eng.yaml#L1); [suite filters](https://github.com/openshift-eng/ci-test-mapping/blob/21407df0b29d5638cadeaed8e9f75f84b01fe6b5/pkg/bigquery/test_table.go#L71).
- util.StableID directly prefixes the hash with TestInfo.Suite. Changing Suite therefore changes the ID even if the test name and ownership are unchanged. [StableID](https://github.com/openshift-eng/ci-test-mapping/blob/21407df0b29d5638cadeaed8e9f75f84b01fe6b5/pkg/util/tests.go#L65).
- TestIdentifier.setDefaults calls that helper after applying global and component rename handling. When adding an identity namespace to TestInfo, carry it through the reconstructed TestInfo used for global renames. Keep the existing test-name canonicalization unchanged. [setDefaults](https://github.com/openshift-eng/ci-test-mapping/blob/21407df0b29d5638cadeaed8e9f75f84b01fe6b5/pkg/components/component.go#L88).
- Mapping rows must retain the actual execution suite in TestOwnership.Suite. Produce a row for each observed (name, actual suite) pair, with the same stable ID for ordinary lifecycle/shard aliases. Retain the old (name, openshift-tests) rows too. Replacing all output suite fields with the legacy namespace would break consumers that join on the observed suite.
- Recommended data extension: have cloud ingestion expose a nullable identity_suite column. Add TestInfo.IdentitySuite and use COALESCE(NULLIF(identity_suite,''), testsuite) for identity classification/allowlisting, while selecting testsuite as the actual Suite. util.StableID uses IdentitySuite when present and the current Suite fallback otherwise. This supports arbitrary extension suite names without enumerating every shard or broadly normalizing unrelated suites.
- Audit suite-sensitive ownership policy separately from identity. ComponentMatcher has exact/regex suite matching. The current literal openshift-tests matches are primarily configuration/tests rather than a large set of production ownership matchers. Obsolete-test matching is also pair-based, but its current explicit entries belong to other suites. There is no need to rewrite those unrelated entries. [suite matchers](https://github.com/openshift-eng/ci-test-mapping/blob/21407df0b29d5638cadeaed8e9f75f84b01fe6b5/pkg/config/component.go#L158); [obsolete-test rules](https://github.com/openshift-eng/ci-test-mapping/blob/21407df0b29d5638cadeaed8e9f75f84b01fe6b5/pkg/obsoletetests/ocp_obsoletetests.go#L16).
- Strengthen migration verification by comparing stable ID, component, capabilities, and obsolete status across old/new suite aliases. Existing map-verify compares the same (name,suite) key and can miss ownership loss when the suite itself changes. [mapping verification](https://github.com/openshift-eng/ci-test-mapping/blob/21407df0b29d5638cadeaed8e9f75f84b01fe6b5/cmd/ci-test-mapping/mapping_verify.go#L66).

Delivery: ingestion filters and identity input; stable-ID compatibility; alias mappings; mixed old/new fixtures and generated-data review. Ship before renamed origin results.

**Consumer review: openshift/sippy**

Risk: silent loss of new suites, missing ownership joins, and altered historical comparisons.

- The PostgreSQL Prow loader explicitly skips non-importable top-level suites before extracting tests or constructing synthetic results. Its allowlist contains only the two legacy origin names and unrelated patterns. The new ordinary and upgrade names tested in the replay are all rejected. [suite allowlist](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/db/suites.go#L21); [import gate](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/dataloader/prowloader/prow.go#L1019).
- Fix the metadata decoder before relying on a property-based import gate. Sippy currently models the properties container as the property element; standard properties/property XML yields an object with empty Name and Value. Correct the XML path/element model and add round-trip coverage. [suite property model](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/apis/junit/types.go#L40).
- Make the import decision accept recognized identity metadata, retaining current legacy/unrelated-suite behavior. Do not use only openshift/ matching: files, all, kubernetes suites, and extension-defined names are valid origin invocations. Keep the actual suite name when extracting the test.
- No new PostgreSQL run-suite column is needed for the core request: the writer already creates suites rows, puts suite_id on prow_job_run_tests, and keeps tests keyed by name. Suite dimensions also exist in daily and cumulative summaries. [test and suite inserts](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/dataloader/prowloader/pgwriter/pgwriter.go#L340).
- Preserve actual-suite ownership aliases. The ownership loader imports (name,suite) rows and stores the stable unique_id separately. PostgreSQL joins on test_id plus suite_id; BigQuery joins on testsuite plus test_name and reports cm.id. Both need old and new mapping pairs to remain available. [ownership upsert](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/dataloader/testownershiploader/testownershiploader.go#L104); [PostgreSQL ownership join](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/api/componentreadiness/dataprovider/postgres/provider.go#L459); [BigQuery ownership join](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/api/componentreadiness/dataprovider/bigquery/querygenerators.go#L430).
- Preserve the synthetic name [sig-sippy] openshift-tests should work and its suite sippy. Current synthetic conversion derives test status from imported non-special tests, rather than requiring testsuite.name == openshift-tests. Spot-check queries select the synthetic test by name. Therefore the primary hazard is filtering away its input, not a need to rename the synthetic test. Test failed, successful, informing-only, aborted, and errored runs, including two-suite upgrades. Current BigQuery spot-check methods are no-ops; do not claim existing backend parity there. [synthetic input conversion](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/dataloader/prowloader/testconversion/testconversion.go#L32); [synthetic status](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/synthetictests/ocp_synthetic_tests.go#L149); [spot-check query](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/api/componentreadiness/dataprovider/postgres/provider.go#L1042); [BigQuery spot-check methods](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/api/componentreadiness/dataprovider/bigquery/provider.go#L423).

Delivery: property parsing, import classification, alias-aware ownership verification, PostgreSQL/BigQuery comparison tests, and spot-check regression coverage. Actual suite rows can be added without relabeling historical runs.

**Consumer review: openshift/ci-tools**

Risk: aggregation can reject valid results, miss upgrade checks, or change pass/fail decisions during the transition.

- anyRealTestsRan recognizes a suite with the regex openshift-tests|e2e|E2E|BackendDisruption. New names such as openshift/conformance/parallel and openshift/upgrade/all do not match. Where no other recognized suite is present, aggregation can report that none of the jobs ran real tests. Use recognized reporting metadata with legacy fallback. [aggregation guard](https://github.com/openshift/ci-tools/blob/d5d01ecf301b98438187bae06654550f9d389a1a/pkg/jobrunaggregator/jobrunaggregatoranalyzer/analyzer.go#L308).
- The required upgrade check explicitly targets openshift-tests-upgrade. Its lookup requires the suite to match before looking for the testcase. Accept the new upgrade reporting identity while preserving old input. [upgrade check identifier](https://github.com/openshift/ci-tools/blob/d5d01ecf301b98438187bae06654550f9d389a1a/pkg/jobrunaggregator/jobruntestcaseanalyzer/analyzer.go#L43); [exact suite lookup](https://github.com/openshift/ci-tools/blob/d5d01ecf301b98438187bae06654550f9d389a1a/pkg/jobrunaggregator/jobruntestcaseanalyzer/analyzer.go#L269).
- Suite metadata must survive aggregation. combineTestSuite creates suites by name and combines tests/children, but does not copy suite properties; existing property propagation is only for testcases. Copy and validate the compatibility properties when combining suites. For conflicting identity namespaces under the same reported name, fail clearly rather than silently selecting one. [suite combination](https://github.com/openshift/ci-tools/blob/d5d01ecf301b98438187bae06654550f9d389a1a/pkg/jobrunaggregator/jobrunaggregatoranalyzer/junit.go#L93).
- Historical baselines use (test name, combined suite name), independently of stable ownership IDs. A rename can miss historical data and hit the existing 70% default baseline. Plan an explicit compatibility lookup by stable identity/legacy namespace for the same job, with new-suite data preferred when enough exists. Define aggregation across aliases when a baseline window straddles cutover; do not normalize away the suite in emitted reports. [history keys](https://github.com/openshift/ci-tools/blob/d5d01ecf301b98438187bae06654550f9d389a1a/pkg/jobrunaggregator/jobrunaggregatoranalyzer/pass_fail.go#L170); [missing-history behavior](https://github.com/openshift/ci-tools/blob/d5d01ecf301b98438187bae06654550f9d389a1a/pkg/jobrunaggregator/jobrunaggregatoranalyzer/pass_fail.go#L600).
- Historical test-summary generation separately hardcodes suiteName = openshift-tests; the underlying query filters TestsSummaryByDate.suite = @suite_name. Extend this input to include actual suites classified into the original identity family, and aggregate at the intended test grain. Otherwise the input for historical test analysis/retry decisions will shrink after rollout. [summary caller](https://github.com/openshift/ci-tools/blob/d5d01ecf301b98438187bae06654550f9d389a1a/pkg/jobrunaggregator/jobrunhistoricaldataanalyzer/analyzer.go#L107); [summary query](https://github.com/openshift/ci-tools/blob/d5d01ecf301b98438187bae06654550f9d389a1a/pkg/jobrunaggregator/jobrunaggregatorlib/ci_data_client.go#L1127).
- The old 4.10 entry in testsRequiringHistoryRewrite is a historical exception, not a reason to rename historical data. Other openshift-tests hits refer to repository/job names or fixtures and are not JUnit suite contracts.

Delivery: real-suite detection, upgrade selection, metadata propagation, baseline compatibility, and historical query changes. Preserve pass/fail decisions for identical mixed-format input fixtures before enabling production output.

**Consumer review: openshift-eng/ci-cloud-functions**

Risk: raw BigQuery IDs change even if ci-test-mapping is fixed.

- The Python JUnitHandler stores testsuite.name and independently derives test_id as testsuite:md5(test_name), except for its existing explicit {hex} name override. The local replay executes this actual startElement method and confirms ID churn. [JUnitHandler](https://github.com/openshift-eng/ci-cloud-functions/blob/b8380a4cb5397514ab690f8a1d679a9d6cf10719/ci-to-bigquery/gcs_finalize_event.py#L1300).
- Keep testsuite as the observed execution name. Read the identity-suite property and use it only for the ID namespace. With metadata absent, preserve the current legacy and non-origin behavior. Keep explicit ID overrides and OOMCheck handling unchanged.
- Add nullable identity_suite to the appropriate JUnit schemas and JUnitTestRecord for both junit and junit_pr. Deploy schema changes before writing the extra field. This is a recommended additive schema change to support ci-test-mapping reliably; the existing testsuite field itself already accepts the new values.
- Implement suite context correctly for both SAX and ElementTree paths. The inspected default uses SAX, but both paths call JUnitHandler. Reset/stack suite state across multiple or nested testsuites; distinguish suite properties from testcase properties. Do not let a prior suite's identity leak into the next one. Properties must be read before testcases.
- Flake tracking currently keys by test_id. Preserving the namespace across different execution suites can cause same-name tests in different suites in one XML document to share flake state. Key retry/flake bookkeeping by observed suite plus test identity, while preserving the exported ID. Fail+pass within the same suite still counts as a flake; a failure in one suite and pass in another must stay separate. [flake bookkeeping](https://github.com/openshift-eng/ci-cloud-functions/blob/b8380a4cb5397514ab690f8a1d679a9d6cf10719/ci-to-bigquery/gcs_finalize_event.py#L1353).
- The artifact filename filter already accepts junit and e2e-monitor-tests files; keeping filenames stable avoids a discovery change. The Go ci-data-loader handles interval, metrics, autodl, and symptom files, not this JUnit-to-row path. [artifact filter](https://github.com/openshift-eng/ci-cloud-functions/blob/b8380a4cb5397514ab690f8a1d679a9d6cf10719/ci-to-bigquery/gcs_finalize_event.py#L389); [Go loader scope](https://github.com/openshift-eng/ci-cloud-functions/blob/b8380a4cb5397514ab690f8a1d679a9d6cf10719/ci-data-loader/functions.go#L190).

Delivery: schema addition, suite context parsing, raw-ID compatibility, and same-suite/cross-suite flake tests. No bulk historical reload is required.

**Optional variant-registry work**

The issue describes Suite as formerly deprecated, but the reviewed implementation actively derives Suite from the job name as parallel, serial, etcd-scaling, or unknown. Suite is also in production Component Readiness db_group_by. Replacing those values with full execution suites would separate base and sample comparison cells even if every stable test ID is preserved. [setSuite](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/variantregistry/ocp.go#L588); [grouping configuration](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/config/views.yaml#L24).

For the first delivery, expose the executed suite set separately (for example ExecutedSuite), outside default comparison grouping. Compute it from one recent, sufficiently complete run; sort and deduplicate before joining with +. An upgrade plus conformance can then display openshift/upgrade/all+openshift/conformance/parallel. Do not union six months of results, which would combine retired and current configurations.

If reusing the Suite key is a requirement, make that a separate migration with an explicit historical grouping policy. New output alone cannot recover exact suites for old results. Also, a job-level registry can describe only the latest observed suite set, not a historical per-run truth.

Queries based only on test rows cannot see an empty/all-skipped suite: Sippy currently discards skipped cases. For complete invocation visibility, including early failure or no executed tests, record a separate per-run suite observation from report metadata. Do not manufacture testcase rows just to make the suite visible.

**Risks: shard changes, cumulative summaries, and historical comparisons**

Preserving a stable ownership ID does not preserve the PostgreSQL aggregation key. Once the import gate accepts the new names, shard rebalancing and stage promotion can split one test's results across several suite IDs even when its testcase name and stable ownership ID stay unchanged.

**Raw result history.** The writer resolves tests by exact testcase name and suites by exact suite name. A move from openshift/conformance/parallel/stable-05 to openshift/conformance/parallel/stable-07 keeps the numeric tests.id but uses the destination suite_id for new prow_job_run_tests rows. Historical rows retain their original suite_id. Suite rows are shared across tests; an existing destination name reuses its ID, including when a test returns to an earlier shard. The name change alone does not multiply raw result rows when each execution still produces one result. Source: [test and suite persistence](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/dataloader/prowloader/pgwriter/pgwriter.go#L340).

**Additional summary series.** Both test_daily_totals and test_cumulative_summaries use the unique key (release, date, test_id, suite_id, lifecycle, prow_job_id). The stable ownership unique_id is not part of that key. A previously unseen destination combination starts a separate cumulative series, without transferring the old suite's prefix sums. New-series rows are created from the first execution date through tomorrow. Sources: [summary indexes](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/db/migrations/000011_add_lifecycle_to_summaries.up.sql#L17); [summary row creation and updates](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/dataloader/prowloader/pgwriter/pgwriter.go#L546).

For the same test, Prow job, release, and blocking/informing lifecycle, an illustrative move looks like this:

| Snapshot | stable-05 prefix runs | stable-07 prefix runs | Cumulative rows |
| --- | --- | --- | --- |
| Before the move | 100 | Absent | 1 |
| After 10 executions in shard 07 | 100 | 10 | 2 |
| After another 10 executions in shard 07 | 100 | 20 | 2 |

**Inactive series keep growing in storage.** Daily carry-forward copies every previous snapshot row, including keys that no longer receive results. The old prefix remains constant, but another row containing it is written each day. Dropping old date partitions does not remove that key from newer snapshots. Growth follows distinct historical (release, test, job, suite, lifecycle) combinations, rather than the number of moves: returning to a previously used combination resumes its series. A fixed set of suite names bounds the suite dimension, but repeated rebalancing can spread tests across more of those combinations. This increases daily inserts, index storage, WAL, and report scans. For illustration, 100,000 additional combinations present throughout 90 retained snapshots mean nine million additional cumulative rows. This is not a production estimate. Source: [unfiltered carry-forward](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/dataloader/prowloader/pgwriter/pgwriter.go#L645).

**Job layout affects the incremental cost.** If different shards run in different Prow jobs, a move already changes prow_job_id and creates another summary series under legacy naming. Do not attribute that entire increase to suite renaming. Measure distinct suite IDs per (release, test, job, lifecycle) to isolate the additional effect. The initial switch from openshift-tests to a selected suite still splits an existing combination, even if each job subsequently keeps a fixed suite.

**Stable aliases do not yet guarantee correct reports.** Prefix subtraction remains correct within each suite; an inactive series contributes zero runs to a window after its last execution. Cross-suite history requires explicit alias aggregation. Ordinary test reports group by suite_id, so they expose separate histories. PostgreSQL Component Readiness also aggregates by test_id and suite_id before joining ownership. Its Go scanners then assign results to a map keyed only by stable ownership ID and variants. Multiple aliases in one window and variant group can overwrite each other's counts instead of being summed. Missing aliases can instead exclude results at the ownership join. Sources: [test report grouping](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/db/query/test_queries.go#L294); [Component Readiness aggregation and ownership join](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go#L126); [combined result assignment](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go#L665); [standalone result assignment](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/api/componentreadiness/dataprovider/postgres/cr_queries.go#L781).

**Suite is an active comparison variant.** No evidence of current deprecation was found in the inspected code, configuration, or relevant registry history. The registry still assigns Suite, PostgreSQL explicitly imports it through importantVariants, and Component Readiness includes it in db_group_by. Reusing this key for execution suites changes an existing comparison dimension. Source: [PostgreSQL variant import list](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/testidentification/ocp_variants.go#L28), alongside the registry and grouping links above.

**Variant combination changes are separate from suite ID changes.** Renaming JUnit testsuite.name alone does not change the registry's job-name-derived Suite value. If the registry value is also replaced, PostgreSQL resolves the complete updated variants array to another variant_combination_id on prow_jobs, creating or reusing a matching combination. The trigger runs on inserts and updates of variants. Sources: [combination resolution](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/db/migrations/000003_create_variant_combinations.up.sql#L12); [trigger attachment](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/db/db.go#L560).

| Change | Summary storage effect | Reporting effect |
| --- | --- | --- |
| Change only JUnit suite name | A new suite_id combination starts another summary series | Suite-specific histories split |
| Change only registry Suite value | Existing summary keys and counts remain; variant_combination_id is not in their key | The job's history is regrouped through its current variants |
| Change both | New suite-specific series and a changed job variant combination | Both effects apply |

**Historical variants are not execution snapshots.** The variant syncer updates the existing prow_jobs row, and reports join historical results and summaries to that row's current variant_combination_id. Changing a job from Suite=parallel to Suite=openshift/conformance/parallel/stable-07 therefore also reclassifies its older results under the new value. Within one job, both base and sample windows can follow the latest classification, concealing the transition. Keeping historical suite_id values unchanged does not preserve historical variant attribution. BigQuery also joins job_variants by job name, and the registry updates those values in place. Sources: [PostgreSQL variant sync](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/dataloader/variantsyncer/variantsyncer.go#L42); [historical summary join](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/db/query/test_queries.go#L294); [BigQuery variant joins](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/api/componentreadiness/dataprovider/bigquery/querygenerators.go#L400); [registry updates](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/variantregistry/loader.go#L259).

**Shard-valued variants can break comparison continuity.** Across jobs or releases whose current Suite values differ, the same stable test ID under stable-05 and stable-07 belongs to different comparison groups. This can produce missing-basis/sample behavior or smaller statistical samples. A changed numeric combination ID alone does not force separate Component Readiness groups: combinations are projected onto db_group_by dimensions first. The problem here is that Suite is one of those dimensions. A multi-suite value such as openshift/upgrade+openshift/conformance/parallel is one exact grouping value for every test in the job; adding another invocation changes the classification even for tests whose own execution suite is unchanged. Source: [variant projection and grouping](https://github.com/openshift/sippy/blob/bfb72a3c644162c6ddfd0d333c939cd978bd2796/pkg/api/componentreadiness/dataprovider/postgres/variants.go#L44).

Before enabling renamed output:

- Retain old and new ownership aliases. Aggregate comparison counts across aliases by stable identity and comparison variants before failure thresholds and result-map construction; preserve the latest failure timestamp and define how execution-suite provenance is displayed. Verify parity with BigQuery.
- Add regression coverage spanning legacy output, multiple shard moves within one comparison window, a return to an earlier shard, inactive carry-forward, and moves between different Prow jobs. Assert complete counts and unchanged statistical decisions, not just stable IDs.
- Measure affected key counts and project cumulative rows across the retention window. Establish acceptable storage and query costs; raw-result counts alone do not reveal the carry-forward cost.
- Preserve the existing Suite comparison classification unless a separate migration defines baseline compatibility and historical attribution. Prefer per-run execution metadata for historical suite/shard truth. If ExecutedSuite is added to the job registry for current-job discovery, keep it outside comparison grouping and PostgreSQL's importantVariants list to avoid changing variant_combination_id. Adding it to importantVariants would change combination IDs even if it remained outside db_group_by.
- Cover registry Suite changes, cross-release shard differences, and changing multi-suite sets in regression fixtures. Verify both historical attribution and comparison groups, and confirm that a variant-only update leaves summary keys and counts intact.
- Evaluate the [additive suite-properties alternative](trt-3030-junit-suite-properties-alternative.md) if execution visibility is sufficient. It retains legacy suite_id values and stores shard selection as execution provenance outside existing summary keys. Movement between different Prow jobs still creates separate job keys.

These risks are based on source inspection, without production database measurements or a live integration replay of shard movement or variant changes.

**Rollout and acceptance**

1. Finalize the reporting names, compatibility metadata, additive BigQuery field, and supported fallback rules.
2. Land consumer readers, schema support, and ci-tools aggregation compatibility while origin still emits legacy names.
3. Emit metadata with legacy names; verify it survives main reports, monitor reports, and aggregated reports.
4. Run an opt-in origin pilot for conformance, a dedicated suite, an extension suite, file selection, and upgrade followed by conformance. Seed/publish new (name,suite) mapping aliases from enumerated membership or this isolated pilot before broad enablement; retain old aliases.
5. Verify both database backends with a base window containing legacy names and a sample window containing new names. Compare run counts, status, ownership, stable IDs, and drill-down results. Validate aggregation baselines independently of stable IDs.
6. Enable default names on the forward development branch only. Monitor skipped-suite logs, missing mappings, aggregation guard failures, missing-history fallbacks, and spot-check counts. Roll back producer naming if necessary; keep compatible readers and mappings for results already written.

Minimum regression fixture matrix:

| Scenario | Required result |
| --- | --- |
| Legacy XML without new properties | Existing import, raw ID, stable ID, and status unchanged |
| Active -> stable -> minimal or shard move, same test name | Actual suite changes; ordinary test identity stays identical |
| Upgrade and ordinary run with same test name | Two observed suites; old upgrade/ordinary ID distinction preserved |
| Main and monitor reports | Same selected-suite context and correct identity namespace |
| Extension-defined suite or files/all | Imported without relying on openshift/ naming |
| Fail+pass in one suite | Existing flake result preserved |
| Fail in one suite, pass in another | Two independent outcomes |
| Informing-only failures, success, aborted/error run | Existing synthetic and spot-check semantics preserved |
| Old and new mapping aliases in one comparison | Historical results join once to their correct owner and stable ID |
| ci-tools aggregation round trip | Metadata retained, upgrade check found, valid suite recognized, baseline policy preserved |
| Empty/all-skipped execution | Per-run suite observation records invocation if that optional completeness feature is implemented |

**Evidence and remaining limits**

Focused source replays were performed during the 2026-11-10 investigation. The Python replay executed the inspected cloud JUnitHandler.startElement method. A standalone Go harness executed the inspected StableID, importability, and real-suite predicates with the Sippy JUnit types. The only substitutions were local external type names and an equivalent string-set implementation. The temporary harnesses are not prerequisites for reading or implementing this plan.

The investigation checked 11 Go predicate cases, five cloud ID cases, and standard suite-property decoding:

| Check | Observed current behavior |
| --- | --- |
| Legacy openshift-tests and openshift-tests-upgrade | Imported by Sippy and recognized by the aggregation predicate |
| openshift/conformance/parallel and active-02, stable-05, minimal descendants | Rejected by Sippy's import gate and missed by the aggregation predicate |
| openshift/upgrade/all, kubernetes/conformance, all, files, custom-extension-suite | Rejected by Sippy's import gate and missed by the aggregation predicate |
| Same test name across old, active, stable, and upgrade suite names | Both ID generators change the namespace prefix; the unchanged name has the same hash |
| Existing explicit cloud ID override in a testcase name | Override remains authoritative |
| Standard properties/property element with identity-suite metadata | Sippy decodes an empty Name and Value instead of the property contents |

All 43 source links in the original review were checked against the downloaded commit snapshots for matching revisions, existing paths, and valid line numbers.

These are focused source replays, not complete repository builds or live database integration tests. Deployed BigQuery view definitions and deployment versions were not queried. Inspect component_mapping_latest and TestsSummary/TestsSummaryByDate definitions before rollout to ensure aliases are retained and nullable metadata reaches queries. The suite-name change also requires compatible BigQuery/PostgreSQL aggregate/query behavior, rather than merely a successful XML parse.


**Implementation work packages**

- [ ] Agree on the reporting-name and metadata contract, including upgrade prefix reservation and legacy fallback behavior.
- [ ] Add the nullable BigQuery identity namespace field to all supported JUnit destinations before deploying writers.
- [ ] Update ci-cloud-functions parsing, exported raw IDs, and suite-scoped flake bookkeeping.
- [ ] Update ci-test-mapping filtering, identity namespace propagation through renames, and old/new ownership aliases.
- [ ] Update Sippy suite-property parsing and import classification; verify ownership joins, summaries, drill-downs, and synthetic spot-check results.
- [ ] Update ci-tools suite recognition, upgrade checks, suite-property propagation, historical baseline lookup, and test-summary queries.
- [ ] Add origin reporting context to main and monitor reports; stage metadata under legacy names before enabling selected names.
- [ ] Run the pilot and mixed-format regression matrix, publish the required mapping aliases, and verify deployed BigQuery views.
- [ ] Enable selected names on the forward development branch and monitor the rollout; retain reader compatibility when rolling back producer naming.
- [ ] Evaluate executed-suite variant reporting and complete per-run suite observations as separate optional work.

**Decisions to finalize before implementation**

The recommendations above establish a concrete starting design. Confirm the property names, upgrade reporting names, BigQuery column name, and initial release boundary before changing the wire format. Define how ci-tools combines historical alias data across the cutover without changing the intended baseline policy. Keep the optional variant-registry migration separate unless its historical grouping semantics are resolved with the core change.
