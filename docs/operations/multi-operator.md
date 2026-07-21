# Multi-Operator Topologies

One operator deployment = one Zitadel instance = one binding credential. To manage several instances — or to split responsibility for one instance — you deploy several operators. This guide covers when and how.

## When you need multiple operators

- **Separate identity planes** — one cluster hosts both an internal employee IdP and an external customer IdP (different Zitadel instances).
- **Environment separation** — the development cluster's operator points at a testing org/instance; stage/prod operators point at the real customer instance.
- **Privilege separation on one instance** — a locked-down `iam-owner` operator owns instance state, while tenant workloads are served either by the same operator through scope maps or by separate `org-owner` operators.
- **Compliance boundaries** — ISO 27001 / SOC 2 style requirements that development tooling can never hold production identity credentials.

Note what you do **not** need extra operators for: serving many organizations on one instance. A single `iam-owner` operator with one `ScopeMap` per organization does that with per-scope delegated credentials — see [Large installations](large-installations.md).

## Reference topology: employee + customer

```
Cluster (devel)
├── zitadel-operator-employee        (namespace: zitadel-operator-employee)
│     domain:  auth.internal.example.com     binding: iam-owner
│     watchNamespaces: [zitadel-operator-employee, argocd, monitoring, gateway]
│     scope maps: infra org → project per concern
│
└── zitadel-operator-customer        (namespace: zitadel-operator-customer)
      domain:  auth.example.com  (devel: auth.internal.example.com, Testing org)
      binding: org-owner
      watchNamespaces: [zitadel-operator-customer, billing, storefront, api-platform]
      scope maps: customer org → project per product namespace
```

The employee operator manages infrastructure identity (ArgoCD, dashboards, gateways); the customer operator manages product identity. In devel, the customer operator points at a **Testing organization on the employee instance** with an `org-owner` binding — identical CRD behavior, zero customer-data risk. In stage/prod it points at the real customer instance.

### Values files

```yaml
# values-employee.yaml
config:
  domain: auth.internal.example.com
  binding: iam-owner
  watchNamespaces: [zitadel-operator-employee, argocd, monitoring, gateway]
rbac:
  namespaces: [zitadel-operator-employee, argocd, monitoring, gateway]
credentials:
  secretName: zitadel-employee-key
  key: key.json
```

```yaml
# values-customer-devel.yaml — Testing org on the employee instance
config:
  domain: auth.internal.example.com
  binding: org-owner              # credential is ORG_OWNER in the Testing org only
  watchNamespaces: [zitadel-operator-customer, billing, storefront, api-platform]
rbac:
  namespaces: [zitadel-operator-customer, billing, storefront, api-platform]
credentials:
  secretName: zitadel-customer-key
  key: key.json
```

For prod, only `domain` (and the credential Secret contents) change. Each operator then gets its own scope maps in its own namespace — routing surfaces never mix, because every map must assert `spec.instance` matching its operator.

## Isolation layers

Isolation is enforced at three levels, outermost first:

```
┌────────────────────────────────────────────────────────────┐
│ 1  K8s RBAC (rbac.namespaces)         API-server enforced  │
│    └─ 2  watchNamespaces               informer filter     │
│        └─ 3  Scope maps + delegation   semantic routing,   │
│              (spec.instance assertion)  fail-closed,       │
│                                         per-scope creds    │
└────────────────────────────────────────────────────────────┘
```

1. **RBAC** — in namespaced mode the operator's ServiceAccount can only read/write CRs in its declared namespaces; a misconfigured watch list cannot override this.
2. **`watchNamespaces`** — events from other namespaces never enter the cache.
3. **Scope maps** — inside the watched set, every namespace must resolve to exactly one scope, reconciled with a delegated credential that is API-limited to that scope. Cross-instance contamination is blocked by the mandatory `spec.instance` assertion on maps and, for shared namespaces, by CR-level `spec.instance` pins ([dual-serving](dual-serving.md)).

## Deployment order

1. Create namespaces (with tenancy labels, if selector rules will use them) — via GitOps/IaC.
2. Install the **CRD chart once** (shared by all operators).
3. Create each operator's credential Secret (ESO/External Secrets from a cloud store).
4. Install each operator release with its own values.
5. Create scope maps in each operator's namespace.
6. Apply tenant CRs.

Notes:

- CRDs are cluster-scoped: one install, never per operator. Operators act independently on the shared definitions.
- Rotating one operator's credential restarts only that operator.
- Adding a namespace to an operator: label it (if selector-routed), add it to `watchNamespaces` **and** `rbac.namespaces`, upgrade the release, and ensure a scope-map rule covers it.
- Two operators for **different instances** may deliberately share a namespace — that is [dual-serving](dual-serving.md), and CRs there need `spec.instance` pins. Two operators for the **same instance** must keep disjoint namespace sets.

## Leader election with co-located deployments

Each release derives its lease ID from its Helm fullname, so any number of operator deployments — even in the same namespace — hold distinct leases. Never disable leader election in a real deployment: a rolling update always runs two pods of the same operator simultaneously.

## Troubleshooting multi-operator setups

See [Troubleshooting](troubleshooting.md) — in particular `InstanceMismatch` (a map asserting the wrong instance), `AmbiguousInstance` (unpinned CR in a dual-served namespace), and "CR not reconciled at all" (namespace missing from `watchNamespaces`/RBAC).
