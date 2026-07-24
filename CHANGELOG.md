# Changelog

All notable changes to the zitadel-operator are documented here.

## [Unreleased]

## [0.19.2] — 2026-07-25

### Added

- **`OIDCApp` `spec.idTokenUserinfoAssertion`** (optional, default `false`): asserts the userinfo claims — including action-appended claims such as the rbac-mapper's `groups` — into the ID token at issuance. Required for consumers that read groups from the ID token (ArgoCD, kubelogin-style flows). Applied at creation and drift-corrected on every sync.

## [0.19.1] — 2026-07-24

### Fixed

- **Zitadel client calls are now bounded — a dead connection can no longer wedge a controller worker forever.** The SDK's JWT-profile auth fetches an OAuth token with an HTTP POST *per RPC*, on `context.Background()`, through a shared HTTP client with **no timeout**: when the pooled HTTP/2 connection to the token endpoint died silently (observed in production: a `connection reset by peer` followed by total controller silence — the single reconcile worker blocked inside the token fetch, healthz still green, restart re-wedged within a minute). Four layers now bound every call:
  - the token-endpoint HTTP client carries a hard 30 s request timeout;
  - its HTTP/2 transport health-checks pooled connections (`ReadIdleTimeout` 30 s / `PingTimeout` 15 s), so dead connections are closed and redialed;
  - tokens are cached via `oauth2.ReuseTokenSource` (one token fetch per expiry instead of one per RPC);
  - the gRPC channel gets keepalive pings (30 s / 20 s, also without active streams) and a default 60 s deadline on any unary call whose context has none.
  Both standard and split-horizon modes are covered. A stuck call now fails with an error, the reconcile requeues with backoff, and the operator self-heals.

## [0.19.0] — 2026-07-22 "fleet simplification"

A strictly **additive** minor release serving the fleet deployment shape — one org = one SA = one operator, no ScopeMaps — now documented as the recommended topology in [docs/install/deployment-shapes.md](docs/install/deployment-shapes.md). ScopeMaps, `iam-owner` bindings and delegation remain fully supported.

### Added

- **`ProjectRole` CRD.** One role key per CR, reconciled into a project named by `spec.projectRef` (a `Project` CR) or `spec.projectId` (raw ID, mutually exclusive — `InvalidSpec` fail-closed condition otherwise; project-scoped namespaces may omit both and inherit the scope project). `spec.key` defaults to the CR name, `displayName` defaults to the key, optional `group`. Create-or-adopt with `displayName`/`group` drift correction; a changed key replaces the old role (keys are immutable in Zitadel); deletion removes the role. Unlike `Project` `spec.roles` (an authoritative full-set sync), `ProjectRole` manages exactly its own key so several namespaces/charts can contribute roles to one project — the mechanical `{namespace}:{role}` vocabulary downstream. Do not combine both mechanisms for one project.
- **`MachineUser` explicit grant target.** New optional `spec.projectRef`/`spec.projectId` (mutually exclusive) name the project `spec.roles` is granted on — the fleet shape's replacement for the project-scope requirement. Precedence: explicit ref/ID → scope project → previously recorded `status.projectId`. Polite `ProjectNotReady` requeue while the referenced `Project` has no ID yet; a changed target drops the grant on the previous project. The `RolesRequireProjectScope` message now points at `projectRef`.
- **ForeignManager guard.** Two operator deployments serving the *same* instance have identical SSA field managers, so the v0.18 dual-serving gate cannot tell them apart when their namespace selections accidentally overlap. Each tenant CR is now stamped at adoption with `zitadel.truvity.io/managed-by: <management identity>` (`<instance>/org/<boundOrgId>` for org-owner, `<instance>/ns/<operatorNamespace>` for iam-owner). A non-matching operator sets a `ForeignManager` condition (dedicated `zitadel-guard/<identity>` field manager — never touches the owner's status ownership) and skips the CR entirely, including deletion (a same-instance foreign delete is not a no-op). Transfer ownership by editing/removing the annotation; existing field-manager strings are unchanged, so v0.18-managed resources keep their ownership.
- **`Project` settings applied and drift-corrected.** `spec.assertRolesOnAuth` and `spec.checkAuthorizationOnAuth` were declared but only honored at create time (and ignored on adoption); they are now applied at creation and drift-corrected on every sync via the project v2 API.
- `status.observedGeneration` on `Project` and `ProjectRole`.

### Documentation

- New [Deployment shapes](docs/install/deployment-shapes.md): the fleet topology as the recommended shape, `spec.instance` pins vs the ForeignManager guard, in-namespace `Project`/`ProjectRole` declaration, with ScopeMaps/iam-owner documented as supported alternatives.
- Regenerated CRD API reference; README/multi-operator pointers updated.

## [0.18.0] — 2026-07-21 (INF-422)

**BREAKING** — see [docs/MIGRATION-0.18.md](docs/MIGRATION-0.18.md) for the full v0.17 → v0.18 guide.

### Pre-release review changes (doc-review findings, applied before v0.18.0)

- **CRD renamed `ZitadelScopeMap` → `ScopeMap`** (kind, plural `scopemaps`, files, RBAC). The only prefixed CRD broke the naming convention; renamed while days old.
- **`spec.organization` is optional when `spec.organizationId` is set** (the ID is authoritative; a set-but-different name is drift). Same at rule level: `projectId` no longer requires a `project` name. At least one of organization name/ID is required.
- **Instance alias**: new optional `instanceAlias` config key — the operator's stable identity for `spec.instance` pins, `ScopeMap` assertions and the SSA field manager (`zitadel-operator/<alias>`), defaulting to `domain`. A later domain migration no longer orphans pins or managed fields.
- **Fail-fast when the operator namespace is undeterminable** (`operatorNamespace` unset and `POD_NAMESPACE` empty): previously scope maps silently disabled — fail-permissive for a security-relevant routing surface. Also enforced at startup: `watchNamespaces`, when set, must include the operator namespace; `keyFile` must be set.
- **Steady-state fail-closed backoff**: confirmed rejects (`NoMatchingRule`, `ScopeConflict`, `InstanceMismatch`) re-check on the 5-minute periodic interval instead of every 10s.
- **Long-lived drift is now a condition, not just an expiring Event**: `OrganizationNameDrift` and `BindingContained=False/ForeignOrganization` conditions on the map.
- **Permission-shaped delegation failures** (`PermissionDenied`/`Unauthenticated`) fail closed as conditions instead of counting as controller errors in metrics.
- **Delegation Secrets carry human-readable scope annotations** (`zitadel.truvity.io/scope-instance|org|project`).
- **Known limitation (org-owner):** the binding requires ORG_OWNER in exactly one org — one SA cannot back two org-scoped deployments; use one SA per deployment.
- Chart: disabling leader election now requires `leaderElection.acknowledgeDisabledRisk=true`; `config.port` string typing documented; `just test-integration` timeout fixed (30m); S-161 scenario text corrected to the implemented earliest-wins semantics.

### Removed (breaking, INF-428)

- **`defaultOrganizationId` config key removed.** There is no default scope: a namespace either resolves through a `ScopeMap` or (zero-maps passthrough aside) org-scoped CRs must name their organization explicitly. The operator fails fast at startup when the key is present.
- **`projectScopeLabel` config key removed.** Label-value-as-project routing is superseded by scope maps. Fail-fast at startup when present.
- `watchNamespaces` survives as the coarse informer filter only.

### Added

- **SSA status discipline (v0.18 prerequisite).** All status writes moved to Server-Side Apply with per-instance field manager `zitadel-operator/<domain>`; `conditions` are `listType=map` keyed by `type`. Fixes the condition-wipe bug found in the prototype (read-modify-write `Status().Update` silently dropped other writers' conditions) and makes two-writer dual-serving possible at all.
- **`ScopeMap` CRD + scope resolution (INF-423).** Namespaced CRD evaluated only in the operator's namespace; mandatory `spec.instance` assertion (fail-closed `InstanceMismatch`), selector XOR literal rules, `organizationId`/`projectId` authoritative with name-drift Events, first-match top-down across name-sorted maps, cross-map conflicts fail-closed with Warning Events on both maps, `MapsNotSynced` (transient) distinguished from `NoMatchingRule` (steady-state), zero maps = v0.17 passthrough (rollout gate). **All tenant reconcilers** route through scope resolution; project-scope rules default the project for tenant CRs (recorded in `status.projectId`).
- **Binding levels (INF-424).** Required `binding: iam-owner | org-owner` config assertion, verified at startup via `AuthService.ListMyMemberships` (crash on mismatch). Under `org-owner`: instance-level resources get `Ready=False / NotSupportedAtBindingLevel`; foreign-org scope maps are rejected with a `ForeignOrganization` Event.
- **Internal delegation (INF-425).** Per resolved scope the operator mints a scope-limited machine user (explicit `AddMachineUser` + `AddOrgMember(ORG_OWNER)` / project-create + `AddProjectMember(PROJECT_OWNER)`; never `ORG_PROJECT_CREATOR`) and reconciles tenant CRs with the delegated key. Key persisted to a labeled Secret (`zitadel-delegation-<hash>`) in the operator namespace *before* caching; warm restart from Secrets; lazy validity re-check with re-mint; eager revoke when a scope stops matching + periodic orphan GC; internal 90-day dual-key rotation with grace overlap. During deletion, resolution/delegation failure falls back to the binding client so finalizers cannot deadlock.
- **MachineUser extension (INF-426).** Optional `spec.roles` (user grant on the scope project, synced with drift detection), `spec.key.rotateAfter` + `rotationGrace` (dual-key rotation), key Secret upgraded to a connection bundle (`key.json`, `instanceUrl`, `issuer`, `orgId`, `projectId`, best-effort `instanceId`). Fully backward compatible.
- **Dual-serving (INF-422).** Optional `spec.instance` pin on all tenant CRs. Foreign pin ⇒ CR completely untouched; unset pin on a namespace served by two operators ⇒ both fail closed with `InstanceResolved=False / AmbiguousInstance` via SSA with distinct field managers.

### Changed

- **Leader election on by default (INF-427).** `--leader-elect` defaults to true; new `--leader-election-id` flag, set by the Helm chart from the release fullname so two deployments in one namespace get distinct leases.
- Helm chart: `config.binding` (required), `config.operatorNamespace` (defaults to release namespace), `scopemaps` RBAC, cluster-wide namespace reader whenever namespaced RBAC mode is used.

### Fixed

- **INF-400 root-caused and fixed:** redirect-URI list updates (and any OIDC config drift correction) never converged: `UpdateApplication` always carried the unchanged `Name`, which Zitadel's name-change command rejects with `No changes (COMMAND-2m8vx)` before applying the config update. The name is now sent only when it actually drifted.
- **INF-430 audit:** ApplicationKey re-mints (Secret lost) never persisted the new `status.keyId`, and PersonalAccessToken re-mints never persisted the new `status.tokenId` — deletion then revoked a stale ID and leaked the live credential. Both now persist the ID and revoke the replaced key/token at re-mint time. The OIDCApp/APIApp adoption-regeneration path (0.16.0) is covered by a dedicated integration test.
- Singleton conflict detection now tie-breaks equal creation timestamps (1s granularity) by namespace/name, making the duplicate-singleton winner deterministic.

### Documentation

- Restructured into a Diátaxis-style tree: [install](docs/install/helm.md) (Helm, [configuration reference](docs/install/configuration.md), [binding levels](docs/install/binding-levels.md)), [operations](docs/operations/troubleshooting.md) (multi-operator, scope-map administration & RBAC delegation, dual-serving, large installations, troubleshooting), [architecture](docs/architecture/resource-hierarchy.md) (current-state only), and [development](docs/development/contributing.md).
- **Generated CRD API reference** ([docs/reference/api.md](docs/reference/api.md)) via `crd-ref-docs`, wired into `just generate` and gated by `verify-generate`.
- `docs/DESIGN.md` split: current-state content absorbed into `docs/architecture/`; decision history preserved as dated records in [docs/design/](docs/design/README.md). `docs/GUIDE-MULTI-INSTANCE.md` folded into `docs/operations/`.

## [0.16.0] — 2026-07-05

### Fixed

#### OIDCApp/APIApp: regenerate client secret when adopting an existing application

When an OIDCApp or APIApp CR adopts an existing Zitadel application by name (e.g. after an unclean cluster teardown left an orphaned app), the client secret cannot be read back from the Zitadel API. Previously the adoption path returned an empty secret, so the referenced Kubernetes Secret only ever got the `client_id` key — consumers (e.g. ArgoCD) then failed OIDC with `invalid_client "invalid secret"`.

Now, on adoption of a confidential OIDCApp (`type: confidential`, `authMethod != none`) or a basic-auth APIApp (`authMethod: basic`):

- If the referenced Secret already holds a non-empty client secret, it is preserved (no needless rotation)
- If the client secret key is missing or empty, a fresh secret is generated via the Zitadel `GenerateClientSecret` API and written to the Secret

Sibling controllers were audited for the same gap: ApplicationKey, MachineUser, and PersonalAccessToken already mint a fresh key/token when the referenced Secret lacks data, so no change was needed there.

#### APIApp: corrected return order in `createAPIApp`

`createAPIApp` returned `(clientID, clientSecret, appID)` while the caller expected `(appID, clientID, clientSecret)`, so a freshly created APIApp stored the client ID as application ID, the client secret as client ID, and the application ID as client secret (same class of bug fixed for OIDCApp in 0.14.0).

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
