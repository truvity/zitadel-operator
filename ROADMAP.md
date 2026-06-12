# Zitadel Operator Roadmap

## Current: v0.4.x (v1alpha1 — maintenance mode)

Bug fixes only. No new features on v1alpha1 API.

### Implemented Resources
- Organization (cluster-scoped)
- Project (cluster-scoped) with roles
- OIDCApp (namespaced) with full OIDC config
- MachineUser (namespaced) with key management
- IdentityProvider (cluster-scoped)
- LoginPolicy (cluster-scoped)
- ProjectGrant (namespaced)
- UserGrant (namespaced)
- ProjectMember (namespaced)
- ApplicationKey (namespaced)
- PersonalAccessToken (namespaced)
- PasswordComplexityPolicy (cluster-scoped)
- LockoutPolicy (cluster-scoped)
- Action (namespaced)
- InstanceMember (namespaced)
- OrgMember (namespaced)

### Known Issues
- **INF-362**: OIDCApp reconcile hot-loop (~2s interval instead of 5min)
- **INF-356**: Pulumi/Operator split (dual management of some apps)
- **INF-357/358**: No org-scoped resource support

---

## Next: v1.0.0 (v1alpha2 — architecture redesign)

**Ticket:** INF-363  
**Design:** [docs/DESIGN-v2.md](docs/DESIGN-v2.md)

### Step 0: Test Harness (REPL development mode)
- [ ] `~/.config/zitadel-operator/config.yaml` loader
- [ ] e2e-framework TestMain (Kind cluster up/down)
- [ ] CRD install, one trivial test (Project lifecycle)

### Step 1: Fix Existing Code Under Tests (v1alpha1, ship v0.5.0)
- [ ] Hot-loop fix (INF-362): GenerationChangedPredicate + conditional status
- [ ] Full OIDCApp drift detection (all mutable fields)
- [ ] Integration tests: Project + OIDCApp lifecycle

### Step 2: v1alpha2 Core Resources
- [ ] v1alpha2 Organization (namespaced) with ref/id support
- [ ] v1alpha2 Project (namespaced) with organizationRef/organizationId
- [ ] v1alpha2 OIDCApp with projectRef/projectId + Secret cleanup + observe mode
- [ ] v1alpha2 MachineUser

### Step 3: Operator Infrastructure
- [ ] Config file loader (`--config` flag, YAML)
- [ ] Namespace-scoped watching (`watchNamespaces`)
- [ ] Cross-namespace ref resolution

### Step 4: Extended Resources
- [ ] IdentityProvider org-scoped (INF-357)
- [ ] LoginPolicy org-scoped (INF-357)
- [ ] UserGrant
- [ ] ActionTarget + ActionExecution (Actions v2)
- [ ] EmailProvider + SmsProvider

### Step 4b: Ecosystem Repos
- [ ] Create `truvity/zitadel-rbac-mapper` repo (public, MIT, devbox, Justfile, GoReleaser, GH Actions)
  - Skeleton: LICENSE, README, .gitignore, devbox.json, .envrc, Justfile, go.mod
  - .goreleaser.yaml (multi-arch binaries + ko images + Lambda ZIPs)
  - .github/workflows/ (ci.yml + release.yml)
  - charts/zitadel-rbac-mapper/ (Helm chart referencing multi-arch GHCR image)
  - deploy/example/ (Pulumi Go program: Lambda from GitHub Release + K8s from Helm)
- [ ] Create `truvity/zitadel-notify-relay` repo (public, MIT, devbox, Justfile, GoReleaser, GH Actions)
  - Same skeleton as above
  - Initial backends: AWS SES (email) + AWS SNS (SMS)
  - Extensible provider interface for future delivery backends
  - deploy/example/ (Pulumi Go program: Lambda + SES/SNS IAM)
- [ ] Create `truvity/google-group-sync` repo (public, MIT, devbox, Justfile, GoReleaser, GH Actions)
  - Same skeleton as above
  - deploy/example/ (Pulumi Go program: Lambda + CronJob patterns)
- [ ] Implement rbac-mapper (groups→grants mapping webhook)
- [ ] Implement notify-relay (Zitadel HTTP provider → AWS SES/SNS delivery)
- [ ] Implement google-group-sync (Google Directory API fetcher)
- [ ] Add examples/ in zitadel-operator showing Actions v2 + HTTP provider wiring with these repos

### Step 5: Error Handling + Release
- [ ] Error states, finalizer edge cases, debounce
- [ ] Migration script v1alpha1 → v1alpha2
- [ ] Documentation
- [ ] Release v1.0.0-alpha1

---

## Future (post-v1.0)

### Actions v2
- ActionTarget — webhook/HTTP target with signing key (org-scoped)
- ActionExecution — condition → targets binding (org-scoped)

### Messaging & Notification Providers (instance-level)
- EmailProvider — type: smtp (host/port/tls/credentials) or http (webhook endpoint)
- SmsProvider — type: twilio (sid/token/sender) or http (webhook endpoint)

### Other Resources
- ApplicationSaml — SAML application type
- Domain — organization domain management
- LabelPolicy — branding and theming
- PrivacyPolicy — privacy links and ToS
- DomainPolicy — domain validation rules
- NotificationPolicy — notification settings
- DefaultOidcSettings — instance-level OIDC settings
- IdpAzureAd — Azure AD identity provider
- IdpLdap — LDAP identity provider
- ProjectGrantMember — project grant member management

---

## Design Principles

1. **Explicit over implicit** — every resource declares its organization
2. **Hierarchical references** — Organization → Project → App/User
3. **Operator-per-instance** — bound to one Zitadel instance via startup config
4. **Namespace-based isolation** — multiple operators via namespace scoping + K8s RBAC
5. **No CRD for connection** — instance config is deployment config, not a reconciled resource
6. **Terraform parity** — resource coverage and explicit scoping match the Terraform provider

## Testing Model

| Package | Purpose | Deps | Command |
|---------|---------|------|---------|
| `tests/unit/` | Logic, normalization, filtering | None | `go test ./tests/unit/...` |
| `tests/integration/` | Full reconcile loop (Kind + real Zitadel) | Docker, `~/.config/zitadel-operator/config.yaml` | `go test -tags=integration ./tests/integration/...` |
