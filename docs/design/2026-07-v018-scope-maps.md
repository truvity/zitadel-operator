# Design Record: v0.18 — Scope Maps & Internal Delegation

**Date:** 2026-07-21 (design final)
**Tickets:** INF-422 (umbrella), INF-423–428, INF-435
**Status:** Implemented on `feat/v018-scope-maps`; prototyped on `proto/v018-scope-maps` (see [prototype findings](../research/proto-v018-findings.md))
**Breaking:** Yes — config keys removed (see [Breaking changes](#breaking-changes-v017--v018))

> Historical record of the v0.18 design as decided. Current-state documentation:
> [Scope resolution](../architecture/scope-resolution.md), [Internal delegation](../architecture/delegation.md),
> [Dual-serving](../architecture/dual-serving.md), [Status & SSA](../architecture/status-and-ssa.md),
> [Binding levels](../install/binding-levels.md). Migration: [MIGRATION-0.18.md](../MIGRATION-0.18.md).

v0.18 replaces the two implicit routing mechanisms (`defaultOrganizationId`, `projectScopeLabel`) with an explicit, delegable, fail-closed model: **scope maps** route tenant namespaces to Zitadel scopes, and the operator reconciles each namespace with an internally minted, scope-limited credential (**internal delegation**). Comparative research behind the routing surface: [operator config patterns](../research/operator-config-patterns.md).

## Binding levels

One operator = one instance + one credential, as before; new in v0.18 the config asserts what that credential is (`binding: iam-owner | org-owner`, required), verified at startup against `AuthService.ListMyMemberships` — mismatch crashes before any reconcile. Degradation under `org-owner` is uniform, not per-feature: one validation matrix (binding level × scope shape); instance-level paths get `NotSupportedAtBindingLevel`, foreign-org maps are rejected with an Event.

## ScopeMap CRD

A **namespaced CRD** evaluated only in the operator's own namespace — the map's location is part of its authority. Deliberately *not* a ConfigMap: the survey (ArgoCD, Keycloak, ESO, Capsule, Crossplane) found zero precedent for ConfigMap-based semantic routing, and a CRD buys OpenAPI validation, a status subresource, printer columns, and per-object RBAC.

**Delegation model:** admins create a company's map; that company's platform team maintains it via K8s RBAC `resourceNames`. `resourceNames` cannot gate `create` ([kubernetes#80295](https://github.com/kubernetes/kubernetes/issues/80295)) — so map creation stays admin-only, which is exactly the intended trust split.

Field semantics as designed (implemented as specified):

- `spec.instance` **required** — fail-closed `InstanceMismatch` on foreign maps; the guard against a map landing in the wrong operator's namespace.
- `spec.organization` + optional authoritative `spec.organizationId`; name disagreement is `OrganizationNameDrift` (Event, non-fatal); name-only maps resolve via the map controller into `status.resolvedOrganizationId`, keeping the resolver free of Zitadel calls.
- `rules[]` ordered, first match top-down, across all maps name-sorted; each rule `namespaceSelector` XOR `namespaces[]`, optional `project` (+ `projectId` on literal rules only); no project = org scope.
- Cross-map match = `ScopeConflict`, fail-closed, Warning Events on both maps.

**Namespace labels are facts, not routing config.** Labels are GitOps-stamped orthogonal keys; routing authority lives in the maps. That inversion is the Capsule CVE-2025-55205 lesson: the tenant never holds the pen that draws its own boundary.

## Resolution taxonomy

Exactly one outcome per namespace: resolved scope · passthrough (zero maps) · `MapsNotSynced` (transient) · `NoMatchingRule` · `ScopeConflict` · `InstanceMismatch` · `ScopeMapNotReady`. Distinguishing `MapsNotSynced` from `NoMatchingRule` is mandatory — collapsing them turns every restart into a rejection storm.

**Zero maps = passthrough (rollout gate)** — a prototype-forced correction: strict fail-closed from the first moment would make installing the CRD a flag-day. Strictness begins with the first map. GA enablement is this gate plus `operatorNamespace` (unset disables scope maps); no extra config flag was added.

**Scope-defaulted project** — a project-scope rule creates/pins the project; tenant CRs need no `projectRef`/`projectId`, and the resolved project is recorded in `status.projectId` so deletion works after maps are gone. Generalized from OIDCApp to all project-level kinds during implementation.

## Internal delegation

Per resolved scope the operator mints and caches a scope-limited SA and reconciles tenant CRs **with the delegated key**; the binding credential only mints/rotates/revokes.

- **Explicit create + explicit grant, never ORG_PROJECT_CREATOR** ([zitadel#10561](https://github.com/zitadel/zitadel/issues/10561): the creator role does not own what it creates). Org scope ⇒ `AddOrgMember(ORG_OWNER)`; project scope ⇒ project created first (binding) + `AddProjectMember(PROJECT_OWNER)` — PROJECT_OWNER proven sufficient for full app CRUD via the v2 API.
- **Secrets-backed cache** — labeled Secret per scope, persisted **before** the in-memory cache (crash cannot lose a key); warm restart by label list; lazy validity re-check (out-of-band deletion ⇒ re-mint).
- **Eager revoke + orphan GC**; **90-day dual-key rotation** with grace overlap.
- **Deletion fallback** (prototype-forced): during deletion, resolution/delegation failure falls back to the binding client — finalizers never deadlock; a delegate must outlive the tenant CRs it serves.

Auditability: `AdminService.ListEvents` by editor. The prototype's actor proof — all `project.application.*` events authored by the delegate, zero by the binding; `project.added` by the binding — is the trust boundary, assertable in production the same way.

## MachineUser extension

Existing kind extended, no new CRD: `spec.roles` (grants within the resolved scope — narrow allowed, widen never), `spec.key.rotateAfter` dual-key rotation (+ `rotationGrace`, added beyond the design text for operability), output Secret upgraded to a connection bundle. Fully backward compatible.

## Dual-serving and CR-level `spec.instance`

Optional pin on all tenant kinds: pinned-to-me ⇒ reconcile; pinned-foreign ⇒ completely untouched (no finalizer/status/API calls, clean handover on repin); unset while dual-served ⇒ both operators fail closed with `AmbiguousInstance` via SSA with distinct field managers.

## SSA status discipline (hard prerequisite)

The prototype demonstrated that read-modify-write `Status().Update()` **wipes conditions** — a model that cannot support two writers at all. Decision: all status writes move to SSA (field manager `zitadel-operator/<domain>`), landed as the first commit of the batch, before any feature work; the dual-serving smoke is gated on it. Conditions become `listType=map` keyed by `type`.

## Leader election

Always on, unconditionally — even `replicas: 1` runs two pods during a rolling update; without a lease both reconcile (double-minted delegates, racing Secret writes). Lease ID derives from the Helm fullname (the previous hardcoded ID would make two deployments in one namespace fight over one lease).

## Breaking changes (v0.17 → v0.18)

Deliberately breaking (0.x semver; gitops is the only consumer):

- **`defaultOrganizationId` removed** — no default scope exists; implicit org fallback is exactly the ambient authority this redesign removes.
- **`projectScopeLabel` removed** — label-value-as-project routing superseded by scope maps (Capsule lesson).
- Fail-fast at startup when a removed key is present, pointing at the migration guide.
- `watchNamespaces` survives as the coarse informer filter only.

## Single-writer stance

Within a managed scope the operator is the single writer with drift detection — the Keycloak lesson (half-ownership silently diverges; either full ownership or explicit one-shot, never the silent middle). Out-of-cluster provisioning (instance, orgs, corporate IdPs) stays in Pulumi; the boundary is the org/project line, not shared writes on one resource.

## Rejected: templating

Per-namespace templating of scope maps (generate an org/project per namespace pattern) was rejected: convention-based separation suffices, per-namespace Zitadel isolation is not required, and a template engine inside the routing surface trades auditability for convenience.
