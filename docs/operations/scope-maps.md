# Scope-Map Administration & Delegation

`ScopeMap` objects are the routing surface of the operator: they decide which Zitadel scope each tenant namespace reconciles into. This guide covers day-2 administration — creating maps, delegating their maintenance to teams with narrowly scoped RBAC, rollout, and decommissioning. Semantics (rule matching, fail-closed taxonomy) live in [Scope resolution](../architecture/scope-resolution.md).

## Ground rules

- Maps live **only in the operator's namespace** (`operatorNamespace`); anywhere else they are inert.
- One map per Zitadel **organization** is the intended granularity — the map name identifies the org it serves, which is what makes per-map RBAC delegation meaningful.
- Every map must assert `spec.instance` = the operator's instance identity (its `instanceAlias`, or `domain` when no alias is set). Wrong instance ⇒ the whole map is fail-closed (`InstanceMismatch`).
- Map **creation stays admin-only**; per-map **maintenance is delegable** (see below).
- Namespace labels used by selector rules are GitOps-stamped facts. Never let a tenant team write the labels that route its own namespaces.

## Creating a map

```yaml
apiVersion: zitadel.truvity.io/v1alpha2
kind: ScopeMap
metadata:
  name: acme-corp                        # convention: the org it serves
  namespace: zitadel-operator
spec:
  instance: prod-internal                # the operator's instanceAlias (or domain)
  organization: Acme Corp
  organizationId: "325908855630427886"   # recommended: pin the ID (authoritative)
  rules:
    - name: product-namespaces
      namespaceSelector:
        matchLabels:
          tenancy.example.com/company: acme
      project: acme-platform             # project scope; omit for org scope
    - name: ops
      namespaces: [acme-ops]
```

Recommendations:

- **Pin `organizationId`.** With only a name, the map controller must resolve it (transient `ScopeMapNotReady` until it does), and an org rename in Zitadel changes what the map means. With the ID pinned, a renamed org merely produces an `OrganizationNameDrift` condition (plus a Warning Event) on the map.
- **Prefer selector rules for fleets, literal rules for exceptions.** A selector rule absorbs new namespaces automatically as GitOps stamps labels; literal rules pin exact names.
- **Names are for humans, IDs for machines.** `organizationId`/`projectId` are authoritative when set; the paired name is then optional and informational (a set-but-different org name surfaces as an `OrganizationNameDrift` condition + Event). At least one of `organization`/`organizationId` is required; a rule may use `projectId` with or without a `project` name.
- Rule order matters within a map (first match top-down), and maps are evaluated name-sorted. Keep one namespace matched by exactly one rule in exactly one map — cross-map matches are conflicts, fail-closed on both maps with Warning Events.

Verify:

```bash
kubectl -n zitadel-operator get scopemaps
# NAME        INSTANCE            ORGANIZATION   READY
# acme-corp   auth.example.com    Acme Corp      true
kubectl -n zitadel-operator describe scopemap acme-corp   # conditions + Events
```

## Delegating map maintenance to a team

The intended trust split, straight from Kubernetes RBAC mechanics:

- **Admins create maps.** `resourceNames` cannot gate `create` ([kubernetes#80295](https://github.com/kubernetes/kubernetes/issues/80295) — at admission the name may not exist yet), so anyone with `create` on scope maps can create *any* map. Keep `create` (and `delete`) with platform admins.
- **Teams maintain their own map** via a Role restricted with `resourceNames` to exactly the named map(s).

### Recipe: Acme's platform team maintains `acme-corp`

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: scopemap-acme-corp-maintainer
  namespace: zitadel-operator            # the operator's namespace
rules:
  - apiGroups: ["zitadel.truvity.io"]
    resources: ["scopemaps"]
    resourceNames: ["acme-corp"]
    verbs: ["get", "update", "patch"]
  - apiGroups: ["zitadel.truvity.io"]
    resources: ["scopemaps"]
    verbs: ["list", "watch"]             # list/watch cannot be name-scoped;
                                         # grants visibility of map names/specs only
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: scopemap-acme-corp-maintainer
  namespace: zitadel-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: scopemap-acme-corp-maintainer
subjects:
  - kind: Group
    name: acme-platform-team             # or a ServiceAccount / User
    apiGroup: rbac.authorization.k8s.io
```

What the team can now do: edit `acme-corp`'s rules (add/remove namespaces, change projects) and read its status/conditions. What it cannot do: create new maps, delete the map, touch any other org's map, or touch anything else in the operator namespace. If `list` across maps is too much visibility, drop the second rule — `kubectl get scopemap acme-corp` (a name-scoped `get`) still works.

### Recipe: read-only auditors

```yaml
rules:
  - apiGroups: ["zitadel.truvity.io"]
    resources: ["scopemaps"]
    verbs: ["get", "list", "watch"]
```

### GitOps variant

When maps are applied by a GitOps controller, delegation moves to the repository layer: per-map CODEOWNERS on the map files, admin-owned pipeline applying them. The RBAC recipes above then apply to the GitOps applier's ServiceAccount rather than humans, and `resourceNames` still caps the blast radius of a compromised per-team applier.

Note for SSA-based appliers (Argo CD server-side apply, Flux): `resourceNames`-scoped `patch` works, but the applier must not need `create` — pre-create the map once as admin.

## Rollout: from zero maps to routed

The zero-maps state is **passthrough** (pre-v0.18 behavior), so rollout is incremental, org by org — but the **first map you create ends passthrough for every namespace the operator serves**: from that moment, any namespace matching no rule is fail-closed (`NoMatchingRule`; confirmed rejects re-check on the 5-minute periodic interval — spec edits still reconcile immediately).

1. Inventory the namespaces the operator serves (check `watchNamespaces`, or all namespaces in cluster mode) and every org/project their CRs use.
2. Write maps covering **all** of them — not just the first org you care about.
3. Apply the maps in one change; watch for `ScopeResolved=False` conditions and map Events.
4. Only then start removing now-redundant explicit `organizationId`/`projectRef` fields from tenant CRs (optional — explicit references that agree with the scope remain valid).

## Delegate hygiene

Delegated service accounts are operator-managed; you should never create, modify, or delete them by hand. What to know for operations:

- They appear in Zitadel as machine users `zitadel-operator-delegate-<hash>`, with `ORG_OWNER` or `PROJECT_OWNER` on exactly their scope; their keys live in Secrets `zitadel-delegation-<hash>` (label `zitadel.truvity.io/delegation`) in the operator namespace.
- Deleting a delegate in Zitadel out-of-band is self-healing: the operator lazily re-mints on next use.
- Deleting the Secret out-of-band forces a re-mint as well; the orphaned Zitadel SA is swept by the periodic GC.
- Removing a rule/map eagerly revokes the delegates it produced. Full lifecycle: [Internal delegation](../architecture/delegation.md).

```bash
# List live delegation Secrets
kubectl -n zitadel-operator get secrets -l zitadel.truvity.io/delegation
```

## Decommissioning a tenant

Order matters — a delegate must outlive the tenant CRs it serves:

1. Delete the tenant CRs (finalizers clean up Zitadel resources via the delegate).
2. Remove the rule (or the map) — this eagerly revokes the delegate and deletes its Secret.
3. Remove namespace labels / namespaces themselves.

Doing it backwards does not deadlock — deletion falls back to the binding client — but it turns a clean, delegate-authored teardown into binding-authored cleanup, which is worth avoiding for audit hygiene.
