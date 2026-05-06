# Exec-Plans

Active feature implementation tracking for Sippy.

## What are Exec-Plans?

Exec-plans bridge the gap between enhancements (design) and PRs (implementation). Use them to track multi-week features that span multiple PRs.

**See [Tier 1 Exec-Plans Guide](https://github.com/openshift/enhancements/tree/master/ai-docs/workflows/exec-plans/) for**:
- Templates
- When to use exec-plans
- How to structure exec-plans
- Completion workflow

## Structure

```text
exec-plans/
└── active/     # Active features being implemented
```

**Note**: No `completed/` directory. When a feature is done, extract knowledge to ADRs/architecture docs, then delete the exec-plan.

## When to Use Exec-Plans

**Use exec-plans for**:
- Multi-week features (> 5 PRs)
- Cross-module changes (backend + frontend + dataloader)
- Features requiring coordination (database migration + API + UI)

**Don't use exec-plans for**:
- Single PR fixes
- Small features (< 3 PRs)
- Maintenance work

## Template

See [Tier 1 Template](https://github.com/openshift/enhancements/tree/master/ai-docs/workflows/exec-plans/template.md)

## Completion Workflow

1. **Extract knowledge**: Add learnings to ADRs or architecture docs
2. **Update docs**: Reflect new architecture in `architecture/components.md`
3. **Delete exec-plan**: Remove file from `active/`

See [Tier 1 Guide](https://github.com/openshift/enhancements/tree/master/ai-docs/workflows/exec-plans/README.md#completion-workflow) for details.
