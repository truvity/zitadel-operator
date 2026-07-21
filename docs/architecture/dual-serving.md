# Dual-Serving Semantics

Dual-serving is the contract for the case where **two operator deployments bound to different Zitadel instances legitimately serve the same namespace** — for example a product namespace holding both an employee-facing dashboard app (internal instance) and a customer-facing app (customer instance).

Every tenant CR kind carries an optional `spec.instance` — a pin naming the Zitadel domain the CR belongs to. Each operator evaluates the pin against its own configured `domain`:

| `spec.instance` on the CR | This operator's behavior |
| --- | --- |
| Matches this operator's domain | Reconcile normally through the resolved scope; `InstanceResolved=True / Pinned` |
| A foreign domain | **Completely untouched** — no finalizer, no status writes, no Zitadel calls. The CR belongs to the other operator. |
| Unset, no other operator seen | Reconcile normally; the operator announces itself first (`InstanceResolved=True / Assumed`) |
| Unset, namespace is dual-served | **Both operators fail closed**: `InstanceResolved=False / AmbiguousInstance`, no external action from either |

Repinning a CR from one domain to the other hands it over cleanly: the old owner stops touching it (its recorded IDs do not exist on the other instance, so even deletion is a server-side no-op there), the new owner adopts or creates.

## How ambiguity is detected

There is no coordination channel between the operators — detection rides on Server-Side Apply managed fields:

1. Before its first external action on an unpinned CR, an operator SSA-writes a presence condition (`InstanceResolved=True / Assumed`) through its own field manager `zitadel-operator/<domain>`.
2. Every operator checks the CR's `managedFields` for a *different* `zitadel-operator/*` field manager.
3. If one exists, the namespace is dual-served: the operator sets `InstanceResolved=False / AmbiguousInstance` and stops. The condition content is deterministic and identical from every operator, so co-ownership converges instead of flapping.

Setting `spec.instance` resolves the ambiguity; during deletion the gate is skipped so finalizer cleanup always proceeds.

## Prerequisites (why this needs v0.18 machinery)

- **[SSA status discipline](status-and-ssa.md)** — two writers on one status subresource requires per-field-manager ownership; read-modify-write status updates provably wipe the other writer's conditions.
- **Distinct field managers** — the manager name embeds the instance domain, which is what makes the foreign-manager scan meaningful.
- **Distinct leader-election leases** — the Helm chart derives the lease ID from the release fullname, so two deployments coexist in one namespace.
- Both operators need the namespace in their `watchNamespaces`/RBAC set, and each needs a scope-map rule (or passthrough) covering it.

Operational how-to (values files, rollout order, verification): [Dual-serving one namespace](../operations/dual-serving.md).
