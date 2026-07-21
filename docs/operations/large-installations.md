# Large Multi-Tenant Installations

This guide is for the "one platform, many companies" shape: a single Zitadel instance (or a small number of them) serving tens of organizations and hundreds of tenant namespaces, operated by a central platform team with per-tenant teams consuming identity self-service.

## Recommended shape

- **One `iam-owner` operator per Zitadel instance**, in a locked-down operator namespace. Scope maps — not extra operators — carry the multi-tenancy: one map per organization, delegated per-scope credentials doing the actual reconciliation.
- **One `ScopeMap` per organization**, named after the org, maintained by that org's platform contacts via [`resourceNames` RBAC](scope-maps.md#delegating-map-maintenance-to-a-team).
- **Selector rules driven by GitOps-stamped namespace labels** (`tenancy.example.com/company: acme`), so onboarding a namespace is a label in the namespace factory, not a map edit.
- **Project scopes for app namespaces**, org scopes only for namespaces that genuinely manage org-wide state (policies, IdPs, users).

```
Zitadel instance ──── operator (iam-owner, ns zitadel-operator)
  org Acme    ◄─ map acme-corp    ◄─ namespaces labeled company=acme     (delegates: 1 per scope)
  org Beta    ◄─ map beta-inc     ◄─ namespaces labeled company=beta
  org Gamma   ◄─ map gamma-llc    ◄─ namespaces labeled company=gamma
```

Why not operator-per-org? An `org-owner` operator per organization multiplies deployments, credentials, and upgrade surfaces linearly with tenants, for isolation the delegation model already provides at API level (each scope's delegate cannot act outside its org/project). Reserve separate operators for separate *instances* or hard compliance boundaries — see [Multi-operator topologies](multi-operator.md).

## Onboarding a tenant (runbook)

1. **Org exists?** Created out-of-band (IaC) or via an `Organization` CR in a platform namespace.
2. **Map** (admin): create `ScopeMap <org>` with a selector rule for the tenant's label, `organizationId` pinned. Optionally bind the tenant's platform group to the [maintainer Role](scope-maps.md#recipe-acmes-platform-team-maintains-acme-corp).
3. **Namespaces** (namespace factory): stamp the tenancy label; in namespaced RBAC mode also extend `watchNamespaces` + `rbac.namespaces` and upgrade the release.
4. **Tenant applies CRs** — no `organizationId`/`projectId` boilerplate needed; the scope supplies them.

Offboarding is the reverse, in [decommissioning order](scope-maps.md#decommissioning-a-tenant): CRs → rules/map → namespaces.

## Scaling characteristics and knobs

| Dimension | What grows | Notes |
| --- | --- | --- |
| Namespaces | informer memory, resolution work | Resolution is in-memory (no Zitadel calls); selector evaluation is cheap. Use `watchNamespaces` only if you must bound visibility — at hundreds of namespaces cluster mode is simpler and RBAC-mode Role sprawl is worse. |
| Scope maps / rules | resolution work per reconcile | Keep rules coarse (one selector rule per org beats one literal rule per namespace). |
| Scopes (org, project combos) | delegated SAs + Secrets, warm-start time | One machine user + one Secret per scope. Prefer fewer, broader project scopes over one project per namespace unless isolation demands it. |
| Tenant CRs | reconcile throughput | Idempotent reconciles with 5-minute periodic requeue; no drift = no Zitadel write. Watch the operator's Zitadel API error rates as tenant count grows. |

Singleton knobs worth knowing: delegation GC sweeps every 10 minutes; delegate keys rotate every 90 days (dual-key, zero downtime); fail-closed CRs requeue every 10 seconds — a large pool of *permanently* unmapped CRs is therefore noisy, fix or remove them rather than letting them churn.

## Operational guardrails

- **Protect the operator namespace.** It holds the binding credential (IAM_OWNER), every delegation Secret, and the routing surface. Admission-restrict who can create anything there; scope maps are the only object tenants' representatives should touch, via `resourceNames`.
- **Alert from the standard controller-runtime metrics** — see the [metrics reference](metrics.md) for the families and starting alert rules.
- **Watch for `ScopeConflict` Events** on maps — the signal that two orgs' rules claim one namespace. Fail-closed protects you, but the fix (tightening selectors) is on the platform team.
- **Alert on `MapsNotSynced` persisting** beyond startup and on `DelegationFailed` conditions — both indicate the platform side (informers, Zitadel availability, permissions) rather than tenant error.
- **Pin `organizationId` in every map.** At fleet scale, name-only maps make org renames a routing event; ID-pinned maps make them a cosmetic drift Event.
- **Audit with the event log.** `AdminService.ListEvents` filtered by editor separates delegate-authored tenant changes from binding-authored minting — the [actor-proof invariant](../architecture/delegation.md#auditability-the-actor-proof) holds at any scale.
- **CRD chart upgrades are fleet-wide.** All operators on a cluster share CRDs; roll the CRD chart first, then operators, and keep operator versions within one minor of each other.

## Namespace-factory checklist (per tenant namespace)

- tenancy label(s) stamped by GitOps — never writable by the tenant
- (RBAC mode) namespace added to the operator release values
- tenant team RBAC: full CRUD on tenant CR kinds *in their namespaces only*; no access to the operator namespace apart from the optional per-map maintainer Role
