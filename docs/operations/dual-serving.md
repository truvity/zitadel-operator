# Dual-Serving One Namespace

Two operator deployments bound to **different Zitadel instances** can deliberately serve the same namespace — e.g. a `billing` namespace holding a customer-facing app (customer instance) and an employee-facing admin dashboard (internal instance). The contract that makes this safe is specified in [architecture: dual-serving](../architecture/dual-serving.md); this page is the operational how-to.

## Setup checklist

1. **Both operators watch the namespace** — it appears in each release's `config.watchNamespaces` *and* `rbac.namespaces`.
2. **Each operator has a scope-map rule covering the namespace** (in its own operator namespace, for its own instance) — or runs in passthrough mode.
3. **Every CR in the namespace is pinned** with `spec.instance`:

```yaml
apiVersion: zitadel.truvity.io/v1alpha2
kind: OIDCApp
metadata:
  name: billing-admin-dashboard
  namespace: billing
spec:
  instance: internal            # employee operator's instanceAlias
  redirectUris: [...]
  secretRef: {name: billing-admin-oidc}
---
apiVersion: zitadel.truvity.io/v1alpha2
kind: OIDCApp
metadata:
  name: billing-customer-app
  namespace: billing
spec:
  instance: customer            # customer operator's instanceAlias
  redirectUris: [...]
  secretRef: {name: billing-customer-oidc}
```

The pin value is the operator's **instance identity** — its configured `instanceAlias`, or `domain` when no alias is set. Prefer an alias: it survives instance domain migrations without touching a single pin.

## What happens without a pin

An unpinned CR in a dual-served namespace is **fail-closed by both operators**: each marks `InstanceResolved=False / AmbiguousInstance` via SSA (their conditions coexist — distinct field managers), and neither touches Zitadel. Nothing breaks, nothing races; the CR simply waits for a pin. This is by design: guessing an instance for identity resources is worse than stopping.

```bash
kubectl -n billing get oidcapp billing-new-app -o jsonpath='{.status.conditions}' | jq
# ... "type":"InstanceResolved","status":"False","reason":"AmbiguousInstance",
#     "message":"namespace is served by multiple zitadel operators; set spec.instance ..."
```

Fix: add `spec.instance`. The pinned operator proceeds on its next reconcile; the other stops writing status entirely.

## Handover between instances

To move a CR from instance A to instance B:

1. Change `spec.instance` from A's identity to B's identity.
2. Operator A stops touching the CR (its finalizer cleanup for A-side resources is **not** run by repinning — the recorded IDs simply become foreign; delete + recreate the CR instead if you want the A-side resource removed).
3. Operator B adopts by name or creates fresh, and rewrites the output Secret with B-side credentials.

For a clean migration of a credentialed app, prefer: create a new pinned CR for B, cut consumers over, then delete the A-pinned CR (A's finalizer cleans up its instance).

## Rollout order for making a namespace dual-served

Starting from a namespace served by operator A only:

1. Pin **all existing CRs** in the namespace to A's domain (`spec.instance: <A>`). Unpinned CRs would fail closed the moment B appears.
2. Add the namespace to operator B's `watchNamespaces`/RBAC and scope maps.
3. Create B-pinned CRs.

## Requirements recap

- Distinct instances — dual-serving is not for two operators on the *same* instance (keep those on disjoint namespaces).
- Both operators at v0.18+ (SSA status discipline; distinct `zitadel-operator/<instance identity>` field managers).
- Leader election on with per-release lease IDs (chart default).
