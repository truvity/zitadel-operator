# Zitadel Operator Roadmap

## Current: v1.0.0-alpha1 (v1alpha2)

### Implemented Resources (v1alpha2 — all namespaced)
- Organization (with displayName override)
- Project (with organizationRef/organizationId, cross-namespace ref)
- OIDCApp (with projectRef/projectId, Secret output, full drift detection)
- MachineUser (with organizationRef/organizationId, key generation)

### Architecture
- Config file (`--config`) replaces CLI flags
- `watchNamespaces` support via K8s cache scoping
- Namespace-based multi-operator isolation (Role/RoleBinding per namespace)
- `GenerationChangedPredicate` on all controllers (no hot-loops)
- Conditional status updates (no write if unchanged)
- Graceful retry for transient ref-not-ready errors (RequeueAfter: 10s)
- Finalizer-based cleanup on deletion

### Integration Tests (9 scenarios)
- Organization lifecycle (create, verify, delete with finalizer)
- Organization with custom display name
- Project with default org (config.defaultOrganizationId)
- Project with organizationRef (cross-resource resolution)
- Project with explicit organizationId
- OIDCApp with projectRef (full lifecycle + Secret)
- OIDCApp drift detection (redirect URIs, token type, role assertions)
- MachineUser lifecycle (create, key generation, Secret)
- MachineUser with organizationRef

---

## Future

### Extended Resources (INF-368)
- [ ] IdentityProvider (org-scoped)
- [ ] LoginPolicy (org-scoped)
- [ ] UserGrant
- [ ] ActionTarget + ActionExecution (Actions v2)
- [ ] EmailProvider + SmsProvider

### Production Hardening
- [ ] Structured conditions on status (Ready, Synced, Error)
- [ ] Exponential backoff for persistent Zitadel API errors
- [ ] Debounce rapid spec updates
- [ ] Metrics (reconcile duration, error rate, Zitadel API latency)

### Ecosystem Integration
- [ ] Examples: Actions v2 wiring with zitadel-rbac-mapper
- [ ] Examples: MachineUser for CI/CD bots

---

## Design Principles

1. **Explicit over implicit** — every resource declares its organization
2. **Hierarchical references** — Organization → Project → App/User
3. **Operator-per-instance** — bound to one Zitadel instance via config file
4. **Namespace-based isolation** — multiple operators via namespace scoping + K8s RBAC
5. **No CRD for connection** — instance config is deployment config, not a reconciled resource
6. **Terraform parity** — resource coverage matches the Terraform provider

## Testing Model

| Package | Purpose | Deps | Command |
|---------|---------|------|---------|
| `internal/config/` | Config loader unit tests | None | `go test ./internal/config/...` |
| `tests/integration/` | Full reconcile loop (envtest + real Zitadel) | Docker-free (envtest), `~/.config/zitadel-operator/config.yaml` | `go test -tags=integration ./tests/integration/...` |
