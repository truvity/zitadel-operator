# Deployment Shapes

The operator supports several topologies. This page names them, recommends one, and tells you when to reach for the alternatives. It assumes you have read [Binding levels](binding-levels.md).

## Recommended: the fleet shape — one org = one SA = one operator

Since v0.19 the recommended way to serve one Zitadel instance with several organizations is a **fleet of small `org-owner` operators**: each organization gets its own service account (`ORG_OWNER` in exactly that org), and each cluster that hosts workloads for the org runs one operator deployment bound with that credential.

```
Zitadel instance (alias: prod)
├── org "acme"      ← SA acme-operator      (ORG_OWNER in acme only)
│     └── cluster A: deployment zitadel-operator-acme
│           binding: org-owner, watchNamespaces: [acme-*, ...]
└── org "globex"    ← SA globex-operator    (ORG_OWNER in globex only)
      └── cluster A: deployment zitadel-operator-globex
            binding: org-owner, watchNamespaces: [globex-*, ...]
```

What makes this the default recommendation:

- **The credential is the scope.** An `org-owner` binding is structurally incapable of touching another org — no routing layer required. The v0.18 exactly-one-org assertion is the design, not a limitation.
- **No ScopeMaps needed.** Namespace selection is done with `watchNamespaces` plus per-namespace RBAC (`rbac.namespaces` in the chart); everything the operator can see belongs to its org. ScopeMaps remain fully supported for the single-big-operator alternative below.
- **Projects are declared in-namespace.** A `Project` CR (and its `ProjectRole` CRs) live next to the workload that needs them; project-scoped CRs (`OIDCApp`, `APIApp`, `SAMLApp`, `MachineUser` role grants, `UserGrant`, members, keys) name their project with `spec.projectRef`.
- **Small blast radius everywhere:** per-org credentials, per-org RBAC, per-org upgrade cadence.

### Values sketch

```yaml
# values-acme.yaml
config:
  domain: auth.example.com
  instanceAlias: prod           # stable identity; survives domain migration
  binding: org-owner            # SA is ORG_OWNER in org "acme" only
  watchNamespaces: [zitadel-operator-acme, acme-billing, acme-api]
rbac:
  namespaces: [zitadel-operator-acme, acme-billing, acme-api]
credentials:
  secretName: zitadel-acme-key
  key: key.json
```

### In-namespace project declaration

```yaml
apiVersion: zitadel.truvity.io/v1alpha2
kind: Project
metadata:
  name: billing
  namespace: acme-billing
spec:
  organizationId: "312312312312312312"   # the bound org
---
apiVersion: zitadel.truvity.io/v1alpha2
kind: ProjectRole
metadata:
  name: billing-reader
  namespace: acme-billing
spec:
  projectRef: {name: billing}
  key: "acme-billing:reader"             # mechanical {namespace}:{role} vocabulary
  displayName: Billing reader
---
apiVersion: zitadel.truvity.io/v1alpha2
kind: MachineUser
metadata:
  name: billing-bot
  namespace: acme-billing
spec:
  organizationId: "312312312312312312"
  userName: billing-bot
  projectRef: {name: billing}            # grant target (v0.19)
  roles: ["acme-billing:reader"]
  keySecretRef: {name: billing-bot-key}
```

`ProjectRole` manages exactly one role key per CR (create-or-adopt, drift-corrected `displayName`/`group`, removed on CR deletion). Use it instead of `Project` `spec.roles` when several namespaces or charts contribute roles to one project — `spec.roles` is an authoritative full-set sync and would remove keys it does not know about. Do not combine both for the same project.

## Sharing a namespace between operators

Two situations can put two operators in front of the same CR. They are handled by two different mechanisms:

### Different instances: `spec.instance` pins (v0.18)

Operators bound to **different** Zitadel instances may deliberately dual-serve a namespace. Every CR there pins its owner with `spec.instance` (the operator's `instanceAlias`); unpinned CRs fail closed with `AmbiguousInstance`. See [Dual-serving one namespace](../operations/dual-serving.md).

### Same instance: the ForeignManager guard (v0.19)

Two fleet operators serve the **same instance**, so their SSA field managers are identical and the v0.18 gate cannot tell them apart. Their namespace selections should be disjoint — but a mis-scoped `watchNamespaces`/RBAC overlap must not let two operators fight over one CR (each would try to reconcile it into *its own org*).

The guard makes ownership explicit:

- On first reconcile the operator **stamps the CR** with its management identity:

  ```
  metadata.annotations:
    zitadel.truvity.io/managed-by: prod/org/312312312312312312
  ```

  The identity is `<instanceAlias>/org/<boundOrganizationId>` for `org-owner` bindings (stable across restarts, upgrades and re-releases) and `<instanceAlias>/ns/<operatorNamespace>` for `iam-owner`.

- An operator whose identity does **not** match the annotation sets a `ForeignManager` condition and skips the CR entirely — including deletion, because a same-instance foreign delete would remove a *real* resource in the wrong org. The condition is written with a dedicated `zitadel-guard/<identity>` field manager, so it coexists with the owner's conditions and never disturbs its status ownership.

- **Transferring ownership** (e.g. the owning operator was decommissioned): edit or remove the annotation —

  ```bash
  kubectl -n acme-billing annotate project billing zitadel.truvity.io/managed-by-   # release
  ```

  The next operator to reconcile the CR adopts it. Foreign-managed CRs re-check on the periodic interval (5 minutes); bump the spec (any field) to pick a transfer up immediately.

The guard is protective, not a routing mechanism: fix the overlapping `watchNamespaces`/RBAC — the guard just guarantees that a misconfiguration cannot corrupt state while you do.

## Supported alternatives

Both v0.18 topologies remain fully supported and are the right tool in specific situations:

| Shape | When to prefer it | Docs |
| --- | --- | --- |
| **One `iam-owner` operator + ScopeMaps** | Many orgs, centrally administered; per-scope delegated credentials; instance-level resources (`Default*` policies, instance IdPs, org creation) managed as CRs | [Scope maps](../operations/scope-maps.md), [Large installations](../operations/large-installations.md) |
| **Mixed: `iam-owner` platform operator + `org-owner` fleet** | A platform team owns instance state while product teams run fleet operators for their orgs | [Multi-operator topologies](../operations/multi-operator.md) |
| **Dual-serving across instances** | One namespace holds CRs for two different Zitadel instances | [Dual-serving](../operations/dual-serving.md) |

Note that the fleet shape deliberately gives up instance-level resources: under `org-owner` they degrade to `Ready=False / NotSupportedAtBindingLevel` ([binding levels](binding-levels.md)). Keep one `iam-owner` owner for instance state — an operator elsewhere, or IaC.
