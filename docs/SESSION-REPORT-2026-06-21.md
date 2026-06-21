# Session Report — 2026-06-21

**Repository:** `truvity/zitadel-operator`  
**Commit:** `42f2439` (pushed to master, CI running)  
**Duration:** Full day session  
**Scope:** Extend operator from 17 CRDs to 42 CRDs, covering all non-IdP Zitadel Terraform provider resources

---

## What Was Done

### Summary

Added 25 new Custom Resource Definitions with full reconcilers, integration tests, Helm RBAC, and documentation. The operator now covers every Zitadel resource except typed IdPs (GitLab, Azure AD, Apple, SAML, JWT, LDAP) and deprecated v1 Actions.

### Starting State

- 17 CRDs (Organization, Project, OIDCApp, APIApp, SAMLApp, ApplicationKey, PersonalAccessToken, MachineUser, UserGrant, ActionTarget, ActionExecution, ProjectMember, ProjectGrantMember, OrgMetadata, Domain, ProjectGrant, IdentityProvider)
- ~27 integration tests
- Helm chart with RBAC for 17 resources

### Ending State

- 42 CRDs
- 75 integration tests (all passing against real Zitadel Cloud)
- Helm chart with RBAC for 42 resources
- 0 lint issues, 0 vulnerabilities, build clean
- `just check` passes fully (build + test + lint + vuln + verify-generate)
- Pushed to master as commit `42f2439`

---

## New Resources Added (25 total)

### Instance-Level (IAM_OWNER, Admin API) — 17 new

| CRD | API Methods | Delete Behavior |
|-----|-------------|-----------------|
| DefaultLoginPolicy | GetLoginPolicy / UpdateLoginPolicy | Reset to safe defaults |
| DefaultDomainPolicy | GetDomainPolicy / UpdateDomainPolicy | Reset to safe defaults |
| DefaultLockoutPolicy | GetLockoutPolicy / UpdateLockoutPolicy | Reset to 0/0 |
| DefaultPasswordComplexityPolicy | GetPasswordComplexityPolicy / UpdatePasswordComplexityPolicy | Reset to 8/true/true/true/false |
| DefaultPasswordAgePolicy | GetPasswordAgePolicy / UpdatePasswordAgePolicy | Reset to 0/0 |
| DefaultNotificationPolicy | GetNotificationPolicy / UpdateNotificationPolicy | Reset to false |
| DefaultLabelPolicy | GetLabelPolicy / UpdateLabelPolicy + ActivateLabelPolicy | Reset to empty + activate |
| DefaultPrivacyPolicy | GetPrivacyPolicy / UpdatePrivacyPolicy | Reset to empty strings |
| DefaultOIDCSettings | GetOIDCSettings / UpdateOIDCSettings | Reset to 12h/12h/720h/2160h |
| DefaultMessageText | SetDefault*MessageText (10 types) | ResetCustom*MessageTextToDefault |
| GoogleIdP | AddGoogleProvider / UpdateGoogleProvider | RemoveIDPFromLoginPolicy |
| GitHubIdP | AddGitHubProvider / UpdateGitHubProvider | RemoveIDPFromLoginPolicy |
| EmailProvider | Add/Update EmailProviderSMTP or HTTP + Activate | RemoveEmailProvider |
| SmsProvider | Add/Update SMSProviderTwilio or HTTP + Activate | RemoveSMSProvider |
| InstanceMember | AddIAMMember / UpdateIAMMember | RemoveIAMMember |

### Organization-Level (ORG_OWNER, Management API) — 8 new

| CRD | API Methods | Delete Behavior |
|-----|-------------|-----------------|
| LoginPolicy | AddCustomLoginPolicy / UpdateCustomLoginPolicy | ResetLoginPolicyToDefault |
| LockoutPolicy | AddCustomLockoutPolicy / UpdateCustomLockoutPolicy | ResetLockoutPolicyToDefault |
| PasswordComplexityPolicy | AddCustomPasswordComplexityPolicy / UpdateCustomPasswordComplexityPolicy | ResetPasswordComplexityPolicyToDefault |
| PasswordAgePolicy | AddCustomPasswordAgePolicy / UpdateCustomPasswordAgePolicy | ResetPasswordAgePolicyToDefault |
| NotificationPolicy | AddCustomNotificationPolicy / UpdateCustomNotificationPolicy | ResetNotificationPolicyToDefault |
| LabelPolicy | AddCustomLabelPolicy / UpdateCustomLabelPolicy + Activate | ResetLabelPolicyToDefault |
| PrivacyPolicy | AddCustomPrivacyPolicy / UpdateCustomPrivacyPolicy | ResetPrivacyPolicyToDefault |
| HumanUser | AddHumanUser (User v2 API) / DeleteUser | DeleteUser |
| OrgMember | AddOrgMember / UpdateOrgMember | RemoveOrgMember |
| MessageText | SetCustom*MessageText (10 types) | ResetCustom*MessageTextToDefault |

---

## Architecture Decisions Made

### 1. Default/Org Paired Policy Pattern

Every Zitadel policy exists at two levels. We use shared field structs (`json:",inline"`) to DRY the fields:

```go
// Shared (policy_fields.go)
type LockoutPolicyFields struct { ... }

// Instance-default
type DefaultLockoutPolicySpec struct { LockoutPolicyFields `json:",inline"` }

// Org-scoped (adds org resolution)
type LockoutPolicySpec struct {
    OrganizationRef *ResourceRef
    OrganizationId  string
    LockoutPolicyFields `json:",inline"`
}
```

### 2. Type Discriminator Pattern (MessageText)

Instead of 10+ CRDs for each message type, we use 2 CRDs with a `type` enum:
- `DefaultMessageText` (instance-level, Admin API)
- `MessageText` (org-level, Management API)

The reconciler switches on `spec.type` to select the correct API method.

### 3. SMS Field Warning

The `verifySmsOtp` message type only uses `language` + `text` (no email fields). Instead of a separate CRD or blocking validation, the operator sets a non-blocking `SmsFieldWarning` condition if email-only fields are set.

### 4. Secret References

All sensitive values use `SecretKeyRef` (name + key) referencing a K8s Secret. If the Secret doesn't exist, the controller sets `Ready=False` with reason `SecretNotFound` and requeues at 10s.

### 5. Singleton Semantics

Instance-default policies are singletons — the policy always exists in Zitadel. The operator reads → diffs → updates on drift. Multiple CRs for the same singleton results in last-writer-wins (both become Ready).

---

## Files Created/Modified

### Type definitions (api/v1alpha2/) — 21 new files
- `defaultloginpolicy_types.go`, `defaultdomainpolicy_types.go`, `googleidp_types.go`, `githubidp_types.go`
- `loginpolicy_types.go`, `lockoutpolicy_types.go`, `passwordcomplexitypolicy_types.go`
- `emailprovider_types.go`, `smsprovider_types.go`, `humanuser_types.go`, `orgmember_types.go`
- `instancemember_types.go`, `labelpolicy_types.go`, `notificationpolicy_types.go`
- `passwordagepolicy_types.go`, `privacypolicy_types.go`, `defaultoidcsettings_types.go`
- `defaultlockoutpolicy_types.go`, `defaultpasswordcomplexitypolicy_types.go`
- `defaultpasswordagepolicy_types.go`, `defaultnotificationpolicy_types.go`
- `defaultlabelpolicy_types.go`, `defaultprivacypolicy_types.go`
- `defaultmessagetext_types.go`, `messagetext_types.go`
- `policy_fields.go` (shared field structs)

### Controllers (internal/controller/) — 25 new files
One per CRD, following the established pattern (get → finalizer → resolve deps → ensure resource → status update).

### Integration tests (tests/integration/) — 6 new files
- `policy_test.go` — first 7 CRDs + negative cases + E2E idpRef resolution
- `batch2_test.go` — HumanUser, OrgMember, InstanceMember, LabelPolicy, NotificationPolicy, PasswordAgePolicy, SmsProvider, GitHubIdP
- `defaultpolicies_test.go` — 5 Default* policies + drift detection + duplicate creation
- `negative_test.go` — org/user/secret ref not ready, invalid specs
- `privacy_oidc_test.go` — DefaultPrivacyPolicy, PrivacyPolicy, DefaultOIDCSettings
- `messagetext_test.go` — DefaultMessageText, MessageText

### Modified files
- `api/v1alpha2/groupversion_info.go` — registered all 42 types
- `cmd/operator/main.go` — registered all 42 controllers
- `tests/integration/main_test.go` — registered all 42 controllers for envtest
- `tests/integration/helpers_test.go` — `isReady()` handles all 42 types
- `charts/zitadel-operator/templates/clusterrole.yaml` — 42 resources × 3 sections
- `charts/zitadel-operator/templates/role.yaml` — same
- `.golangci.yml` — suppressed SA1019 (deprecated SDK) and ST1003 (CRD naming)
- `Justfile` — added `export GOWORK := "off"` for parent workspace compatibility
- `README.md` — 42 CRDs, catalog tables, GitHub badges
- `ROADMAP.md` — v0.13.0, updated Future
- `docs/DESIGN.md` — CRD Design Patterns section (3 patterns)
- `docs/TEST-SCENARIOS.md` — sections 16-19

### New files
- `CHANGELOG.md` — v0.13.0, v0.12.0, v0.11.0

---

## Integration Test Coverage

### Positive tests (all include create → update → delete + status checks)
Every CRD has at least one lifecycle test with mutation step.

### Negative tests
| Category | Tests |
|----------|-------|
| Org ref not ready | LoginPolicy, PasswordComplexityPolicy, LockoutPolicy, PasswordAgePolicy, NotificationPolicy, LabelPolicy, PrivacyPolicy, HumanUser |
| User ref not ready | OrgMember, InstanceMember |
| Secret not found | GoogleIdP, GitHubIdP, EmailProvider SMTP, SmsProvider Twilio |
| IdP ref not ready | DefaultLoginPolicy |
| Invalid spec | EmailProvider (both smtp+http), SmsProvider (both twilio+http) |
| Singleton drift | DefaultLockoutPolicy (external API change → operator corrects) |
| Singleton duplicate | DefaultLockoutPolicy (two CRs → both Ready, last-writer-wins) |

### Test execution
All 75 tests run against real Zitadel Cloud instance (`zitadel-operator-tests-pylwi3.eu1.zitadel.cloud`) using envtest. JWT key from system keyring. GitHub IdP test uses OAuth App credentials from keyring (skips gracefully if not configured).

---

## Configuration Required for Tests

### System keyring (secret-tool)
| Service | Key | Value |
|---------|-----|-------|
| `zitadel-operator` | `jwt-key` | Zitadel SA JWT key JSON |
| `zitadel-operator` | `github-idp-client-id` | GitHub OAuth App Client ID |
| `zitadel-operator` | `github-idp-secret` | GitHub OAuth App Client Secret |

### Config file
```yaml
# ~/.config/zitadel-operator/config.yaml
domain: zitadel-operator-tests-pylwi3.eu1.zitadel.cloud
port: "443"
insecure: false
defaultOrganizationId: "376735634926216553"
```

---

## What's NOT Done (Intentionally)

1. **Typed IdPs** (GitLab, Azure AD, Apple, SAML, JWT, LDAP, GitHub Enterprise Server) — marked in ROADMAP as "request feature if needed". We don't have test infrastructure for these providers.

2. **Deprecated v1 Actions** (`zitadel_trigger_actions`) — replaced by Actions v2 (ActionTarget + ActionExecution already implemented).

3. **Git tag / release** — commit pushed to master for CI validation, no tag yet. Tag after CI passes.

---

## Known Issues / Tech Debt

1. **Zitadel SDK deprecated methods** — `AddHumanUser`, `AddOrgMember`, `AddIAMMember` etc. are marked deprecated in favor of v2 APIs that aren't stable yet. Suppressed via `.golangci.yml` `-SA1019` check. When Zitadel v2 APIs stabilize, these should be migrated.

2. **No webhook validation** — CRD validation is only via kubebuilder markers (enum constraints, required fields). No admission webhook for complex validation (e.g., preventing both organizationRef AND organizationId from being set).

3. **No exponential backoff** — transient Zitadel API errors use fixed 10s requeue. ROADMAP item for production hardening.

4. **No Prometheus metrics** — no custom reconcile counters or API latency histograms. ROADMAP item.

---

## How to Continue

### If CI fails
The CI workflow (`ci.yaml`) runs `devbox run -- just check`. If it fails:
1. Most likely cause: devbox environment difference (controller-gen version, Go version)
2. Check if `GOWORK=off` is respected in the CI environment
3. The Justfile now exports `GOWORK=off` globally, so it should work

### To release
```bash
git tag v0.13.0
git push origin v0.13.0
```
GoReleaser will build multi-arch images, push to GHCR, package Helm charts.

### To add typed IdPs later
Follow the GoogleIdP/GitHubIdP pattern:
1. Create `api/v1alpha2/<provider>idp_types.go` (same struct pattern)
2. Create `internal/controller/<provider>idp_controller.go` (same Admin API pattern)
3. Register in groupversion_info.go, main.go, main_test.go, helpers_test.go
4. Add to Helm RBAC
5. Run `just generate`
6. Write integration test (requires provider-specific test credentials in keyring)
