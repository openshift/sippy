# Architectural Decision Records (ADRs)

Component-specific decisions for Sippy. For cross-repo decisions, see [Tier 1 ADRs](https://github.com/openshift/enhancements/tree/master/ai-docs/decisions).

## Active ADRs

- [adr-0001-bigquery-data-source.md](adr-0001-bigquery-data-source.md) - Use BigQuery as primary data source
- [adr-0002-component-readiness-views.md](adr-0002-component-readiness-views.md) - PostgreSQL views for component readiness
- [adr-0003-variant-extraction.md](adr-0003-variant-extraction.md) - Extract variants from job names using regex

## ADR Template

Use [adr-template.md](adr-template.md) when creating new ADRs.

## When to Create an ADR

**Create ADR for**:
- Sippy-specific architectural decisions
- Data modeling choices
- Technology selections for Sippy

**Do NOT create ADR for**:
- Cross-repo decisions (use Tier 1 instead)
- Trivial implementation details
- Temporary workarounds

## ADR Lifecycle

1. **Proposed**: Draft ADR, under discussion
2. **Accepted**: Decision made, implemented
3. **Deprecated**: Still in use, but discouraged
4. **Superseded**: Replaced by newer ADR
