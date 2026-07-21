# Zitadel Operator Roadmap

## Current: v0.18 (v1alpha2, unreleased) — see [CHANGELOG.md](CHANGELOG.md)

v0.18 adds scope maps (`ScopeMap`), internal delegation, binding
levels, dual-serving, and SSA status writes — documented under
[docs/architecture/](docs/architecture/resource-hierarchy.md).

### Implemented Resources (43 CRDs)

#### Project-Level (PROJECT_OWNER)
- OIDCApp — OIDC application with Secret output (confidential/public, drift detection)
- APIApp — API/M2M application with Secret output (basic/private_key_jwt)
- SAMLApp — SAML application (metadata XML or URL)
- ApplicationKey — JWT key for applications (Secret output)
- ProjectMember — Assign user roles on a project
- ProjectGrantMember — Assign user roles on a project grant

#### Organization-Level (ORG_OWNER)
- Organization — Create/manage organizations
- Project — Create/manage projects with role sync
- MachineUser — Service accounts with JWT key generation
- PersonalAccessToken — PAT for machine users (Secret output)
- UserGrant — Assign project roles to users
- ProjectGrant — Share a project with another organization
- OrgMetadata — Key-value metadata on the org
- Domain — Register org domain for domain discovery
- IdentityProvider — Configure generic OIDC identity provider
- LoginPolicy — Org-scoped login policy (custom override of instance default)
- PasswordComplexityPolicy — Org-scoped password complexity policy
- LockoutPolicy — Org-scoped lockout policy
- HumanUser — Human user lifecycle management (User v2 API)
- OrgMember — Org-level role assignment
- LabelPolicy — Org-scoped branding/label policy (activate after update)
- NotificationPolicy — Org-scoped notification policy
- PasswordAgePolicy — Org-scoped password age policy
- PrivacyPolicy — Org-scoped privacy policy (custom override of instance default)
- MessageText — Org-scoped custom message text (type discriminator, Management API)

#### Instance-Level (IAM_OWNER)
- ActionTarget — Webhook targets for Actions v2
- ActionExecution — Bind targets to trigger conditions
- DefaultLoginPolicy — Instance default login policy (singleton, drift detection)
- DefaultDomainPolicy — Instance default domain policy (singleton, drift detection)
- DefaultLockoutPolicy — Instance default lockout policy (singleton, drift detection)
- DefaultPasswordComplexityPolicy — Instance default password complexity (singleton, drift detection)
- DefaultPasswordAgePolicy — Instance default password age (singleton, drift detection)
- DefaultNotificationPolicy — Instance default notification policy (singleton, drift detection)
- DefaultLabelPolicy — Instance default label/branding policy (singleton, activate after update)
- DefaultPrivacyPolicy — Instance default privacy policy (singleton, drift detection)
- DefaultOIDCSettings — Instance default OIDC settings (token lifetimes, singleton, drift detection)
- GoogleIdP — Instance-scoped Google identity provider (Admin API, exposes status.idpID)
- GitHubIdP — Instance-scoped GitHub identity provider (same pattern as GoogleIdP)
- EmailProvider — Email delivery provider (SMTP or HTTP webhook, activated after create)
- SmsProvider — SMS delivery provider (Twilio or HTTP, activated after create)
- InstanceMember — Instance-level role assignment (IAM_OWNER, IAM_ORG_MANAGER, etc.)
- DefaultMessageText — Instance default message text (type discriminator, Admin API)

### Architecture
- Config file (`--config`) with single YAML
- `watchNamespaces` for namespace isolation
- Helm chart with ConfigMap + Secret volume mounts
- Namespace-scoped RBAC (Role/RoleBinding per namespace)
- `GenerationChangedPredicate` on all controllers (no hot-loops)
- Conditional status updates
- Structured status conditions (`Ready=True/False` with reason codes)
- Graceful retry for transient ref-not-ready errors (10s requeue)
- Finalizer-based cleanup on deletion

### Integration Tests
100 test functions against a real Zitadel instance via envtest — negative
cases, condition verification, idempotent reconcile, drift, delegation,
dual-serving. Catalog: [docs/TEST-SCENARIOS.md](docs/TEST-SCENARIOS.md);
harness: [docs/development/integration-tests.md](docs/development/integration-tests.md).

## Future

### Additional Resources
- [ ] Typed IdPs (GitLab, Azure AD, Apple, SAML, JWT, LDAP) — request feature if needed

## Design Principles

1. **Explicit over implicit** — every resource declares its organization
2. **Hierarchical references** — Organization → Project → App/User
3. **Operator-per-instance** — bound to one Zitadel instance via config file
4. **Namespace-based isolation** — multiple operators via namespace scoping + K8s RBAC
5. **No CRD for connection** — instance config is deployment config, not a reconciled resource
6. **Terraform provider parity** — resource coverage matches the Zitadel Terraform provider for org/project scope

### Tech Debt
- [ ] Migrate deprecated SDK calls to v2 APIs once stable (SA1019 nolint markers): HumanUser (AddHumanUser→CreateUser), OrgMember/InstanceMember (permission service v2), MachineUser, ApplicationKey, Domain, OrgMetadata, PersonalAccessToken, Project roles, ProjectGrant, ProjectGrantMember, ProjectMember, UserGrant

### Future Validation — GitOps Composition Coverage

The existing integration suite covers per-CRD lifecycle (create/update/delete) in isolation. These planned scenarios validate the **composed flows** a GitOps deployment actually applies. Priority: 1–3 are the GitOps-confidence set (must pass before Pulumi→operator migration cutover); 4–5 are follow-ups.

#### 1. Brownfield Adoption (highest priority — validate before migration)

Pre-create a Project + OIDCApp via the Zitadel API (simulating existing Pulumi-managed resources), then apply matching CRs; the operator must adopt them in place — no duplicate, and the OIDCApp retains its existing `client_id`.

**Why it matters:** The planned retirement of the Pulumi ZITADEL stack assumes the operator can take over already-existing resources without recreating them. Recreate would churn every `client_id`/`client_secret` and break bound apps (kubelogin, ArgoCD, etc.).

**⚠️ May surface a missing capability:** The current adoption logic matches by display name. If the CRD `metadata.name` differs from the Zitadel resource name, adoption may fail. This scenario may require a new feature: match-by-ID or an `adopt` annotation (`zitadel.truvity.io/adopt-id: "<existing-zitadel-id>"`). Record as a feature item if the test reveals adoption gaps.

**Must be validated before migration cutover.**

#### 2. Apply-All-at-Once / Out-of-Order Convergence

Apply a full bundle (Organization → Project → roles → OIDCApp → MachineUser → ProjectMember → policies → IdP) in shuffled/reverse order in a single `kubectl apply`, as ArgoCD does. Assert eventual all-Ready with no wedge or crash — parents-not-ready handled by requeue.

**Validates:** Order-agnostic convergence at bundle scale. Tests that the requeue-on-ref-not-ready pattern works end-to-end across the full dependency graph without requiring wave annotations or sync ordering.

#### 3. Per-Cluster Onboarding Bundle

Project → roles → kubelogin OIDCApp → scoped MachineUser → ProjectMember(PROJECT_OWNER). Assert both output Secrets (app `client_id`/`client_secret` and machine `key.json`) materialize with stable key names.

**Validates:** The most-repeated onboarding flow (one per K8s cluster). Confirms the secret-output contract that downstream ExternalSecrets PushSecret depends on — key names must be stable across reconcile cycles.

#### 4. Full Instance-Global Bootstrap Bundle (follow-up)

All Default* policies + GoogleIdP + EmailProvider + Organization + console UserGrant applied together. Assert all-Ready and end-to-end `idpRef` resolution (GoogleIdP → DefaultLoginPolicy).

**Validates:** The kernel global operator reconciling a fresh Zitadel instance from scratch. Mirrors the ArgoCD App-of-Apps that bootstraps instance identity.

#### 5. Composite Drift Correction (follow-up)

Out-of-band edit of a referenced resource in Zitadel (e.g., change an OIDCApp's redirect URIs in the console). Operator reverts to spec on the next periodic requeue.

**Validates:** Drift enforcement for non-singleton resources (extends the existing DefaultLockoutPolicy drift test to app-level resources). Confirms the operator is the source of truth for all managed resources.

## Testing Model

| Package              | Purpose                                      | Command                                             |
| -------------------- | -------------------------------------------- | --------------------------------------------------- |
| `internal/config/`   | Config loader unit tests                     | `go test ./internal/config/...`                     |
| `tests/integration/` | Full reconcile loop (envtest + real Zitadel) | `go test -tags=integration ./tests/integration/...` |
