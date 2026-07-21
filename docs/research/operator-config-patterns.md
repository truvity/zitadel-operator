# Operator Configuration Patterns — Comparative Research (v0.18 routing surface)

Question answered: **should the v0.18 scope-map routing surface be ConfigMaps or a
CRD?** Decided 2026-07-21 for the v0.18 design (INF-422/INF-423). Prototype
evidence lives in `proto-v018-findings.md` (branch `proto/v018-scope-maps`);
this document records the comparative survey and the verdict.

## Verdict

**Namespaced CRD (`ZitadelScopeMap`), living in the operator's namespace.
Not ConfigMaps.**

1. **Zero precedent.** None of the surveyed operators uses ConfigMaps for
   per-tenant *semantic routing*. ConfigMaps appear only as operator-global
   settings (ArgoCD's `argocd-cm`); every surveyed project that needed a
   delegable, validated, per-tenant surface built a CRD for it.
2. **Label-driven routing is the CVE pattern.** Capsule's namespace-label
   tenancy routing produced CVE-2025-55205 (cross-tenant namespace capture via
   label manipulation). Routing authority must live in objects the routed
   tenant cannot write — our maps sit in the operator namespace; namespace
   labels are GitOps-stamped facts that maps *interpret*.
3. **RBAC delegation works the same on CRs.** K8s RBAC `resourceNames` grants
   per-object update/patch/delete on custom resources exactly as on
   ConfigMaps. `create` cannot be gated by `resourceNames` either way
   ([kubernetes#80295]) — so map creation stays admin-only, which matches the
   intended trust split (admin mints a company's map; the company's platform
   team maintains it).
4. **The CRD is strictly richer.** OpenAPI schema validation at admission,
   a status subresource (conditions: `InstanceMatch`, `Ready`), printer
   columns, and Events with a typed subject. A ConfigMap gives none of these;
   validation errors would surface only as Events on an untyped object.

## Survey

### ArgoCD — ConfigMaps for instance config, CRD for the tenant surface

ArgoCD keeps operator-global configuration in well-known ConfigMaps
(`argocd-cm`, `argocd-rbac-cm`) — settings owned by the ArgoCD admins,
never delegated. The moment configuration became *per-tenant* (which repos,
destinations, and clusters a team may deploy to), ArgoCD introduced the
**`AppProject` CRD**: schema-validated, status-bearing, RBAC-delegable.
That split — ConfigMap for the instance, CRD for the delegated tenancy
surface — is exactly the line our scope maps sit on, and they sit on the
CRD side of it.

- https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/
- https://argo-cd.readthedocs.io/en/stable/user-guide/projects/

### Keycloak — the single-writer lesson

The legacy Keycloak operator partially managed realms: state was imported,
then out-of-band edits diverged silently — the operator neither owned nor
reconciled what it had created. The rewritten operator made the ambiguity
explicit: **`KeycloakRealmImport` is deliberately one-shot** ("the operator
does not watch or reconcile subsequent changes"), an admission that
half-ownership is untenable. v0.18 takes the other branch: ownership is
*scoped* (by the maps), but within a managed scope the operator is the
single writer and detects drift. Either full ownership or explicit one-shot
— never the silent middle.

- https://www.keycloak.org/operator/realm-import
- https://github.com/keycloak/keycloak-realm-operator (legacy lineage)

### External Secrets Operator — namespaced config CRDs consumed by a central controller

ESO models per-scope backend configuration as CRDs: namespaced
**`SecretStore`** (tenant-scoped authority) and cluster-scoped
**`ClusterSecretStore`** (admin authority), both with status conditions and
validation. Precedent for the exact shape we need: a config-like object,
expressed as a CRD, referenced/consumed by a central controller, with the
namespaced variant carrying the delegated (narrower) authority.

- https://external-secrets.io/latest/api/secretstore/
- https://external-secrets.io/latest/api/clustersecretstore/

### Capsule — CVE-2025-55205, the label-routing cautionary tale

Capsule routes namespaces into tenants via namespace metadata. CVE-2025-55205
(GHSA, fixed in v0.10.4): a tenant owner could manipulate namespace labels so
that a namespace was captured into / evaluated against another tenant —
cross-tenant boundary bypass, because **routing trusted data the routed party
could write**. Design consequence for v0.18: namespace labels are facts
(GitOps-stamped, orthogonal keys, boolean opt-in style also valid), the maps
that interpret them live in the operator's namespace, and map creation is
admin-only. The tenant never holds the pen that draws its own boundary.

- https://github.com/projectcapsule/capsule/security/advisories (CVE-2025-55205)
- https://nvd.nist.gov/vuln/detail/CVE-2025-55205

### Crossplane — scope/credential config as CRDs

Crossplane's **`ProviderConfig`** is the canonical "which credential/scope
does this resource reconcile under" object — a CRD referenced by managed
resources, not a ConfigMap, despite being pure configuration. Multiple
ProviderConfigs coexist and resources select one; the analogy to per-org
scope maps selecting the delegated credential is direct.

- https://docs.crossplane.io/latest/concepts/providers/#provider-configuration

### Kubernetes RBAC — what `resourceNames` can and cannot delegate

`resourceNames` in a Role restricts get/update/patch/delete to named objects
— on any resource type, CRs included. It **cannot restrict `create`**: at
admission time the name may not exist yet (generateName), so the API server
ignores `resourceNames` for create ([kubernetes#80295], long-standing,
working as intended). Consequence embraced by the design: per-map maintenance
is delegable; minting new maps is not.

- [kubernetes#80295]: https://github.com/kubernetes/kubernetes/issues/80295
- https://kubernetes.io/docs/reference/access-authn-authz/rbac/#referring-to-resources

## What this decided in v0.18

| Decision | Grounding |
| --- | --- |
| `ZitadelScopeMap` = namespaced CRD in the operator namespace | ArgoCD AppProject, ESO SecretStore, Crossplane ProviderConfig; zero CM precedent |
| Namespace labels are facts, never routing authority | Capsule CVE-2025-55205 |
| Map create admin-only, maintenance delegated via `resourceNames` | kubernetes#80295 |
| Single writer within a managed scope, drift detection on | Keycloak lesson |
| Status conditions + Events as the rejection surface | CRD status subresource (unavailable on ConfigMaps) |

Source data: `docs/research/proto-v018-findings.md` (prototype evidence) and
the 2026-07-21 design discussion (INF-422 umbrella).
