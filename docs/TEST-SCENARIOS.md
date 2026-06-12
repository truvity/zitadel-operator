b# Zitadel Operator — E2E Test Scenarios

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

### S-005: Organization CR with operator default org
- Operator config has `defaultOrganizationId` set
- Apply a Project CR without `organizationRef` or `organizationId`
- **Expect:** Project created in the default org

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

## Running Scenarios

```bash
# All scenarios (requires Kind + real Zitadel configured in ~/.config/zitadel-operator/config.yaml)
go test -tags=integration -v ./tests/integration/...

# Specific scenario group
go test -tags=integration -v -run TestOrganization ./tests/integration/...
go test -tags=integration -v -run TestOIDCApp ./tests/integration/...

# Single scenario
go test -tags=integration -v -run TestOIDCApp/S-030_idempotency ./tests/integration/...
```
