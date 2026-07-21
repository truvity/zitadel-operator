# Binding Levels: `iam-owner` vs `org-owner`

Every operator deployment mounts exactly one Zitadel credential and declares what that credential is:

```yaml
config:
  binding: iam-owner   # or org-owner
```

This is the **first deployment decision** you make, because it determines the operator's blast radius, which CRDs it can serve, and how it degrades when a resource asks for more than the credential can do.

## What the assertion means

| Binding | The credential must be | The operator serves |
| --- | --- | --- |
| `iam-owner` | A service account with instance-level `IAM_OWNER` | The entire CRD surface: instance-level resources, any organization, any project |
| `org-owner` | A service account with `ORG_OWNER` in **exactly one** organization (and no `IAM_OWNER`) | Everything inside that one organization |

At startup the operator calls `AuthService.ListMyMemberships` and verifies the assertion **in both directions**:

- Asserting `iam-owner` with a lesser credential fails — otherwise you would get confusing permission errors at reconcile time instead of one clear crash.
- Asserting `org-owner` with an `IAM_OWNER` credential also fails — the operator refuses to run with more privilege than declared.
- Asserting `org-owner` with a credential that is `ORG_OWNER` in several organizations fails — the binding must identify exactly one bound org.

A verification failure crashes the pod before any reconcile. This is intentional: the binding is a security boundary, not a hint.

## Instance-level deployment (`iam-owner`)

Choose `iam-owner` when the operator is the platform-level manager of a Zitadel instance:

- Bootstraps and owns instance defaults (`Default*` policies, `DefaultOIDCSettings`, `DefaultMessageText`)
- Manages instance-scoped IdPs (`GoogleIdP`, `GitHubIdP`), notification providers (`EmailProvider`, `SmsProvider`), Actions v2 (`ActionTarget`, `ActionExecution`), `InstanceMember`
- Creates organizations (`Organization` CR)
- Serves scope maps for **any** organization on the instance — the natural fit for [large multi-tenant installations](../operations/large-installations.md) where one operator serves many customer orgs

The cost is credential weight: the mounted key is an instance admin. The [internal delegation](../architecture/delegation.md) model contains this — the binding credential is only used to mint/rotate/revoke scope-limited delegates, and every tenant CR is reconciled with a delegate key — but the key in the Secret still *is* IAM_OWNER. Protect the operator namespace accordingly.

## Org-level deployment (`org-owner`)

Choose `org-owner` when the operator should be structurally incapable of touching anything outside one organization:

- Application teams' identity automation for a single org (apps, service accounts, grants, org policies)
- Delegated administration: a central team runs the instance (via Pulumi/Terraform or an `iam-owner` operator elsewhere), and each org gets its own operator with an org-scoped credential
- Compliance boundaries — e.g. a development cluster's operator bound to a "Testing" org, with no code path to production organizations

### Degradation is uniform, not per-feature

Under `org-owner` the operator still installs all controllers. What a lesser binding cannot do fails closed with one consistent signal:

| Situation | Behavior |
| --- | --- |
| Instance-level CR (any `Default*` policy, instance IdP, provider, Action, `InstanceMember`, `Organization` creation) | `Ready=False / NotSupportedAtBindingLevel` — no Zitadel call is attempted |
| Scope map whose organization is not the bound org | Map fails closed `NotSupportedAtBindingLevel` + `ForeignOrganization` Warning Event |
| Org/project resources inside the bound org | Reconciled normally (including delegation, which mints `PROJECT_OWNER` delegates for project scopes) |

There are no per-feature special cases: one validation matrix (binding level × scope shape) decides. Moving a CR to an `iam-owner`-bound operator, or upgrading the credential + assertion, immediately unblocks it.

## Choosing

| Question | If yes → |
| --- | --- |
| Does anything need instance defaults, instance IdPs, notification providers, or Actions? | `iam-owner` (at least one such deployment per instance) |
| Should this deployment create organizations or serve scope maps for many orgs? | `iam-owner` |
| Is the deployment for one org's teams, or in a lower-trust environment? | `org-owner` |
| Unsure? | Start with `org-owner` — the failure mode is a visible `NotSupportedAtBindingLevel` condition, never silent over-privilege. |

A common production pattern combines both: one `iam-owner` operator in a locked-down platform namespace owning instance state, plus `org-owner` operators (or scope-mapped namespaces under the platform operator) for tenant workloads. See [Multi-operator topologies](../operations/multi-operator.md).

## Relation to delegation

The binding credential is the *minting* authority, not the working credential. Regardless of binding level, once a namespace resolves through a scope map the operator reconciles it with a delegated service account limited to that scope (`ORG_OWNER` on the scope org, or `PROJECT_OWNER` on the scope project). Auditing Zitadel events shows the delegate — not the binding — as the author of tenant-resource changes. Details: [Internal delegation](../architecture/delegation.md).
