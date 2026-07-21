# Zitadel Operator — E2E Test Scenarios

**Purpose:** Comprehensive test scenarios for REPL-style development. Each scenario validates a specific behavior end-to-end: CR applied → operator reconciles → Zitadel state verified → K8s state verified.

**Convention:** Each scenario has a unique ID (`S-XXX`) for reference in test code.

---

## 1. Organization

### S-001: Create Organization
- Apply Organization CR with `name: test-org-{uuid}`
- **Expect:** status.organizationId is set, status.ready = true
- **Verify in Zitadel:** organization exists with matching name

### S-002: Organization already exists in Zitadel (adopt)
- Pre-create org in Zitadel via API
- Apply Organization CR with matching name
- **Expect:** operator discovers existing org, sets status.organizationId, does NOT create duplicate

### S-003: Delete Organization
- Delete the Organization CR
- **Expect:** finalizer runs, org deleted in Zitadel, CR removed

### S-004: Delete Organization — resources still referencing it
- Create Org → Project → OIDCApp
- Delete Org CR
- **Expect:** Org deleted in Zitadel (cascades all children there), child CRs (Project, OIDCApp) go to error state on next reconcile with condition = "organization not found"

### S-005: Organization CR with operator default org — RETIRED in v0.18
- Relied on the removed `defaultOrganizationId` config key (INF-428); the
  scope-defaulted equivalent is covered by S-225 (scope supplies the org/project)

### S-006: Invalid organizationId (non-existent)
- Apply Project CR with `organizationId: "999999999"`
- **Expect:** status.ready = false, condition message = "organization not found"
- **Expect:** operator retries (requeue), does NOT crash

---

## 2. Project

### S-010: Create Project
- Apply Project CR with `organizationRef` pointing to an existing Org CR
- **Expect:** status.projectId set, status.ready = true
- **Verify in Zitadel:** project exists under the correct org

### S-011: Create Project with inline roles
- Apply Project CR with `roles: [admin, viewer, editor]`
- **Expect:** all three roles exist in Zitadel project

### S-012: Update Project roles (add + remove)
- Start with `roles: [admin, viewer]`
- Update to `roles: [admin, editor]`
- **Expect:** `viewer` removed, `editor` added, `admin` unchanged

### S-013: Create Project with raw organizationId
- Apply Project CR with `organizationId: "{real-id}"` (no Org CR)
- **Expect:** project created in that org, status.ready = true

### S-014: Delete Project
- Delete Project CR
- **Expect:** project deleted in Zitadel, finalizer removed

### S-015: Delete Project — apps still referencing it
- Create Project → OIDCApp referencing it
- Delete Project CR
- **Expect:** Project deletion proceeds (Zitadel cascades app deletion), OIDCApp CR goes to error state on next reconcile

### S-016: Project with organizationRef — Org CR not yet ready
- Apply Project CR referencing an Org CR that hasn't reconciled yet
- **Expect:** Project stays not-ready, condition = "waiting for organization"
- Apply Org CR, let it become ready
- **Expect:** Project reconciles and becomes ready

### S-017: Project already exists in Zitadel (adopt by name)
- Pre-create project in Zitadel
- Apply Project CR with same name under same org
- **Expect:** operator adopts existing project, sets status.projectId

---

## 3. OIDCApp

### S-020: Create confidential OIDCApp
- Apply OIDCApp CR with type=confidential, projectRef pointing to existing Project
- **Expect:** status.clientId set, status.ready = true
- **Expect:** K8s Secret created with client_id + client_secret keys
- **Verify in Zitadel:** OIDC app exists with correct redirect URIs

### S-021: Create public OIDCApp (no secret)
- Apply OIDCApp CR with type=public
- **Expect:** status.clientId set, status.ready = true
- **Expect:** NO K8s Secret created (public apps have no client_secret)

### S-022: Update OIDCApp redirect URIs
- Create app with `redirectUris: [https://a.com/cb]`
- Update to `redirectUris: [https://a.com/cb, https://b.com/cb]`
- **Expect:** Zitadel app updated with both URIs
- **Expect:** status.ready remains true (no re-creation)

### S-023: Update OIDCApp postLogoutRedirectUris
- Add postLogoutRedirectUris to existing app
- **Expect:** Zitadel app updated, no re-creation

### S-024: Update OIDCApp accessTokenType (bearer → jwt)
- Change accessTokenType from bearer to jwt
- **Expect:** Zitadel app updated

### S-025: Delete OIDCApp
- Delete OIDCApp CR
- **Expect:** OIDC app deleted in Zitadel, K8s Secret deleted (credentials are useless after app removal), finalizer removed

### S-026: OIDCApp with custom secret keys
- Apply OIDCApp with `secretRef.keys.clientId: "oidc.clientID"` and `secretRef.keys.clientSecret: "oidc.clientSecret"`
- **Expect:** Secret created with custom key names

### S-027: OIDCApp with extraData in secret
- Apply OIDCApp with `secretRef.extraData: {issuer: "https://auth.truvity.xyz"}`
- **Expect:** Secret contains the extra key-value pair alongside client credentials

### S-028: OIDCApp — project not ready yet
- Apply OIDCApp referencing a Project CR that hasn't reconciled
- **Expect:** OIDCApp stays not-ready, condition = "waiting for project"
- Once Project becomes ready → OIDCApp reconciles

### S-029: OIDCApp — cross-namespace projectRef
- Project CR in namespace `zitadel-system`
- OIDCApp CR in namespace `argocd` with `projectRef: {name: infra, namespace: zitadel-system}`
- **Expect:** app created successfully under that project

### S-030: OIDCApp idempotency (no drift = no API call)
- Apply OIDCApp, wait for ready
- Trigger re-reconcile (e.g., periodic requeue)
- **Expect:** NO Zitadel API update call (drift detection says "no change")
- **Verify:** operator logs show no "updating" message, only "reconciled"

### S-031: OIDCApp already exists in Zitadel (adopt by name)
- Pre-create OIDC app in Zitadel with matching name under same project
- Apply OIDCApp CR
- **Expect:** operator adopts it, writes clientId to status, ensures Secret exists

### S-032: OIDCApp — Zitadel app deleted externally
- Create OIDCApp via operator, wait for ready
- Delete the app directly in Zitadel (console/API)
- Wait for next reconcile
- **Expect:** operator detects missing app, re-creates it, new clientId in status

---

## 4. MachineUser

### S-040: Create MachineUser
- Apply MachineUser CR with organizationRef
- **Expect:** status.userId set, status.ready = true
- **Verify in Zitadel:** machine user exists

### S-041: MachineUser with key generation
- Apply MachineUser with `generateKey: true`
- **Expect:** key JSON written to Secret (secretRef)
- **Verify:** Secret contains valid JWT key JSON

### S-042: Delete MachineUser
- Delete MachineUser CR
- **Expect:** user deleted in Zitadel, finalizer removed

### S-043: MachineUser — update userName (immutable field)
- Attempt to change userName on existing MachineUser
- **Expect:** either rejected by webhook OR operator recreates user

---

## 5. IdentityProvider (Org-scoped)

### S-050: Create Google IdP
- Apply IdentityProvider CR with type=google, organizationRef, clientId/Secret from K8s Secret
- **Expect:** IdP created at org level in Zitadel, status.idpId set

### S-051: Create GitHub IdP
- Same pattern, type=github
- **Expect:** IdP created, status.ready = true

### S-052: Update IdP scopes
- Change scopes on existing IdP
- **Expect:** Zitadel IdP updated

### S-053: Delete IdP
- **Expect:** IdP removed from Zitadel, unlinked from login policy

### S-054: IdP — credentials Secret not found
- Apply IdP CR referencing a Secret that doesn't exist
- **Expect:** status.ready = false, condition = "secret not found"
- Create the Secret
- **Expect:** next reconcile succeeds

---

## 6. LoginPolicy (Org-scoped)

### S-060: Create LoginPolicy
- Apply LoginPolicy CR with organizationRef
- **Expect:** login policy configured at org level

### S-061: Update LoginPolicy (enable external IdP)
- Set allowExternalIdp: true
- **Expect:** Zitadel login policy updated

### S-062: LoginPolicy — add IdP to allowed list
- Reference an IdentityProvider CR in the LoginPolicy
- **Expect:** IdP added to the org's allowed identity providers

---

## 7. UserGrant

### S-070: Create UserGrant (user + project + roles)
- Apply UserGrant CR referencing a user (by ID or CR) and project
- **Expect:** user grant created in Zitadel with specified roles

### S-071: Update UserGrant roles
- Change roles list
- **Expect:** grant updated in Zitadel

### S-072: Delete UserGrant
- **Expect:** grant removed in Zitadel

---

## 8. Actions v2

### S-080: Create ActionTarget (webhook)
- Apply ActionTarget CR with endpoint URL and signing key secret
- **Expect:** target created in Zitadel, status.targetId set

### S-081: Create ActionExecution (event condition)
- Apply ActionExecution CR binding an event to the target
- **Expect:** execution created in Zitadel

### S-082: Update ActionTarget endpoint
- Change the endpoint URL
- **Expect:** target updated in Zitadel

### S-083: Delete ActionTarget — execution still references it
- **Expect:** target deletion blocked or execution goes to error state

---

## 9. Notification Providers (Instance-level)

### S-090: Create EmailProvider (SMTP)
- Apply EmailProvider CR with type=smtp, host, credentials
- **Expect:** SMTP provider configured in Zitadel instance

### S-091: Create EmailProvider (HTTP webhook)
- Apply EmailProvider CR with type=http, endpoint URL
- **Expect:** HTTP email provider configured, activated

### S-092: Create SmsProvider (Twilio)
- Apply SmsProvider CR with type=twilio, credentials from Secret
- **Expect:** Twilio SMS provider configured in Zitadel

### S-093: Create SmsProvider (HTTP webhook)
- Apply SmsProvider CR with type=http, endpoint URL
- **Expect:** HTTP SMS provider configured, activated

### S-094: Update EmailProvider credentials
- Rotate SMTP password (update Secret)
- **Expect:** operator detects Secret change, updates Zitadel provider

---

## 10. Error States & Recovery

### S-100: Zitadel unreachable at startup
- Operator starts, Zitadel instance is down
- **Expect:** operator fails health check, does NOT crash-loop indefinitely
- **Expect:** clear error in logs with connection details

### S-101: Zitadel unreachable during reconcile
- Operator running fine, Zitadel goes down
- **Expect:** reconcile fails, CR condition = "zitadel unreachable"
- **Expect:** operator retries with exponential backoff
- Zitadel comes back
- **Expect:** next retry succeeds, CR becomes ready

### S-102: Zitadel returns 403 (insufficient permissions)
- Operator SA lacks permission for the operation
- **Expect:** clear error condition on CR = "permission denied: {detail}"
- **Expect:** operator does NOT retry immediately (no hot-loop)

### S-103: Zitadel returns 409 (conflict / already exists)
- Race condition: resource created externally between list and create
- **Expect:** operator handles gracefully, adopts existing resource

### S-104: Invalid CR spec (validation)
- Apply OIDCApp with both `projectRef` AND `projectId` set
- **Expect:** validation error (webhook or controller-level), CR not reconciled

### S-105: Referenced CR deleted (dangling ref)
- OIDCApp references Project CR → delete Project CR
- **Expect:** OIDCApp goes to error state on next reconcile, condition = "project CR not found"

### S-106: Secret referenced by CR is deleted
- OIDCApp's secretRef Secret is deleted externally
- **Expect:** operator re-creates the Secret on next reconcile (if it has the data)

### S-107: CR applied to namespace not in watchNamespaces
- Operator watches `[zitadel-system]`
- Apply OIDCApp in namespace `default`
- **Expect:** operator ignores it completely (not in cache)

---

## 11. Idempotency & Drift

### S-110: No-op reconcile (no drift)
- CR is in sync with Zitadel
- Trigger reconcile
- **Expect:** no Zitadel API write calls, status unchanged

### S-111: External drift — field changed in Zitadel console (enforce mode)
- Change redirect URI directly in Zitadel console
- Wait for periodic requeue
- **Expect:** operator detects drift, reverts to CR spec (operator is source of truth)

### S-111b: External drift — observe mode
- Apply OIDCApp with `policy: observe`
- Change redirect URI directly in Zitadel console
- Wait for periodic requeue
- **Expect:** operator detects drift, sets condition = "drift detected: redirectUris differs"
- **Expect:** operator does NOT revert the change
- **Expect:** status.driftDetected = true

### S-112: Status-only update does not trigger reconcile
- Operator updates status.lastSyncTime
- **Expect:** this does NOT cause immediate re-reconcile (GenerationChangedPredicate)

### S-113: Rapid spec updates (debounce)
- Apply CR, immediately update spec 3 times in quick succession
- **Expect:** operator reconciles the final state, not intermediate states

---

## 12. Lifecycle & Finalizers

### S-120: Finalizer added on first reconcile
- Apply any CR
- **Expect:** finalizer `zitadel.truvity.io/finalizer` is present on the CR

### S-121: Deletion blocked until Zitadel cleanup succeeds
- Delete CR while Zitadel is unreachable
- **Expect:** CR stays in Terminating, finalizer not removed
- Zitadel comes back
- **Expect:** finalizer runs cleanup, removes finalizer, CR deleted

### S-122: Force delete (remove finalizer manually)
- Remove finalizer from CR manually
- **Expect:** CR deleted from K8s, resource remains orphaned in Zitadel
- **Note:** this is expected behavior (user chose to orphan)

---

## 13. Instance-Level Policies (P0 — IAM_OWNER, Admin API)

### S-130: DefaultLoginPolicy — create and update
- Apply DefaultLoginPolicy with allowExternalIdp: true
- **Expect:** status.ready = true, instance login policy updated
- Update to allowExternalIdp: false
- **Expect:** drift detected, Zitadel instance default updated

### S-131: DefaultLoginPolicy — deletion resets to safe baseline
- Delete DefaultLoginPolicy CR
- **Expect:** finalizer resets instance to safe defaults (userLogin: true, allowExternalIdp: false)
- **Expect:** CR deleted

### S-132: DefaultDomainPolicy — create and update
- Apply DefaultDomainPolicy with userLoginMustBeDomain: false
- **Expect:** status.ready = true, instance domain policy updated
- Update validateOrgDomains: true
- **Expect:** drift reconciled

### S-133: DefaultDomainPolicy — deletion resets to safe baseline
- Delete DefaultDomainPolicy CR
- **Expect:** resets to safe defaults (all true), CR deleted

### S-134: GoogleIdP — lifecycle with secret ref
- Create K8s Secret with clientSecret
- Apply GoogleIdP with clientSecretRef referencing the Secret
- **Expect:** status.idpID set, status.ready = true
- Update scopes
- **Expect:** provider updated in Zitadel

### S-135: GoogleIdP — secret not found
- Apply GoogleIdP referencing a Secret that doesn't exist
- **Expect:** Ready=False, condition = "SecretNotFound"
- Create the Secret
- **Expect:** next reconcile succeeds

### S-136: GoogleIdP → DefaultLoginPolicy idpRef resolution (end-to-end)
- Create GoogleIdP, wait for Ready (status.idpID set)
- Create DefaultLoginPolicy with idps[].idpRef pointing to the GoogleIdP CR
- **Expect:** policy reconciles successfully, IDP added to instance login policy
- Delete policy, then IdP
- **Expect:** IDP removed from login policy, clean deletion

---

## 14. Org-Scoped Policies (P1 — ORG_OWNER, Management API)

### S-140: LoginPolicy — create custom org policy
- Create Organization
- Apply LoginPolicy with organizationRef, allowRegister: true
- **Expect:** custom login policy created at org level, status.ready = true

### S-141: LoginPolicy — update (mutation)
- Change allowRegister to false
- **Expect:** custom policy updated in Zitadel

### S-142: LoginPolicy — deletion resets to default
- Delete LoginPolicy CR
- **Expect:** org login policy reset to instance default

### S-143: PasswordComplexityPolicy — lifecycle
- Create Organization
- Apply PasswordComplexityPolicy (minLength: 12, hasLowercase: true, hasUppercase: true)
- **Expect:** custom policy created at org level, status.ready = true
- Update minLength to 16
- **Expect:** policy updated

### S-144: PasswordComplexityPolicy — deletion resets to default
- Delete CR
- **Expect:** org policy reset to instance default

### S-145: LockoutPolicy — lifecycle
- Create Organization
- Apply LockoutPolicy (maxPasswordAttempts: 5, maxOtpAttempts: 3)
- **Expect:** custom policy created at org level, status.ready = true
- Update maxPasswordAttempts to 10
- **Expect:** policy updated

### S-146: LockoutPolicy — deletion resets to default
- Delete CR
- **Expect:** org policy reset to instance default

---

## 15. Email Provider (P2 — IAM_OWNER, Admin API)

### S-150: EmailProvider HTTP — lifecycle
- Apply EmailProvider with http.endpoint
- **Expect:** provider created and activated, status.providerId set, status.ready = true
- Update endpoint
- **Expect:** provider updated

### S-151: EmailProvider SMTP — lifecycle with passwordSecretRef
- Create K8s Secret with SMTP password
- Apply EmailProvider with smtp config + passwordSecretRef
- **Expect:** provider created, activated, status.providerId set
- Delete CR
- **Expect:** provider removed from Zitadel

### S-152: EmailProvider — invalid spec (both smtp and http)
- Apply EmailProvider with both smtp and http set
- **Expect:** Ready=False, condition = "InvalidSpec"

---

## Running Scenarios

```bash
# All scenarios (requires Kind + real Zitadel configured in ~/.config/zitadel-operator/config.yaml)
go test -tags=integration -v ./tests/integration/...

# Specific scenario group
go test -tags=integration -v -run TestOrganization ./tests/integration/...
go test -tags=integration -v -run TestOIDCApp ./tests/integration/...
go test -tags=integration -v -run TestDefaultLoginPolicy ./tests/integration/...
go test -tags=integration -v -run TestGoogleIdP ./tests/integration/...
go test -tags=integration -v -run TestLoginPolicy ./tests/integration/...
go test -tags=integration -v -run TestEmailProvider ./tests/integration/...

# Single scenario
go test -tags=integration -v -run TestOIDCApp/S-030_idempotency ./tests/integration/...
```


---

## 16. Singleton Policies — Drift Detection & Duplicates

### S-160: DefaultLockoutPolicy — drift detection
- Create DefaultLockoutPolicy with maxPasswordAttempts=5
- Externally change value to 99 via Zitadel Admin API
- Trigger requeue (label update on CR)
- **Expect:** operator detects drift and reconciles back to 5
- **Test:** `TestDefaultLockoutPolicy_DriftDetection`

### S-161: DefaultLockoutPolicy — duplicate creation (last-writer-wins)
- Create two DefaultLockoutPolicy CRs with different maxPasswordAttempts values
- **Expect:** both become Ready=true (both successfully call UpdateLockoutPolicy)
- **Expect:** actual Zitadel value matches whichever CR reconciled last
- **Test:** `TestDefaultLockoutPolicy_DuplicateCreation`

---

## 17. Message Text Resources

### S-170: DefaultMessageText init — lifecycle with mutation
- Create DefaultMessageText (type=init), verify Ready
- Mutate Subject field, verify still Ready after reconcile
- Delete (resets to Zitadel default)
- **Test:** `TestDefaultMessageText_Init_Lifecycle`

### S-171: DefaultMessageText passwordReset — lifecycle
- Create DefaultMessageText (type=passwordReset), verify Ready
- Delete (resets to default)
- **Test:** `TestDefaultMessageText_PasswordReset_Lifecycle`

### S-172: MessageText init — org-scoped lifecycle with mutation
- Create Organization, then MessageText (type=init) with orgRef
- Verify Ready, verify organizationId in status
- Mutate Greeting field, verify still Ready
- Delete (resets org message to default)
- **Test:** `TestMessageText_Init_Lifecycle`

### S-173: verifySmsOtp — SmsFieldWarning condition
- Create DefaultMessageText (type=verifySmsOtp) with email-only fields set
- **Expect:** Ready=true (non-blocking), condition SmsFieldWarning=True present
- **Verify:** only language and text are sent to Zitadel API

---

## 18. Batch 2 Resources — Mutations

### S-180: HumanUser — lifecycle with mutation
- Create Organization, HumanUser with orgRef
- Mutate FirstName, verify still Ready
- **Test:** `TestHumanUser_Lifecycle`

### S-181: OrgMember — lifecycle with mutation
- Create Organization, MachineUser, OrgMember
- Mutate Roles to ["ORG_OWNER", "ORG_ADMIN"], verify still Ready
- **Test:** `TestOrgMember_Lifecycle`

### S-182: LabelPolicy — lifecycle with mutation
- Create Organization, LabelPolicy with orgRef
- Mutate PrimaryColor, verify still Ready
- **Test:** `TestLabelPolicy_Lifecycle`

### S-183: NotificationPolicy — lifecycle with mutation
- Create Organization, NotificationPolicy with orgRef
- Toggle PasswordChange, verify still Ready
- **Test:** `TestNotificationPolicy_Lifecycle`

### S-184: PasswordAgePolicy — lifecycle with mutation
- Create Organization, PasswordAgePolicy with orgRef
- Change MaxAgeDays, verify still Ready
- **Test:** `TestPasswordAgePolicy_Lifecycle`

### S-185: SmsProvider HTTP — lifecycle with mutation
- Create SmsProvider (HTTP), verify Ready + providerId
- Change Endpoint, verify still Ready
- **Test:** `TestSmsProvider_HTTP_Lifecycle`

---

## 19. Privacy & OIDC Policies — Mutations

### S-190: PrivacyPolicy — lifecycle with mutation
- Create Organization, PrivacyPolicy with orgRef
- Mutate HelpLink, verify still Ready
- **Test:** `TestPrivacyPolicy_Lifecycle`

### S-191: DefaultPasswordAgePolicy — mutation
- Create DefaultPasswordAgePolicy, verify Ready
- Change MaxAgeDays, verify still Ready
- **Test:** `TestDefaultPasswordAgePolicy_Lifecycle`

### S-192: DefaultNotificationPolicy — mutation
- Create DefaultNotificationPolicy, verify Ready
- Toggle PasswordChange, verify still Ready
- **Test:** `TestDefaultNotificationPolicy_Lifecycle`

### S-193: DefaultLabelPolicy — mutation
- Create DefaultLabelPolicy, verify Ready
- Change PrimaryColor, verify still Ready
- **Test:** `TestDefaultLabelPolicy_Lifecycle`


---

## 20. Future — GitOps Composition Coverage (PLANNED, NOT IMPLEMENTED)

These scenarios validate composed flows that a real GitOps deployment applies. They are NOT yet implemented — recorded here for future development. See ROADMAP.md for prioritization.

### S-200: Brownfield Adoption (P1 — GitOps confidence)
- Pre-create Project + OIDCApp via Zitadel API (simulating Pulumi-managed state)
- Apply matching CRs with same display names
- **Expect:** operator adopts existing resources in-place; no duplicates created
- **Expect:** OIDCApp status.clientId matches the pre-existing client ID (no churn)
- **Expect:** K8s Secret populated with the existing credentials
- **Priority:** MUST pass before any Pulumi→operator migration cutover
- **Risk:** may surface missing adoption capability (match-by-ID or adopt annotation)
- **Status:** PLANNED

### S-201: Apply-All-at-Once / Out-of-Order Convergence (P1 — GitOps confidence)
- Apply full bundle in random order: Organization, Project, roles, OIDCApp, MachineUser, ProjectMember, LoginPolicy, GoogleIdP
- Do NOT wait between applies — single `kubectl apply -f bundle/`
- **Expect:** all resources eventually reach Ready=true (convergence)
- **Expect:** no crash, no wedge, no permanent error state
- **Expect:** parents-not-ready resources requeue and resolve once deps are ready
- **Status:** PLANNED

### S-202: Per-Cluster Onboarding Bundle (P1 — GitOps confidence)
- Apply: Project → roles → kubelogin OIDCApp (confidential) → MachineUser → ProjectMember(PROJECT_OWNER)
- **Expect:** OIDCApp Secret has `client_id` + `client_secret` with stable key names
- **Expect:** MachineUser Secret has `key.json` with stable key name
- **Expect:** key names are deterministic across reconcile cycles (no drift)
- **Status:** PLANNED

### S-203: Full Instance-Global Bootstrap Bundle (P2 — follow-up)
- Apply all Default* policies + GoogleIdP + EmailProvider + Organization + UserGrant together
- **Expect:** all reach Ready=true
- **Expect:** GoogleIdP → DefaultLoginPolicy idpRef resolution works end-to-end
- **Status:** PLANNED

### S-204: Composite Drift Correction (P2 — follow-up)
- Create OIDCApp via operator, wait for Ready
- Externally modify redirectUris via Zitadel console/API
- Wait for periodic requeue (5 min) or trigger via spec touch
- **Expect:** operator detects drift and reverts redirectUris to CR spec
- **Extends:** S-160 (singleton drift) to app-level resources
- **Status:** PLANNED

---

## 21. v0.18 — Scope Maps, Internal Delegation, Multi-Instance (INF-422/INF-435)

All scenarios below are IMPLEMENTED and run against the dedicated test instance; every created resource is prefixed `v018-`.

### S-210: SSA status discipline — foreign-manager condition survives
- OIDCApp waiting on an unresolved ref accumulates Ready=False conditions
- A condition is applied by a *different* SSA field manager on the status subresource
- Satisfy the ref; the operator writes Ready=True through its own field manager
- **Expect:** the foreign condition AND the operator's Ready condition coexist (regression for the prototype's condition-wipe finding; conditions are listType=map)
- **Test:** `TestSSA_ForeignManagerConditionSurvives`

### S-220: Binding verification — iam-owner assertion passes
- `VerifyBinding(iam-owner)` against the IAM_OWNER test credential succeeds, no bound org
- **Test:** `TestBinding_VerifyIAMOwner`

### S-221: Binding verification — mismatch is fatal
- `VerifyBinding(org-owner)` against the IAM_OWNER credential errors (in `main()` this crashes before any reconcile)
- **Test:** `TestBinding_MismatchCrashes`

### S-222: org-owner degradation matrix
- Instance-level resource (DefaultLockoutPolicy) reconciled under an org-owner binding
- **Expect:** `Ready=False / NotSupportedAtBindingLevel`, no Zitadel call
- **Test:** `TestBinding_DegradationMatrix_OrgOwner`

### S-223: org-owner + foreign-org map
- Scope map for an org foreign to the org-owner binding
- **Expect:** map fail-closed `NotSupportedAtBindingLevel` + `ForeignOrganization` Warning Event
- **Test:** `TestBinding_ForeignOrgMap_Event`

### S-224: Breaking-config fail-fast (INF-428)
- Config containing `defaultOrganizationId` or `projectScopeLabel` refuses to load, pointing at docs/MIGRATION-0.18.md; `binding` is required
- **Tests:** unit `TestLoad_RemovedKeysFailFast`, `TestLoad_MissingBinding`, `TestLoad_InvalidBinding`

### S-225: Scope map — selector rule match, scope-defaulted project
- Namespace routed by a labelSelector rule to a project scope; OIDCApp with no projectRef/projectId becomes Ready with `status.projectId` = scope project; `ScopeResolved=True`; delegation Secret consistent with status
- **Test:** `TestScopeMap_SelectorRuleMatch`

### S-226: Scope map — literal rule + delegated ACTOR PROOF
- Literal-rule scope; after reconcile, `AdminService.ListEvents` on the project aggregate shows ALL `project.application.*` events authored by the delegate and ZERO by the binding credential (`project.added` by the binding — minting is the binding's job)
- **Test:** `TestScopeMap_LiteralMatch_DelegatedActorProof`

### S-227: Scope map — no-match fail-closed
- Maps exist, none match the namespace: `ScopeResolved=False / NoMatchingRule`, never Ready, no Zitadel app created (`MapsNotSynced` is a distinct transient reason)
- **Test:** `TestScopeMap_NoMatch_FailClosed`

### S-228: Scope map — cross-map conflict
- Namespace matched by rules in two maps: fail-closed `ScopeConflict`, Warning Events on BOTH maps
- **Test:** `TestScopeMap_CrossMapConflict`

### S-229: Scope map — instance mismatch
- Map with a foreign `spec.instance`: map gets `InstanceMatch=False / InstanceMismatch` + Ready=false; tenant CRs in its namespaces fail closed with `InstanceMismatch`
- **Test:** `TestScopeMap_InstanceMismatch`

### S-230: SDK surface evidence
- `AuthService.ListMyMemberships` (binding checks) and v2 `UserService.AddKey/ListKeys/RemoveKey` (dual-key rotation) proven live
- **Tests:** `TestScopeMap_SDKSurface_OwnMemberships`, `TestScopeMap_SDKSurface_DualKeyRotation`

### S-231: Delegation — warm restart
- Fresh Manager (simulated restart) warms from the labeled Secret and reuses the existing delegate (no second SA, same key)
- **Test:** `TestDelegation_WarmRestart`

### S-232: Delegation — lazy re-mint
- Delegate SA deleted out-of-band; next Ensure detects the stale credential, drops the Secret and re-mints a new SA
- **Test:** `TestDelegation_LazyRemint`

### S-233: Delegation — internal dual-key rotation
- Key past the rotation cycle: new key minted and swapped into the Secret; two keys live during the grace overlap; sweep revokes the old key after grace
- **Test:** `TestDelegation_Rotation_DualKey`

### S-234: Delegation — eager revoke + GC on unmatch
- Tenant CR deleted, then its map deleted: the eager sweep revokes the delegate SA and deletes its Secret
- **Test:** `TestDelegation_EagerRevoke_OnMapRemoval`

### S-240: Dual-serving — foreign-instance pin ignored
- OIDCApp pinned to another instance: completely untouched (no finalizer, no status, no Zitadel app); repinning to this operator hands the CR over through the delegated scope
- **Test:** `TestDualServe_InstancePin_ForeignInstanceIgnored`

### S-241: Dual-serving — AmbiguousInstance two-manager SSA smoke
- Two reconcilers with distinct `zitadel-operator/<domain>` field managers serve one namespace; unpinned CR: the second operator detects the first via managedFields and marks `InstanceResolved=False / AmbiguousInstance` WITHOUT wiping the first's conditions; the first fails closed too (no external updates); pinning `spec.instance` resolves the ambiguity
- **Test:** `TestDualServe_AmbiguousInstance_TwoManagers`

### S-250: MachineUser — scope roles + connection bundle (INF-426)
- Project-scoped namespace: `spec.roles` become a user grant on the scope project (via the delegated client); the key Secret is a connection bundle (`key.json`, `instanceUrl`, `issuer`, `orgId`, `projectId`, best-effort `instanceId`); narrowing roles updates the grant
- **Test:** `TestMachineUser_ScopeRoles_ConnectionBundle`

### S-251: MachineUser — spec.key.rotateAfter dual-key rotation
- Controller-driven rotation with grace overlap: new key in Secret + status bookkeeping, 2 keys live during grace, old key revoked afterwards; no `key.rotateAfter` = pre-v0.18 behavior (covered by all pre-existing MachineUser tests)
- **Test:** `TestMachineUser_KeyRotation_DualKey`

### S-260: INF-430 — adopted confidential app regenerates client secret
- App pre-created in Zitadel; adopting CR with an empty Secret: `client_secret` regenerated exactly once (no churn on later reconciles)
- **Test:** `TestINF430_AdoptedConfidentialApp_SecretRegenerated`

### S-261: INF-400 — URI-list update staleness repro
- Successive redirect-URI mutations (append, replace, shrink) must each converge server-side with no stale entries
- **Test:** `TestINF400_URIListUpdate_NoStaleState`

### S-270: Leader election — two-process handoff (INF-427)
- Two managers, one lease: exactly one leads; graceful shutdown hands the lease to the standby
- **Test:** `TestLeaderElection_TwoProcessHandoff`
