# Resource Hierarchy

The CRD surface mirrors Zitadel's data model: an **instance** contains **organizations**, organizations contain **projects** and **users**, projects contain **applications** and **grants**. The operator binds to one instance at deployment time (config, not a CRD); everything below that line is declarative.

```
Zitadel instance                       ← operator config (domain + binding), not a CRD
└── Organization
    ├── Project
    │   ├── OIDCApp · APIApp · SAMLApp
    │   ├── ApplicationKey
    │   ├── ProjectMember · ProjectGrant · ProjectGrantMember
    │   └── roles (inline in Project spec)
    ├── MachineUser ── PersonalAccessToken
    ├── HumanUser
    ├── UserGrant · OrgMember
    ├── IdentityProvider (org-scoped OIDC)
    ├── LoginPolicy · PasswordComplexityPolicy · LockoutPolicy · PasswordAgePolicy
    │   · NotificationPolicy · LabelPolicy · PrivacyPolicy
    ├── MessageText
    └── Domain · OrgMetadata
(instance level)
    ├── Default* policies (Login, Domain, Lockout, PasswordComplexity, PasswordAge,
    │   Notification, Label, Privacy, OIDCSettings) · DefaultMessageText
    ├── GoogleIdP · GitHubIdP
    ├── EmailProvider · SmsProvider
    ├── ActionTarget · ActionExecution
    └── InstanceMember
(operator routing)
    └── ScopeMap                ← operator namespace only; see Scope resolution
```

**All 43 CRDs are namespaced.** Namespaces are the tenancy and RBAC boundary: a platform team manages `Organization`/`Project` CRs in its own namespace, application teams create `OIDCApp`/`MachineUser` CRs in theirs, and Kubernetes RBAC plus [scope maps](scope-resolution.md) keep them apart. Every spec/status field is documented in the generated [API reference](../reference/api.md).

## CRD map by required permission

What a CRD needs determines which [binding level](../install/binding-levels.md) (or which delegate) can serve it:

| Level | CRDs | Notes |
| --- | --- | --- |
| **Project** (`PROJECT_OWNER` suffices) | OIDCApp, APIApp, SAMLApp, ApplicationKey, ProjectMember, ProjectGrantMember | Secret outputs: OIDCApp/APIApp (`client_id`, `client_secret`), ApplicationKey (`key.json`) |
| **Organization** (`ORG_OWNER`) | Organization*, Project, MachineUser, HumanUser, PersonalAccessToken, UserGrant, ProjectGrant, OrgMember, OrgMetadata, Domain, IdentityProvider, LoginPolicy, PasswordComplexityPolicy, LockoutPolicy, PasswordAgePolicy, NotificationPolicy, LabelPolicy, PrivacyPolicy, MessageText | Secret outputs: MachineUser (connection bundle), PersonalAccessToken (`token`). *Creating an Organization needs instance level. |
| **Instance** (`IAM_OWNER`) | Default* policies, DefaultOIDCSettings, DefaultMessageText, GoogleIdP, GitHubIdP, EmailProvider, SmsProvider, ActionTarget, ActionExecution, InstanceMember | Under an `org-owner` binding these degrade to `Ready=False / NotSupportedAtBindingLevel` |
| **Operator routing** | ScopeMap | Evaluated only in the operator's namespace; admin-controlled |

## Reference pattern: `*Ref` XOR `*Id`

Every resource references only its **direct parent**. The organization is inherited through the chain — an app never names its org, it gets it from its project.

```yaml
spec:
  # Option A: reference a CR managed by this operator (stays in sync)
  projectRef:
    name: my-project
    namespace: platform      # optional; defaults to the CR's namespace
  # Option B: pin a pre-existing Zitadel resource by raw ID
  projectId: "326225093250303536"
```

| Resource level | References | Org resolution |
| --- | --- | --- |
| Organization | nothing (top level) | is the org |
| Project | Organization | explicit or scope-defaulted |
| OIDCApp, APIApp, SAMLApp, ApplicationKey, ProjectMember, ProjectGrant(Member) | Project | inherited from the project |
| MachineUser, HumanUser, IdentityProvider, org policies, UserGrant, OrgMember, … | Organization | explicit or scope-defaulted |

Resolution rules:

| Fields set | Result |
| --- | --- |
| `*Ref` | Look up the referenced CR (same or named namespace) and use its `status.*Id`; while the parent is not Ready, the child waits with `Ready=False / <Parent>NotReady` and requeues |
| `*Id` | Used directly |
| both | Validation error (mutually exclusive) |
| neither | **Scope-defaulted** (v0.18): in a namespace routed by a scope map, the scope supplies the organization — and for project-scope rules also the project. In passthrough mode, omitting a required parent is an error (`InvalidSpec`). |

Scope-defaulting is what lets a tenant namespace stay boilerplate-free: an `OIDCApp` in a project-scoped namespace needs neither `projectRef` nor `projectId`. The resolved IDs are recorded in the CR's status (`status.organizationId`, `status.projectId`) so deletion keeps working even after maps are removed. A CR that explicitly names an org/project *outside* its namespace's resolved scope fails closed (see [Scope resolution](scope-resolution.md)).

## Cross-namespace references

`*Ref` accepts an optional `namespace`, so a platform-owned `Project` in `platform` can be referenced by an `OIDCApp` in `argocd`. RBAC still applies — the operator must be able to read both namespaces.

## Recurring CRD patterns

### Paired policies (instance default + org override)

Most policies exist twice: an instance-level singleton (`DefaultLockoutPolicy`, Admin API) and an org-scoped override (`LockoutPolicy`, Management API). Both share one field struct in the API (`policy_fields.go`), so their schemas never drift. Deleting an org-scoped policy resets the org to the instance default. Deleting an instance default leaves instance state untouched **unless** the CR carries the opt-in annotation:

```yaml
metadata:
  annotations:
    zitadel.truvity.io/reset-on-delete: "true"
```

### Singleton semantics

Instance defaults always exist in Zitadel — there is no "create". The operator reads current state, detects drift, updates. Only one CR per `Default*` kind may manage the instance: the earliest-created CR wins; later duplicates get `Ready=False / DuplicateSingleton` and take over automatically if the winner is deleted (ties on the 1-second creation-timestamp granularity break deterministically by namespace/name). For `DefaultMessageText`, uniqueness is per `(type, language)` pair.

### Type discriminator (MessageText)

Where Zitadel exposes many structurally identical resources selected by API method, one CRD carries a `type` field (`init`, `passwordReset`, `verifyEmail`, …) instead of ten near-identical CRDs. The `verifySmsOtp` type only supports `language` + `text`; setting email-only fields yields a non-blocking `SmsFieldWarning` condition.

### Secret references in, Secrets out

Sensitive **inputs** (IdP client secrets, SMTP passwords) are never spec literals — they are `SecretKeyRef`s into Kubernetes Secrets (missing Secret ⇒ `Ready=False / SecretNotFound`, requeue until it appears). Credential **outputs** (client secrets, machine keys, tokens) are written to Secrets named by the CR's `secretRef`/`keySecretRef`, with customizable key names and optional `extraData`. The MachineUser output is a full connection bundle (`key.json`, `instanceUrl`, `issuer`, `orgId`, `projectId`, best-effort `instanceId`) — enough to construct a working Zitadel client from the Secret alone.

### Lifecycle: finalizers, adoption, drift

- Every managed CR gets the `zitadel.truvity.io/finalizer`; deletion removes the Zitadel resource first (blocking — the finalizer stays until cleanup succeeds), then the CR. Removing the finalizer manually orphans the Zitadel resource deliberately.
- **Adoption**: if a matching resource already exists in Zitadel (same name under the same parent), the operator adopts it instead of duplicating; confidential apps regenerate their client secret exactly once when the output Secret lacks one.
- **Drift detection**: the operator is the single writer inside its managed scope. All mutable fields are compared on every reconcile (period: 5 minutes); out-of-band edits are reverted. No drift = no API write (idempotent reconcile).

Status semantics, conditions, and the SSA write discipline are specified in [Status & SSA](status-and-ssa.md).
