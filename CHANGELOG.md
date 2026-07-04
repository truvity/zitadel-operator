# Changelog

All notable changes to the zitadel-operator are documented here.

## [0.15.0] — 2026-07-04

### Added

#### ActionTarget: `targetType` and `payloadType` fields

The ActionTarget CRD now exposes full Actions V2 target configuration, making webhook setup fully declarative and survivable across cluster rebuilds without manual intervention.

- **`targetType`** — enum: `restCall` (default), `restWebhook`, `restAsync`
  - `restCall`: Zitadel reads the response body (required for `append_claims`)
  - `restWebhook`: only checks status code, ignores response body
  - `restAsync`: fire-and-forget, no response wait (for event executions)
- **`payloadType`** — enum: `json` (default), `jwt`, `jwe`
  - `json`: JSON body with `X-ZITADEL-Signature` header
  - `jwt`: signed JWT body (receiver verifies via JWKS)
  - `jwe`: encrypted JWT body

Both fields have kubebuilder defaults — existing CRs without these fields continue working unchanged (backward-compatible).

### Changed

- ActionTarget controller now passes `targetType` and `payloadType` on both create and update, ensuring reconciliation always enforces the declared state
- Integration tests updated to exercise `targetType: restCall` + `payloadType: jwt` with `function: preuserinfo` condition (validates the RBAC mapper webhook scenario end-to-end)

### Fixed

- Previously, the controller hardcoded `restCall` target type with no `payloadType` (defaulting to JSON). After a cluster rebuild, manually-configured JWT payload type was lost, breaking JWKS verification on the webhook handler. Now the operator fully manages these fields.

## [0.14.0] — 2026-06-27

### Fixed
- `createOIDCApp` — corrected return order (was swapping appID/clientID/secret)

## [0.13.0] — 2026-06-21

### Added

#### Instance-Level Resources (IAM_OWNER, Admin API)
- **DefaultLoginPolicy** — instance default login policy (singleton, drift detection, idpRef resolution)
- **DefaultDomainPolicy** — instance default domain policy (singleton, drift detection)
- **DefaultLockoutPolicy** — instance default lockout policy (singleton, drift detection)
- **DefaultPasswordComplexityPolicy** — instance default password complexity (singleton, drift detection)
- **DefaultPasswordAgePolicy** — instance default password age (singleton, drift detection)
- **DefaultNotificationPolicy** — instance default notification policy (singleton, drift detection)
- **DefaultLabelPolicy** — instance default label/branding policy (singleton, activate after update)
- **DefaultPrivacyPolicy** — instance default privacy policy (singleton, drift detection)
- **DefaultOIDCSettings** — instance default OIDC settings (token lifetimes, singleton, drift detection)
- **GoogleIdP** — instance-scoped Google identity provider (exposes `status.idpID` for cross-resource refs)
- **GitHubIdP** — instance-scoped GitHub identity provider (same pattern as GoogleIdP)
- **EmailProvider** — instance-level email provider (discriminated SMTP/HTTP, activate after create)
- **SmsProvider** — instance-level SMS provider (discriminated Twilio/HTTP, activate after create)
- **InstanceMember** — instance-level role assignment (IAM_OWNER, IAM_ORG_MANAGER, etc.)
- **DefaultMessageText** — instance default message text with type discriminator (init, passwordReset, verifyEmail, etc.)

#### Organization-Level Resources (ORG_OWNER, Management API)
- **LoginPolicy** — org-scoped login policy (custom override of instance default)
- **PasswordComplexityPolicy** — org-scoped password complexity policy
- **LockoutPolicy** — org-scoped lockout policy
- **PasswordAgePolicy** — org-scoped password age policy
- **NotificationPolicy** — org-scoped notification policy
- **LabelPolicy** — org-scoped label/branding policy (activate after update)
- **PrivacyPolicy** — org-scoped privacy policy (custom override of instance default)
- **HumanUser** — human user lifecycle management (User v2 API)
- **OrgMember** — org-level role assignment
- **MessageText** — org-scoped custom message text with type discriminator (init, passwordReset, verifyEmail, etc.)

#### Singleton Governance
- **Reset-on-delete annotation** (`zitadel.truvity.io/reset-on-delete: "true"`) — opt-in reset of instance-default policies to Zitadel baseline values on CR deletion. Default behavior: leave instance state untouched.
- **Singleton conflict detection** — only the earliest-created CR per Default* kind manages the instance. Duplicates get `Ready=False, reason=DuplicateSingleton`.

#### Infrastructure
- Shared policy field structs (`policy_fields.go`) — DRY between org-scoped and instance-default variants
- `SecretKeyRef` type for consistent secret reference pattern across IdPs and providers
- `resolveUserIdIncludingHuman()` — resolves both MachineUser and HumanUser refs
- `shouldResetOnDelete()` helper — annotation-based opt-in for reset behavior

### Changed
- CRD count: 17 → 42 (25 new resources)
- `.golangci.yml` — removed global SA1019 suppression; deprecated SDK calls now have per-site `//nolint:staticcheck` with migration rationale
- Helm RBAC — extended ClusterRole and Role for all 39 resource types
- Default* controllers — delete no longer resets instance state by default (opt-in via annotation)

### Integration Tests
- 33 new integration test cases (happy path + negative) against real Zitadel Cloud
- Negative cases: secret not found, org/user ref not ready, invalid discriminated specs
- End-to-end: GoogleIdP → DefaultLoginPolicy `idpRef` resolution
- GitHubIdP tests load credentials from system keyring (skip gracefully if not configured)

## [0.12.0] — 2026-06-14

### Added
- ProjectGrantMember CRD
- IdentityProvider CRD (generic OIDC, org-scoped)
- APIApp CRD
- SAMLApp CRD  
- ApplicationKey CRD
- PersonalAccessToken CRD
- ProjectGrant CRD
- Domain CRD
- OrgMetadata CRD
- Project scope validation (`projectScopeLabel` config)

### Changed
- Hot-loop fix: `GenerationChangedPredicate` on all controllers
- Conditional status updates (no unnecessary writes)
- Full drift detection for OIDCApp (all mutable fields)

## [0.11.0] — 2026-05-28

### Added
- Initial v1alpha2 release with 17 CRDs
- Organization, Project, OIDCApp, MachineUser, UserGrant (core)
- ActionTarget, ActionExecution (Actions v2)
- ProjectMember, ProjectGrantMember
- Config file (`--config` flag), namespace isolation, Helm charts
- envtest-based integration test harness
