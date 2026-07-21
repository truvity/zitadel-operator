# Scope Resolution

Scope resolution answers one question per tenant CR: **which Zitadel scope — `(organization)` or `(organization, project)` — does this CR's namespace belong to, and with which credential is it reconciled?** The answer is computed from `ScopeMap` objects and is deliberately fail-closed: a namespace that maps nowhere is rejected, never defaulted.

## ScopeMap

A namespaced CRD, evaluated **only in the operator's namespace** (`operatorNamespace`) — the map's location is part of its authority. Field reference: [API reference — ScopeMap](../reference/api.md#scopemap).

```yaml
apiVersion: zitadel.truvity.io/v1alpha2
kind: ScopeMap
metadata:
  name: acme-corp
  namespace: zitadel-operator            # must be the operator's namespace
spec:
  instance: prod-internal                # REQUIRED — the operator's instance identity (instanceAlias, default domain)
  organization: Acme Corp
  organizationId: "325908855630427886"   # optional; authoritative when set
  rules:
    - name: acme-product-namespaces
      namespaceSelector:                  # selector rule …
        matchLabels:
          tenancy.example.com/company: acme
      project: acme-platform              # → project scope (org, project)
    - name: acme-ops
      namespaces: [acme-ops, acme-audit]  # … XOR literal rule
      # no project → org scope
```

Semantics:

- **`spec.instance` (required)** — every map asserts which instance it belongs to. A mismatch with the operator's `domain` marks the map `InstanceMatch=False / InstanceMismatch` and fail-closes every namespace it would serve. This guards against a map landing in the wrong operator's namespace: two operators' routing surfaces cannot cross-contaminate.
- **`spec.organization` + optional `spec.organizationId`** — the ID is authoritative when present; a disagreeing name is reported as an `OrganizationNameDrift` Event, not an error. Without an ID, the scope-map controller resolves the name and records `status.resolvedOrganizationId` (the resolver itself never calls Zitadel). A matched map without a resolved org yet is `ScopeMapNotReady` — transient.
- **`rules[]`** — ordered; each rule is `namespaceSelector` XOR `namespaces[]`, plus optional `project`/`projectId` in any combination (the ID is authoritative when set; a bare name is resolved/created on first use). No project = org scope. **First match top-down wins**, evaluated across all maps in the operator namespace with maps sorted by name for determinism.
- A namespace matching rules in **two different maps** is a conflict: fail-closed with `ScopeConflict` and Warning Events on **both** maps.

### Labels are facts, not routing config

Selector rules consume namespace labels that are GitOps-stamped facts (`company: acme`, `tier: prod`) — tenant teams never write their own labels, and routing authority lives exclusively in the maps (operator namespace, admin-controlled). This inversion is the lesson of Capsule CVE-2025-55205, where routing trusted labels the routed tenant could write; the full comparative survey (ArgoCD, Keycloak, ESO, Capsule, Crossplane) that shaped this surface is in [research: operator config patterns](../research/operator-config-patterns.md).

## Resolution outcomes

For a tenant CR's namespace, resolution returns exactly one of:

| Outcome | Meaning | Behavior |
| --- | --- | --- |
| resolved scope | exactly one rule matched | reconcile with the [delegated client](delegation.md); `ScopeResolved=True` |
| passthrough | **zero maps exist** | legacy path: reconcile with the binding credential directly |
| `MapsNotSynced` | informers not synced yet (restart) | transient — short requeue (2s), never a rejection |
| `NoMatchingRule` | maps exist, none matched | fail-closed: `ScopeResolved=False`, requeue 10s |
| `ScopeConflict` | rules in ≥2 maps matched | fail-closed + Warning Events on both maps |
| `InstanceMismatch` | only matching map asserts a foreign instance | fail-closed |
| `ScopeMapNotReady` | matched map's org not resolved yet | transient — short requeue |
| `DelegationFailed` | scope resolved but the delegate could not be minted | fail-closed, surfaced as a reconcile error |

Distinguishing `MapsNotSynced` from `NoMatchingRule` is load-bearing: collapsing them would turn every operator restart into a spurious rejection storm.

**Zero maps = passthrough** is the rollout gate: installing the CRD is not a flag day. With no `ScopeMap` objects the operator behaves exactly as pre-v0.18 (binding client, explicit org/project on CRs). Strictness begins the moment the first map appears — from then on, unmapped namespaces are rejected. The same applies when `operatorNamespace` is empty: scope maps are disabled entirely.

## Scope-defaulted organization and project

A resolved scope supplies parents that tenant CRs omit:

- org-scoped CRs (MachineUser, policies, …) without `organizationRef`/`organizationId` inherit the **scope's organization**;
- project-level CRs (OIDCApp, APIApp, ApplicationKey, …) in a project-scope namespace without `projectRef`/`projectId` inherit the **scope's project** (created by the operator if the rule names one that does not exist yet).

Resolved IDs are recorded in CR status (`status.organizationId`, `status.projectId`) so deletion still works after maps are removed. Explicit references remain allowed but are validated against the scope: a CR pinning an organization outside its namespace's scope fails closed (`organization … is outside the namespace's resolved scope`).

## Interaction with binding levels

Scope shape × [binding level](../install/binding-levels.md) forms one validation matrix:

| | `iam-owner` | `org-owner` |
| --- | --- | --- |
| Map for any org / the bound org | honored | honored (bound org only) |
| Map for a foreign org | honored | rejected: `NotSupportedAtBindingLevel` + `ForeignOrganization` Event |
| Instance-level CRs | honored | `Ready=False / NotSupportedAtBindingLevel` |

## Deletion fallback

Strict fail-closed would deadlock finalizers when maps or delegates disappear before the tenant CRs they served. Therefore, **during deletion only**, resolution or delegation failure falls back to the binding client so cleanup always completes. The intended de-provisioning order remains: tenant CRs first, then rules, then maps — see [scope-map administration](../operations/scope-maps.md#decommissioning-a-tenant).

## Where the pieces live

| Concern | Code |
| --- | --- |
| Rule evaluation, error taxonomy | `internal/scopemap/resolver.go` |
| Map validation, org resolution, Events | `internal/controller/scopemap_controller.go` |
| Per-CR resolution + delegated client injection (`tenantPreamble`) | `internal/controller/scope_resolution.go` |
| Delegate mint/rotate/revoke | `internal/delegation/` ([architecture](delegation.md)) |

Design rationale and the decision history: [v0.18 design record](../design/2026-07-v018-scope-maps.md).
