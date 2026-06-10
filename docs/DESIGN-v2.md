# Zitadel Operator v2 — Architecture Design

**Ticket:** INF-363  
**Status:** Draft  
**Breaking:** Yes (v1alpha1 → v1alpha2)

## Table of Contents

1. [Design Principles](#design-principles)
2. [Operator Configuration](#operator-configuration)
3. [Multi-Operator Isolation (Namespace + RBAC)](#multi-operator-isolation-namespace--rbac)
4. [Resource Hierarchy](#resource-hierarchy)
5. [CRD Catalog (v1alpha2)](#crd-catalog-v1alpha2)
6. [Testing](#testing)
7. [Hot-Loop Fix (INF-362)](#hot-loop-fix-inf-362)
8. [Migration Path](#migration-path)
9. [Implementation Phases](#implementation-phases)

---

## Design Principles

1. **Explicit over implicit** — every resource declares which organization it belongs to (like Terraform's `org_id`)
2. **Hierarchical references** — Organization → Project → App/User, with parent references propagating defaults downward
3. **Operator-per-instance** — each operator deployment is bound to exactly one Zitadel instance (configured at startup via config file + mounted secret)
4. **Namespace-based isolation** — multiple operators coexist via namespace scoping + K8s RBAC; no custom labels or controller-class
5. **No CRD for connection config** — instance connection belongs to the deployment, not a reconciled resource
6. **Idempotent reconciliation** — no hot-loops, conditional status updates, proper drift detection
7. **Generic and reusable** — no vendor-specific or Truvity-specific code; the operator works with any Zitadel instance (Cloud or self-hosted) and any deployment method (Helm, ArgoCD, Pulumi, raw manifests)
8. **Terraform provider parity** — resource coverage and field-level feature parity with the official Zitadel Terraform provider; the operator can fully replace Terraform for in-cluster identity management when desired, while coexisting with Terraform/Pulumi for out-of-cluster resources

---

## Operator Configuration

The operator is bound to exactly one Zitadel instance at startup. All configuration is provided via a single YAML config file — no CLI flags beyond the config path itself.

### Config File

```yaml
# /etc/zitadel-operator/config.yaml (mounted from ConfigMap)
domain: zitadel.truvity.xyz
port: "443"
insecure: false
externalDomain: zitadel.truvity.xyz       # Optional: split-horizon routing
keyFile: /etc/zitadel/key.json            # Path to mounted JWT key file
defaultOrganizationId: "325908..."        # Optional: default org for resources that omit it
watchNamespaces:                          # Optional: omit to watch all namespaces
  - zitadel-system
  - argocd
  - hubble-ui
```

The only CLI argument is the config file path (with a sensible default):

```
zitadel-operator --config=/etc/zitadel-operator/config.yaml
```

### Why Config File Only (no CLI flags)

- **One source of truth** — all config in one place, no precedence rules between flags/env/file
- **Matches test pattern** — `~/.zitadel/config.yaml` for local dev/tests, same format in production
- **Simpler Helm chart** — render a ConfigMap, mount it; no long `args:` list in the Deployment
- **Handles growth** — if config gains complex fields (watched namespaces, retry policies), YAML handles it naturally
- **Internal tool** — no public users who need `--help` discoverability

### Credential Management

The JWT key JSON is mounted into the pod from a Kubernetes Secret via a standard volume mount. The config file references it by path (`keyFile`). This gives:
- Automatic rotation via kubelet secret sync (no restart needed)
- No Secret-reading RBAC required at startup
- Standard pattern (same as cert-manager's CA key, ArgoCD's repo credentials)

### Helm Deployment

```yaml
# Helm values
config:
  domain: zitadel.truvity.xyz
  port: "443"
  externalDomain: zitadel.truvity.xyz
  defaultOrganizationId: "325908855630427886"
  watchNamespaces:
    - zitadel-system
    - argocd

credentials:
  secretName: zitadel-admin-sa
  key: zitadel-admin-sa.json
  mountPath: /etc/zitadel
```

Helm renders:
1. A ConfigMap with the operator config YAML (mounted at `/etc/zitadel-operator/config.yaml`)
2. A volume mount from the credentials Secret (mounted at `/etc/zitadel/key.json`)

### Why No ZitadelInstance CRD

| Concern | Solved by |
|---------|-----------|
| Which instance to connect to | Config file (`domain` field) |
| Credential rotation | Volume mount + kubelet secret sync |
| Connection status visibility | Operator health check endpoint + metrics |
| Multiple instances | Multiple operator deployments (one per instance) |
| Runtime reconfiguration | Not needed — restart is fine for instance-level changes |

A CRD adds reconciliation complexity for connection lifecycle (reconnect, retry, health) without clear benefit when the operator is always 1:1 with an instance.

---

## Multi-Operator Isolation (Namespace + RBAC)

**Problem:** Multiple operators in the same cluster, managing different organizations or different Zitadel instances.

**Solution:** Pure Kubernetes primitives — namespace scoping + RBAC. No custom labels or application-level filtering.

### How It Works

Each operator deployment:
1. Watches only specific namespaces (configured via `watchNamespaces` in config)
2. Has a ServiceAccount with RBAC (Roles + RoleBindings) only in those namespaces
3. The K8s API server enforces the boundary — the operator literally cannot see CRs in other namespaces

### Example — Two Operators, Same Cluster

```
Operator A (domain: zitadel.truvity.cloud, watchNamespaces: [zitadel-system, argocd, hubble-ui])
  SA: zitadel-operator-platform
  RoleBindings in: zitadel-system, argocd, hubble-ui
  → manages platform infrastructure apps

Operator B (domain: zitadel.truvity.cloud, watchNamespaces: [product-system, billing])
  SA: zitadel-operator-product
  RoleBindings in: product-system, billing
  → manages product apps
```

### Example — Two Operators, Different Instances

```
Operator A (domain: zitadel.truvity.cloud, watchNamespaces: [prod-apps])
  → talks to production Zitadel

Operator B (domain: zitadel-staging.truvity.cloud, watchNamespaces: [staging-apps])
  → talks to staging Zitadel
```

### Why Not Controller-Class

| Approach | Enforcement | Complexity |
|----------|------------|------------|
| Controller-class label | Application-level (our code filters) | Custom logic, not RBAC-enforced |
| Namespace + RBAC | API server-level (K8s enforces) | Zero custom logic, standard K8s |

Since we don't need two operators in the same namespace, namespace-scoped RBAC gives us everything controller-class would — enforced by the platform, not by our code.

### Helm Deployment

```yaml
# values.yaml for Operator A (platform)
config:
  domain: zitadel.truvity.cloud
  defaultOrganizationId: "325908..."
  watchNamespaces:
    - zitadel-system
    - argocd
    - hubble-ui

rbac:
  namespaces:
    - zitadel-system
    - argocd
    - hubble-ui
```

Helm renders:
- A Role in each namespace (access to CRDs + Secrets)
- A RoleBinding in each namespace (binds SA to Role)
- The operator deployment with `watchNamespaces` in its config

---

## Resource Hierarchy

Follows Zitadel's actual data model — everything lives under an Organization:

```
Zitadel Instance (operator startup config, not a CRD)
└── Organization (namespaced)
    ├── Project (namespaced)
    │   ├── OIDCApp (namespaced)
    │   ├── APIApp (namespaced)
    │   ├── ProjectRole (inline in Project spec)
    │   ├── ProjectGrant (namespaced)
    │   └── ProjectMember (namespaced)
    ├── IdentityProvider (namespaced)
    ├── LoginPolicy (namespaced)
    ├── PasswordComplexityPolicy (namespaced)
    ├── LockoutPolicy (namespaced)
    ├── LabelPolicy (namespaced)
    ├── DomainPolicy (namespaced)
    ├── HumanUser (namespaced)
    ├── MachineUser (namespaced)
    │   ├── PersonalAccessToken (namespaced)
    │   └── MachineKey / ApplicationKey (namespaced)
    ├── UserGrant (namespaced)
    ├── OrgMember (namespaced)
    └── Action (namespaced)
```

**All CRDs are namespaced.** Namespaces serve as RBAC permission boundaries — platform team manages Organization and Project CRs in `zitadel-system`, app teams create OIDCApp/MachineUser CRs in their own namespaces.

### Reference Pattern (Either ID or CR Reference)

Every resource references only its **direct parent** in the hierarchy. References support two modes — raw Zitadel ID or CR name reference. They are mutually exclusive.

**Rule: reference your direct parent only.** Organization is inherited through the chain — an App doesn't need to specify org because it gets it from its Project.

| Resource level | References | Org resolution |
|---------------|-----------|----------------|
| Organization | nothing (top-level) | Is the org |
| Project | Organization (direct parent) | Explicit |
| OIDCApp, APIApp | Project (direct parent) | Inherited from Project |
| ProjectGrant, ProjectMember | Project (direct parent) | Inherited from Project |
| MachineUser, HumanUser | Organization (direct parent) | Explicit |
| IdentityProvider, LoginPolicy | Organization (direct parent) | Explicit |
| UserGrant | Organization (direct parent) + user + project refs | Explicit |

**Organization reference (on Project, User, IdP, Policy resources):**

```yaml
spec:
  # Option A: reference an Organization CR managed by this operator
  organizationRef:
    name: my-org
    namespace: zitadel-system  # optional — defaults to same namespace

  # Option B: reference a pre-existing Zitadel org by raw ID
  organizationId: "325908..."

  # Option C: omit both → use operator config defaultOrganizationId
```

**Project reference (on App, ProjectGrant, ProjectMember resources):**

```yaml
spec:
  # Option A: reference a Project CR (possibly in another namespace)
  projectRef:
    name: my-project
    namespace: zitadel-system  # optional — defaults to same namespace

  # Option B: reference a pre-existing Zitadel project by raw ID
  projectId: "326225..."
```

**Apps do NOT have organizationRef/organizationId** — the org is determined by their Project.

### Resolution Logic

| Field set | Resolution |
|-----------|-----------|
| `organizationRef` | Look up Organization CR (same ns or specified ns) → use `status.organizationId` |
| `organizationId` | Use directly |
| Neither | Use operator config `defaultOrganizationId` |
| Both | Validation error (mutually exclusive) |

| Field set | Resolution |
|-----------|-----------|
| `projectRef` | Look up Project CR (same ns or specified ns) → use `status.projectId` |
| `projectId` | Use directly |
| Neither | Error (project is required for apps/grants) |
| Both | Validation error (mutually exclusive) |

**Org resolution for App-level resources:** The operator resolves the Project first, then uses the Project's organization. No org field on the App itself.

### When to Use Which

| Scenario | Use |
|----------|-----|
| Parent resource is managed by this operator | `*Ref` (name reference) — stays in sync automatically |
| Parent resource is pre-existing (not operator-managed) | `*Id` (raw ID) — no CR needed |
| Single-org setup (all resources in one org) | Omit org entirely — operator default handles it |

### Full Example

```yaml
# Platform team manages org and project in zitadel-system namespace
apiVersion: zitadel.truvity.io/v1alpha2
kind: Organization
metadata:
  name: platform
  namespace: zitadel-system
spec: {}
---
apiVersion: zitadel.truvity.io/v1alpha2
kind: Project
metadata:
  name: infra
  namespace: zitadel-system
spec:
  organizationRef:
    name: platform
    # namespace omitted — same namespace (zitadel-system)
  roles: [admin, viewer]
---
# App team creates OIDCApp in their own namespace — only references Project (org inherited)
apiVersion: zitadel.truvity.io/v1alpha2
kind: OIDCApp
metadata:
  name: argocd
  namespace: argocd
spec:
  projectRef:
    name: infra
    namespace: zitadel-system
  type: confidential
  authMethod: basic
  redirectUris:
    - https://argocd.truvity.xyz/auth/callback
  secretRef:
    name: argocd-oidc
---
# Pre-existing project not managed by operator — use raw project ID (org inherited from project)
apiVersion: zitadel.truvity.io/v1alpha2
kind: OIDCApp
metadata:
  name: legacy-app
  namespace: default
spec:
  projectId: "326225093250303536"
  type: confidential
  authMethod: basic
  redirectUris:
    - https://legacy.example.com/callback
  secretRef:
    name: legacy-app-oidc
---
# Org-level resource (MachineUser) — references Organization directly
apiVersion: zitadel.truvity.io/v1alpha2
kind: MachineUser
metadata:
  name: ci-bot
  namespace: zitadel-system
spec:
  organizationRef:
    name: platform
  userName: ci-bot@platform.zitadel.truvity.xyz
  secretRef:
    name: ci-bot-credentials
```

---

## CRD Catalog (v1alpha2)

### Tier 1 — Core (implement first)

| CRD | Scope | Terraform Equivalent | Notes |
|-----|-------|---------------------|-------|
| Organization | Namespaced | `zitadel_org` | Platform team manages in dedicated ns |
| Project | Namespaced | `zitadel_project` | Has inline roles, references org |
| OIDCApp | Namespaced | `zitadel_application_oidc` | Secret output, references project |
| MachineUser | Namespaced | `zitadel_machine_user` | Key management |

### Tier 2 — Identity & Access

| CRD | Scope | Terraform Equivalent | Notes |
|-----|-------|---------------------|-------|
| HumanUser | Namespaced | `zitadel_human_user` | New |
| UserGrant | Namespaced | `zitadel_user_grant` | Role assignment |
| ProjectGrant | Namespaced | `zitadel_project_grant` | Cross-org grant |
| ProjectMember | Namespaced | `zitadel_project_member` | |
| OrgMember | Namespaced | `zitadel_org_member` | |
| InstanceMember | Namespaced | `zitadel_instance_member` | |

### Tier 3 — Policies & IdP

| CRD | Scope | Terraform Equivalent | Notes |
|-----|-------|---------------------|-------|
| IdentityProvider | Namespaced | `zitadel_idp_*` (google, github, saml, oidc) | Org-scoped (INF-357) |
| LoginPolicy | Namespaced | `zitadel_login_policy` | Org-scoped (INF-357) |
| PasswordComplexityPolicy | Namespaced | `zitadel_password_complexity_policy` | |
| LockoutPolicy | Namespaced | `zitadel_lockout_policy` | |

### Tier 4 — Extended

| CRD | Scope | Terraform Equivalent | Notes |
|-----|-------|---------------------|-------|
| APIApp | Namespaced | `zitadel_application_api` | New |
| ApplicationKey | Namespaced | `zitadel_application_key` | |
| PersonalAccessToken | Namespaced | `zitadel_personal_access_token` | |
| ActionTarget | Namespaced | Actions v2 Target (API-only, no TF resource yet) | Org-scoped, webhook URL + auth config |
| ActionExecution | Namespaced | Actions v2 Execution (API-only, no TF resource yet) | Org-scoped, condition → targets binding |
| EmailProvider | Namespaced | `zitadel_smtp_config` + HTTP email provider | Instance-level, type: smtp or http (webhook) |
| SmsProvider | Namespaced | `zitadel_sms_provider_twilio` + HTTP SMS provider | Instance-level, type: twilio or http (webhook) |
| LabelPolicy | Namespaced | `zitadel_label_policy` | Future |
| DomainPolicy | Namespaced | `zitadel_domain_policy` | Future |

### Key v1alpha1 → v1alpha2 Changes

| Change | Reason |
|--------|--------|
| All resources get explicit `organizationId` or `organizationRef` | Terraform parity, explicit scoping |
| All CRDs are namespaced | RBAC permission boundaries via K8s namespaces |
| Multi-operator via namespace scoping + RBAC | Standard K8s, no custom labels |
| Cross-namespace refs via `{name, namespace}` | Shared projects/orgs across team namespaces |
| No `instanceRef` field on resources | Instance is operator-level config, not per-resource |

---

## Testing

### Two Test Packages

| Package | What | Framework | Build tag | External deps | Run command |
|---------|------|-----------|-----------|--------------|-------------|
| `tests/unit/` | Business logic, drift detection, normalization, client helpers | Standard `go test` | None | None | `go test ./tests/unit/...` |
| `tests/integration/` | Full reconciliation loop (K8s + real Zitadel) | `kubernetes-sigs/e2e-framework` + Kind | `//go:build integration` | Docker, `~/.zitadel/config.yaml` | `go test -tags=integration ./tests/integration/...` |

### Unit Tests (`tests/unit/`)

Standard Go tests. No external dependencies. Test:
- Drift detection logic (compare spec vs observed state)
- Field normalization (e.g., URI trailing slashes, token type mapping)
- Status update logic (conditional update decisions)
- Secret key resolution
- Namespace filtering logic

```go
func TestDriftDetection_RedirectUris(t *testing.T) {
    spec := OIDCAppSpec{RedirectUris: []string{"https://app/callback"}}
    observed := &applicationv2.OIDCConfiguration{...}
    
    assert.True(t, hasDrift(spec, observed))
}
```

### Integration Tests (`tests/integration/`)

Full end-to-end using `kubernetes-sigs/e2e-framework`. Kind cluster spins up in `TestMain`, operator runs inside it (or as a process with Kind's kubeconfig), Zitadel calls go to the real test instance.

#### TestMain Lifecycle

```go
//go:build integration

package integration

func TestMain(m *testing.M) {
    testEnv = env.New()
    kindCluster := kind.NewCluster("zitadel-operator-e2e")

    testEnv.Setup(
        envfuncs.CreateCluster(kindCluster, "zitadel-operator-e2e"),
        envfuncs.CreateNamespace("zitadel-system"),
        installCRDs,          // kubectl apply -f config/crd/bases/
        deployOperator,       // build + load image + deploy, or run as process
    )

    testEnv.Finish(
        cleanupZitadelResources,  // delete test orgs/projects in Zitadel
        envfuncs.DestroyCluster("zitadel-operator-e2e"),
    )

    os.Exit(testEnv.Run(m))
}
```

#### Test Pattern

```go
//go:build integration

func TestOIDCAppLifecycle(t *testing.T) {
    feature := features.New("OIDCApp creates app in Zitadel").
        Setup(createTestProject).
        Assess("OIDCApp becomes ready", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
            client := cfg.Client()
            
            app := &v1alpha2.OIDCApp{...}
            require.NoError(t, client.Resources().Create(ctx, app))
            
            // Wait for status.ready
            err := wait.For(
                conditions.New(client.Resources()).ResourceMatch(app, func(obj k8s.Object) bool {
                    return obj.(*v1alpha2.OIDCApp).Status.Ready
                }),
                wait.WithTimeout(60*time.Second),
            )
            require.NoError(t, err)
            
            // Verify Secret was created
            secret := &corev1.Secret{}
            require.NoError(t, client.Resources().Get(ctx, app.Spec.SecretRef.Name, app.Namespace, secret))
            assert.NotEmpty(t, secret.Data["client_id"])
            
            return ctx
        }).
        Teardown(deleteTestResources).
        Feature()

    testEnv.Test(t, feature)
}
```

### Test Configuration

Tests read connection config from `~/.zitadel/config.yaml` — same shape as operator config. No instance-specific data in the repo.

```yaml
# ~/.zitadel/config.yaml
domain: <your-test-instance>.eu1.zitadel.cloud
port: "443"
keyFile: ~/.zitadel/key.json
# Tests create their own orgs per run and clean up
```

The repo contains only a README explaining setup:

```
tests/integration/README.md  — how to configure ~/.zitadel/config.yaml
tests/integration/testutil/  — config loader, cleanup helpers, name generators
```

### Test Isolation

- Each test run creates a unique test organization (UUID-suffixed name)
- All resources are created within that org
- `TestMain` teardown deletes the test org (cascades to all children)
- Tests can run in parallel (separate orgs)

---

## Hot-Loop Fix (INF-362)

Three changes to fix the 2-second reconciliation loop:

### 1. GenerationChangedPredicate

```go
import (
    "sigs.k8s.io/controller-runtime/pkg/builder"
    "sigs.k8s.io/controller-runtime/pkg/predicate"
)

func (r *OIDCAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha2.OIDCApp{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
        Named("oidcapp").
        Complete(r)
}
```

This ensures the controller only re-reconciles when `.metadata.generation` changes (spec changes), not on status-only updates.

### 2. Conditional Status Update

```go
// Only update status when something changed
statusChanged := cr.Status.ClientId != clientID || !cr.Status.Ready
if statusChanged {
    now := metav1.NewTime(time.Now())
    cr.Status.ClientId = clientID
    cr.Status.Ready = true
    cr.Status.LastSyncTime = &now
    if err := r.Status().Update(ctx, &cr); err != nil {
        return ctrl.Result{}, err
    }
}
```

### 3. Full Drift Detection in `updateOIDCAppIfNeeded`

Currently only compares `RedirectUris`. Must compare all mutable fields:
- RedirectUris
- PostLogoutRedirectUris
- AccessTokenType
- AccessTokenRoleAssertion
- IdTokenRoleAssertion
- AuthMethodType
- ApplicationType

---

## Migration Path

### Strategy: Manual Migration (no conversion webhook)

v1alpha2 is a breaking redesign. A conversion webhook adds complexity for marginal benefit given the small number of deployments.

### Steps

1. **Document mapping** from v1alpha1 → v1alpha2 resource shapes
2. **Migration script** that reads v1alpha1 CRs and generates v1alpha2 YAML
3. **Deploy v1alpha2 CRDs** alongside v1alpha1 (different API group version)
4. **Switch operator** to v1alpha2, apply migrated resources, remove v1alpha1 CRDs

### Migration checklist per resource:
- Add explicit `organizationId` or `organizationRef` where previously implicit
- Update refs to include `namespace` for cross-namespace references
- All resources become namespaced (Organization, Project were cluster-scoped in v1alpha1)
- No controller-class label needed — namespace isolation replaces it

---

## Implementation Phases

Priority: get into integration-test-driven development (Kind + real Zitadel) on day 1. Every subsequent change gets validated immediately.

### Step 0: Test Harness (enables REPL development)
- [ ] `~/.zitadel/config.yaml` loader (domain + keyFile)
- [ ] `TestMain` with e2e-framework — Kind cluster up/down
- [ ] CRD install into Kind (existing v1alpha1 CRDs)
- [ ] One trivial test: apply Project CR → operator creates project in real Zitadel → status.ready = true
- [ ] **Result:** REPL mode unlocked — every change validated instantly

### Step 1: Fix Existing Code Under Tests (v1alpha1, shippable patch)
- [ ] Hot-loop fix: GenerationChangedPredicate + conditional status update (S-030, S-112)
- [ ] Full drift detection for OIDCApp — all mutable fields (S-022, S-023, S-024, S-110, S-111)
- [ ] Integration tests: Project lifecycle (S-010, S-011, S-012, S-014, S-017)
- [ ] Integration tests: OIDCApp lifecycle (S-020, S-021, S-025, S-031, S-032)
- [ ] **Result:** bugs fixed, existing code tested, ship as v0.5.0

### Step 2: v1alpha2 Core Resources (one at a time, each with tests)
- [ ] Organization with organizationRef/organizationId (S-001–S-006)
- [ ] Project with organizationRef/organizationId, cross-ns ref (S-010–S-017)
- [ ] OIDCApp with projectRef/projectId + Secret cleanup + observe mode (S-020–S-032, S-111b)
- [ ] MachineUser (S-040–S-043)
- [ ] **Result:** complete CRUD with v1alpha2 API for core resources

### Step 3: Operator Infrastructure
- [ ] Config file loader (`--config` flag, YAML config)
- [ ] `watchNamespaces` implementation (namespace-scoped cache)
- [ ] Cross-namespace ref resolution (S-029, S-107)
- [ ] **Result:** multi-operator support, namespace isolation

### Step 4: Extended Resources (each with tests)
- [ ] IdentityProvider org-scoped (S-050–S-054)
- [ ] LoginPolicy org-scoped (S-060–S-062)
- [ ] UserGrant (S-070–S-072)
- [ ] ActionTarget + ActionExecution (S-080–S-083)
- [ ] EmailProvider + SmsProvider (S-090–S-094)
- [ ] **Result:** Terraform provider parity

### Step 5: Error Handling & Edge Cases
- [ ] Zitadel unreachable / permission denied / conflicts (S-100–S-107)
- [ ] Finalizer edge cases (S-120–S-122)
- [ ] Debounce rapid updates (S-113)
- [ ] Migration script v1alpha1 → v1alpha2
- [ ] Documentation
- [ ] **Result:** production-ready, release v1.0.0-alpha1

---

## Ecosystem Repositories

Three public repos forming the Zitadel identity automation ecosystem:

| Repository | Purpose | Knows about |
|------------|---------|-------------|
| `truvity/zitadel-operator` | K8s operator (CRDs, reconcilers) + `pkg/webhook/` helpers + examples | Zitadel only |
| `truvity/zitadel-rbac-mapper` | Groups→Zitadel grants mapping webhook (Deployment/Lambda) | Zitadel only (groups come as input) |
| `truvity/zitadel-notify-relay` | HTTP provider relay for Email/SMS (receives Zitadel notification payloads, delivers via AWS SES/SNS/etc.) | Zitadel notification payloads + delivery providers |
| `truvity/google-group-sync` | Google Workspace group fetcher (CronJob/Lambda) | Google only (no Zitadel knowledge) |

### What lives where

**zitadel-operator** provides:
- CRDs to declare ActionTarget + ActionExecution (configures the webhook wiring in Zitadel)
- `pkg/webhook/` Go library (typed payloads, signature verification, HTTP middleware)
- `pkg/notification/` Go library (Email/SMS HTTP provider payload types)
- `examples/` showing how to wire rbac-mapper and google-group-sync as Actions v2 targets

**zitadel-rbac-mapper** provides:
- Standalone binary (HTTP server) that receives group claims and maps them to Zitadel project grants
- Two modes:
  - **Token enrichment** (function manipulation): receives preaccesstoken/preuserinfo calls, resolves groups, returns `append_claims` with groups or per-project role claims
  - **Event-driven sync** (event handler): receives user.human.added/session.added events, resolves groups, syncs UserGrants in Zitadel (add/update/remove)
- Configurable via YAML rules (group → project/role mapping)
- Calls an external groups resolver (e.g., google-group-sync) via HTTP for group membership
- Uses `zitadel-operator/pkg/webhook/` for payload types (ActionsV2Request, SetClaimsResponse, AppendClaim)
- Uses Zitadel Management API v1 for grant CRUD (ListUserGrants, AddUserGrant, UpdateUserGrant, RemoveUserGrant)

**zitadel-notify-relay** provides:

**zitadel-notify-relay** provides:
- Standalone binary (HTTP server) that receives Zitadel HTTP provider notification payloads
- Delivers emails via AWS SES (initial), extensible to other providers (SendGrid, Mailgun, SMTP relay)
- Delivers SMS via AWS SNS (initial), extensible to other providers (Twilio direct, etc.)
- Provider-agnostic interface — add new delivery backends without changing Zitadel config
- Uses `zitadel-operator/pkg/notification/` for typed payload structs

**google-group-sync** provides:
- Standalone binary that fetches groups/membership from Google Directory API
- Uses Google Admin SDK with domain-wide delegation (service account + impersonation)
- Exposes groups via HTTP API: `POST /resolve` with `{"email": "user@domain.com"}` → `{"groups": [...]}`
- Lazy credential loading (secrets from env or AWS Secrets Manager)
- No Zitadel dependency — pure Google Workspace utility
- Can be invoked directly via HTTP or via Lambda invoke (from rbac-mapper)

### Composition flow

```
[Google Workspace] ──google-group-sync──→ [groups API]
                                               │
[User authenticates → Zitadel] ────────────────┘
         │ Actions v2 function manipulation
         ▼
   [rbac-mapper webhook] ──Zitadel API──→ [grants created]
```

### Shared repo conventions

All repos follow the same development tooling pattern (based on the existing zitadel-operator setup):

**Development environment:**
- `devbox.json` — Go, golangci-lint, gopls, goreleaser, govulncheck, just, ko, helm
- `.envrc` — direnv integration (`eval "$(devbox generate direnv --print-envrc)"`)
- No language version pinning in devbox — Go version from devbox `@latest`

**Build system:**
- `Justfile` — development commands (generate, build, test, lint, vuln, snapshot, helm-package)
- No moon (these are standalone repos, not part of the monorepo)

**Linting:**
- `.golangci.yml` — errcheck, govet, staticcheck, unused, gocritic, misspell, gosec, gofmt, goimports
- Local prefix: `github.com/truvity/<repo-name>`

**GitHub Actions:**
- `.github/workflows/ci.yaml` — on PR: lint + test + build
- `.github/workflows/release.yaml` — on tag push (v*): goreleaser release
- `.github/workflows/security.yaml` — govulncheck
- `.github/dependabot.yml` — automated dependency updates

**Testing:**
- Unit tests: `go test ./...` (standard, no build tag, runs in CI)
- Integration tests: `go test -tags=integration ./tests/integration/...` (local only, requires Kind + `~/.zitadel/config.yaml`)
- Integration tests are NOT in CI — they need real Zitadel + Docker for Kind

**Directory structure:**

```
repo/
├── cmd/<binary>/main.go     # Entry point
├── internal/                # Private packages
├── pkg/                     # Public importable packages (where applicable)
├── charts/<name>/           # Helm chart (multi-arch images)
├── deploy/                  # Pulumi Go examples (AWS Lambda + K8s deployment)
│   └── example/main.go     # Pulumi program consuming GitHub Release assets
├── .goreleaser.yaml         # Open-source GoReleaser (no Pro needed)
├── .github/workflows/       # GitHub Actions (build + release)
│   ├── ci.yml              # Lint + unit test on PR
│   └── release.yml         # GoReleaser on tag push
├── Justfile                 # Development commands (build, test, lint, generate)
├── devbox.json             # Development environment (Go, golangci-lint, controller-gen, etc.)
├── .envrc                  # direnv → devbox shell activation
├── go.mod
├── LICENSE                 # MIT
└── README.md
```

### Release & Distribution (all free, public)

| Artifact | Hosted on | Architectures | Published by |
|----------|-----------|---------------|-------------|
| Container images (multi-arch) | GHCR (`ghcr.io/truvity/<repo>`) | linux/amd64, linux/arm64 | GoReleaser (ko, multi-platform) |
| Helm charts (OCI) | GHCR (`oci://ghcr.io/truvity/charts/<name>`) | arch-agnostic (references multi-arch image) | GoReleaser post-hook |
| Lambda ZIPs | GitHub Release assets | linux/amd64, linux/arm64 | GoReleaser archives |
| Raw binaries | GitHub Release assets | linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 | GoReleaser builds |
| Go module | GitHub (proxy.golang.org auto-indexes) | — | `git tag vX.Y.Z` |

### Pulumi deployment examples

Each repo includes `deploy/example/` — a Pulumi Go program showing how to deploy from public GitHub assets to AWS:

```go
// deploy/example/main.go
// Demonstrates: download Lambda ZIP from GitHub Release → deploy to AWS Lambda
// Or: reference GHCR container image → deploy to K8s via Helm
```

This lets users copy the Pulumi example and customize for their environment. Not a published Pulumi component — just a reference implementation.

### GitHub Actions (CI only — no integration tests)

```yaml
# ci.yml — runs on every PR
- golangci-lint
- go test ./... (unit tests only, no build tag)
- go build

# release.yml — runs on tag push (v*)
- goreleaser release --clean
- builds multi-arch binaries (amd64 + arm64)
- builds multi-arch container images (ko, linux/amd64 + linux/arm64)
- creates Lambda ZIPs (amd64 + arm64)
- pushes container images to GHCR
- pushes Helm charts to GHCR (OCI)
- attaches binaries + ZIPs to GitHub Release
```

**Integration tests (Kind + real Zitadel) run locally only** — they require `~/.zitadel/config.yaml` with credentials and Docker for Kind. Not in CI.

### Repository creation tasks

- [ ] Create `truvity/zitadel-rbac-mapper` on GitHub (public, MIT)
  - LICENSE, README.md, .gitignore, devbox.json, .envrc, Justfile, go.mod, .goreleaser.yaml
  - `.github/workflows/ci.yml` + `.github/workflows/release.yml`
- [ ] Create `truvity/zitadel-notify-relay` on GitHub (public, MIT)
  - Same skeleton; initial backend: AWS SES (email) + AWS SNS (SMS)
  - Extensible provider interface for future backends
- [ ] Create `truvity/google-group-sync` on GitHub (public, MIT)
  - Same skeleton as above

---

1. ~~Should we support `organizationRef` in addition to `organizationId`?~~ → **Yes, Option C (either/or).**
2. Should the operator auto-discover the default org from the service account's org membership if `defaultOrganizationId` is omitted from config?
3. For integration tests: should the operator run as a pod inside Kind (more realistic) or as a local process with Kind's kubeconfig (faster iteration)? Likely both — local process for dev, pod for CI.
