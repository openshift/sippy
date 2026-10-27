## Summary

- Adds `docs/plans/trt-3016-triage-archival-plan.md`, a documentation-only implementation plan for the triage archival feature ([TRT-3016](https://redhat.atlassian.net/browse/TRT-3016)).
- The plan proposes adding an `Archived` boolean column to the `triages` table, with daemon-driven auto-archival when all of a triage's regressions belong to GA releases. The default listing endpoint excludes archived triages, with an `include_archived` query parameter to show them.
- Covers data model changes, daemon integration, API modifications, frontend updates, testing strategy, and query validation procedures with concrete code paths and symbols.

**Limitation:** TRT-3016's Jira description was not accessible during planning (Atlassian integration required authentication). All requirements are inferred from codebase evidence. Sections requiring parent verification are marked with `[needs TRT-3016 verification]`.

## Test plan

- [ ] Review plan for alignment with TRT-3016's actual requirements and acceptance criteria
- [ ] Verify that the inferred acceptance criteria (R1-R7) match the parent issue
- [ ] Confirm archival eligibility query against prod-like data (query validation procedure included in the plan)
- [ ] Validate uncertainties U1-U5 against TRT-3016 before implementation begins

🤖 Generated with [Claude Code](https://claude.com/claude-code)
