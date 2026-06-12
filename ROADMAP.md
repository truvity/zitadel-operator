# Zitadel Operator Roadmap

## Current: v0.10.0 (v1alpha2)

### Implemented Resources (12 CRDs)

#### Project-Level (PROJECT_OWNER)
- OIDCApp — OIDC application with Secret output (confidential/public, drift detection)
- ProjectMember — Assign user roles on a project

#### Organization-Level (ORG_OWNER)
- Organization — Create/manage organizations
- Project — Create/manage projects with role sync
- MachineUser — Service accounts with JWT key generation
- UserGrant — Assign project roles to users
- ProjectGrant — Share a project with another organization
- OrgMetadata — Key-value metadata on the org
- Domain — Register org domain for domain discovery
- IdentityProvider — Configure generic OIDC identity provider

#### Instance-Level (IAM_OWNER)
- ActionTarget — Webhook targets for Actions v2
- ActionExecution — Bind targets to trigger conditions

### Architecture
- Config file (`--config`) with single YAML
- `watchNamespaces` for namespace isolation
- Helm chart with ConfigMap + Secret volume mounts
- Namespace-scoped RBAC (Role/RoleBinding per namespace)
- `GenerationChangedPredicate` on all controllers (no hot-loops)
- Conditional status updates
- Graceful retry for transient ref-not-ready errors (10s requeue)
- Finalizer-based cleanup on deletion

### Integration Tests (18 scenarios)
All tests run against a real Zitadel Cloud instance via envtest.

## Future

### Additional Resources
- [ ] LoginPolicy (org-scoped) — complex field set, low change frequency
- [ ] PasswordComplexityPolicy (org-scoped)
- [ ] LockoutPolicy (org-scoped)
- [ ] APIApp — API application type
- [ ] HumanUser — human user management

### Production Hardening
- [ ] Structured status conditions (Ready/Synced/Error with reason codes)
- [ ] Prometheus metrics (custom reconcile counters, Zitadel API latency)
- [ ] Exponential backoff for persistent Zitadel API errors

### Not Planned (instance-level, Zitadel Cloud limitation)
- DefaultLoginPolicy — requires System API
- DefaultDomainPolicy — requires System API
- Instance-level IdP (Google/GitHub) — requires System API

## Design Principles

1. **Explicit over implicit** — every resource declares its organization
2. **Hierarchical references** — Organization → Project → App/User
3. **Operator-per-instance** — bound to one Zitadel instance via config file
4. **Namespace-based isolation** — multiple operators via namespace scoping + K8s RBAC
5. **No CRD for connection** — instance config is deployment config, not a reconciled resource
6. **Terraform provider parity** — resource coverage matches the Zitadel Terraform provider for org/project scope

## Testing Model

| Package | Purpose | Command |
|---------|---------|---------|
| `internal/config/` | Config loader unit tests | `go test ./internal/config/...` |
| `tests/integration/` | Full reconcile loop (envtest + real Zitadel) | `go test -tags=integration ./tests/integration/...` |
