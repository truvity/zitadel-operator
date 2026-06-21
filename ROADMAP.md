# Zitadel Operator Roadmap

## Current: v0.13.0 (v1alpha2)

### Implemented Resources (42 CRDs)

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

### Integration Tests (27 scenarios)
All tests run against a real Zitadel Cloud instance via envtest.
Includes negative cases, condition verification, idempotent reconcile, and edge cases.

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

## Testing Model

| Package              | Purpose                                      | Command                                             |
| -------------------- | -------------------------------------------- | --------------------------------------------------- |
| `internal/config/`   | Config loader unit tests                     | `go test ./internal/config/...`                     |
| `tests/integration/` | Full reconcile loop (envtest + real Zitadel) | `go test -tags=integration ./tests/integration/...` |
