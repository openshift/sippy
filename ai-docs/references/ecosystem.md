# Ecosystem References

Links to Tier 1 platform documentation. Sippy is a component tool, not a platform component, but these patterns are still valuable for development.

**Tier 1 Hub**: https://github.com/openshift/enhancements/tree/master/ai-docs

## Testing Practices (Tier 1)

Sippy-specific testing: See [../SIPPY_TESTING.md](../SIPPY_TESTING.md)

**Generic patterns** (Tier 1):
- [Testing Pyramid](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/testing/pyramid.md) - 60/30/10 ratio guidance
- [E2E Framework](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/testing/e2e-framework.md) - E2E test structure
- [Test Organization](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/testing/) - Where to put tests

## Security Practices (Tier 1)

**Generic patterns** (Tier 1):
- [STRIDE Threat Modeling](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/security/stride.md)
- [Secret Handling](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/security/secrets.md)
- [Input Validation](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/security/) - API input sanitization

## Development Practices (Tier 1)

Sippy-specific development: See [../SIPPY_DEVELOPMENT.md](../SIPPY_DEVELOPMENT.md)

**Generic patterns** (Tier 1):
- [API Evolution](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/development/api-evolution.md) - Versioning, compatibility
- [Code Organization](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/development/) - Repository structure
- [Documentation](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/development/) - What to document

## Reliability Practices (Tier 1)

**Generic patterns** (Tier 1):
- [SLI/SLO/SLA](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/reliability/slo.md) - Service level objectives
- [Observability](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/reliability/observability.md) - Metrics, logging
- [Degraded States](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/reliability/) - Graceful degradation

## Go Best Practices

**Sippy-specific**: Go idiomatic patterns in `pkg/`

**Generic patterns** (Tier 1):
- [Go Standards](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/development/go.md) - Idiomatic Go
- [Error Handling](https://github.com/openshift/enhancements/tree/master/ai-docs/practices/development/) - Error patterns

## Database Patterns

**Sippy-specific**: See [ADR-0002](../decisions/adr-0002-component-readiness-views.md) for component readiness views

**Generic patterns** (search OpenShift dev guides):
- PostgreSQL best practices
- Migration strategies
- Query optimization

## Frontend Patterns

**Sippy-specific**: React components in `sippy-ng/src/`

**Generic patterns** (not in Tier 1, external):
- React best practices
- Material-UI patterns
- State management

## Cross-Repo ADRs (Tier 1)

**Note**: Sippy is not a platform component, but these provide context on OpenShift architecture:

- [Why etcd](https://github.com/openshift/enhancements/tree/master/ai-docs/decisions/adr-0001-etcd.md) - Platform state storage
- [Why CVO Orchestration](https://github.com/openshift/enhancements/tree/master/ai-docs/decisions/) - Operator lifecycle management
- [Why Immutable Nodes](https://github.com/openshift/enhancements/tree/master/ai-docs/decisions/) - Node update strategy

**Sippy-specific ADRs**: See [../decisions/](../decisions/)
