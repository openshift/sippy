---
name: sippy-pre-branch-release-steps
description: "Perform pre-branch release preparation steps"
input: [release]
---

# Pre-Branch Release Steps

This command performs preparation work before a new release branches.

## Arguments

- `[release]`: New release version to prepare for branching (for example, `5.1`)

If the argument is not provided, prompt the user interactively.

## Workflow

1. **Check Sippy for the new release**
   - Query production Sippy release metadata for the requested release.
   - If the release is available, report its current status and metadata.
   - If the release is not available, report that Sippy does not yet know the release, then continue with the release-controller checks.

2. **Verify the release-controller configurations**
   - Inspect the `openshift/release` checkout for the requested release's controller configurations across all streams:
     - `amd64`
     - `multi`
     - `s390x`
     - `arm64`
     - `ppc64le`
   - Identify the corresponding `core-services/release-controller/_releases/*.json` files, including architecture-specific files where applicable.
   - Verify every expected stream has a configuration.
   - Verify aggregation configuration in each applicable stream, including:
     - Aggregated verification entries exist where expected from the previous release.
     - `aggregatedProwJob` is configured correctly.
     - `analysisJobCount` matches the previous-release configuration.
     - `multiJobAnalysis` is enabled for multi-job analysis configurations.
     - `upgradeFrom` streams are correct for micro and minor upgrade jobs.
   - Compare the new release configuration semantically with the previous release configuration; do not require an identical raw diff.
   - If a required stream configuration is missing, report it and mark that stream blocked. Do not force accept payloads for that stream, but continue checking and monitoring the other streams.
   - If a stream configuration is present but materially misconfigured, report the misconfiguration. Continue with that stream only if at least some verification jobs are configured well enough to validate the payload.
   - If a present stream has no usable verification jobs, report it and mark that stream blocked.

3. **Monitor streams for payloads**
   - Before watching for new payloads, check each stream for existing accepted payloads.
   - Count existing accepted payloads toward the required two, regardless of whether they were naturally or forcibly accepted.
   - Monitor the release-controller status pages for all streams:
     - `https://amd64.ocp.releases.ci.openshift.org`
     - `https://multi.ocp.releases.ci.openshift.org`
     - `https://s390x.ocp.releases.ci.openshift.org`
     - `https://arm64.ocp.releases.ci.openshift.org`
     - `https://ppc64le.ocp.releases.ci.openshift.org`
   - Watch for payloads that have attempted to run the stream's configured verification jobs.
   - A payload may be force accepted regardless of whether verification jobs pass, provided the jobs genuinely attempted to run.
   - Do not force accept a payload whose jobs failed because required images were missing; report the missing-image failure instead.
   - Allow naturally passing payloads to become accepted naturally.
   - Force accept only when the stream has fewer than two accepted payloads and the payload is not naturally passing or becoming accepted.

4. **Stop after two accepted payloads per eligible stream**
   - Track accepted payloads separately for each stream.
   - Count both naturally accepted and force-accepted payloads.
   - Stop monitoring an eligible stream after it has two accepted payloads.
   - Stop monitoring a blocked stream; it cannot reach two accepted payloads without configuration repair.
   - Stop the entire workflow after every stream is either eligible with two accepted payloads or blocked with a reported configuration problem.
   - Report the final count and payload identifiers for each eligible stream, along with the blocking reason for each blocked stream.
