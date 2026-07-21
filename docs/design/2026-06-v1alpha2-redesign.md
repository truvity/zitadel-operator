# Design Record: v1alpha2 Architecture Redesign

**Date:** 2026-06 (drafted May–June 2026)
**Ticket:** INF-363
**Status:** Implemented (shipped incrementally, v0.11.0 – v0.13.0)
**Breaking:** Yes (v1alpha1 → v1alpha2)

> Historical record. The implemented, current-state description lives in
> [docs/architecture/](../architecture/); this document preserves the decisions
> and their rationale as taken. Where reality diverged from the plan, a
> *Deviation* note marks it.

## Design principles

1. **Explicit over implicit** — every resource declares which organization it belongs to (like Terraform's `org_id`)
2. **Hierarchical references** — Organization → Project → App/User, parent references propagating defaults downward
3. **Operator-per-instance** — each deployment bound to exactly one Zitadel instance at startup
4. **Namespace-based isolation** — multiple operators coexist via namespace scoping + K8s RBAC; no custom labels or controller-class
5. **No CRD for connection config** — instance connection belongs to the deployment, not a reconciled resource
6. **Idempotent reconciliation** — no hot loops, conditional status updates, proper drift detection
7. **Generic and reusable** — no Truvity-specific code; works with any Zitadel instance and deployment method
8. **Terraform provider parity** — the operator can fully replace Terraform for in-cluster identity management

## Decision: config file only (no CLI flag wall)

All configuration in a single YAML file; the only CLI argument is the config path.

- One source of truth — no precedence rules between flags/env/file
- Matches the test pattern (`~/.config/zitadel-operator/config.yaml` locally, same format in production)
- Simpler Helm chart — render a ConfigMap, mount it
- Handles growth (lists, nested config) naturally

*Deviation:* a handful of process-level flags exist in practice (`--metrics-bind-address`, `--health-probe-bind-address`, and since v0.18 `--leader-elect`/`--leader-election-id`); everything about the Zitadel connection stayed file-only. The original config schema included `defaultOrganizationId` — removed in v0.18 ([scope-maps record](2026-07-v018-scope-maps.md)).

## Decision: no ZitadelInstance CRD

| Concern | Solved by |
| --- | --- |
| Which instance to connect to | Config file (`domain`) |
| Credential rotation | Volume mount + kubelet secret sync |
| Connection status visibility | Health endpoint + metrics |
| Multiple instances | Multiple operator deployments (one per instance) |
| Runtime reconfiguration | Not needed — restart is fine for instance-level changes |

A CRD would add reconciliation complexity for connection lifecycle (reconnect, retry, health) without benefit when the operator is always 1:1 with an instance. Credentials are mounted files (kubelet sync gives rotation without restart; no Secret-read RBAC needed at startup).

## Decision: namespace + RBAC isolation, not controller-class

| Approach | Enforcement | Complexity |
| --- | --- | --- |
| Controller-class label | Application-level (our code filters) | Custom logic, not RBAC-enforced |
| Namespace + RBAC | API-server-level (K8s enforces) | Zero custom logic, standard K8s |

Each operator watches only its namespaces (`watchNamespaces`) and holds Roles/RoleBindings only there; the API server enforces the boundary. Since two operators never need the same namespace (revisited for different-instance pairs in v0.18 dual-serving), namespace-scoped RBAC provides everything controller-class would.

## Decision: explicit hierarchy with `*Ref` XOR `*Id` references

All CRDs namespaced (v1alpha1 had cluster-scoped Organization/Project); every resource references its **direct parent only** — either a CR reference (`organizationRef`/`projectRef`, optionally cross-namespace) or a raw Zitadel ID (`organizationId`/`projectId`), mutually exclusive. Apps carry no org field at all: the org comes from their project. Rationale: Terraform parity, explicit scoping, and namespaces as RBAC boundaries. The third resolution branch at the time — "neither set ⇒ operator-level `defaultOrganizationId`" — was removed in v0.18 in favor of scope maps.

## Decision: v1alpha1 → v1alpha2 migration is manual

A conversion webhook was rejected: marginal benefit for a small number of deployments against real complexity. Plan: document the mapping, generate v1alpha2 YAML from live v1alpha1 CRs, deploy both CRD versions side by side, switch the operator, remove v1alpha1.

## Testing strategy

Two packages: unit tests (pure logic, CI) and integration tests behind a build tag (full reconcile loop against real Zitadel, local only — credentials cannot live in CI). Connection config from `~/.config/zitadel-operator/config.yaml`, secrets from the system keyring.

*Deviation:* the design specified Kind + `kubernetes-sigs/e2e-framework` for the Kubernetes side; the implementation settled on **envtest** (real API server without a cluster) as faster and sufficient — see [Integration-test architecture](../development/integration-tests.md) for the as-built shape and rationale.

## Hot-loop fix (INF-362)

The 2-second reconcile loop was eliminated by three changes, all kept since: `GenerationChangedPredicate` on every controller (status writes don't retrigger), conditional status updates (write only on change), and full-field drift detection (starting with OIDCApp's mutable fields).

## Implementation phases (as planned)

0. Test harness first — REPL-style development from day 1
1. Fix existing v1alpha1 code under tests (hot-loop, drift) — shipped as a patch
2. v1alpha2 core resources (Organization, Project, OIDCApp, MachineUser)
3. Operator infrastructure (config loader, watchNamespaces, cross-namespace refs)
4. Extended resources to Terraform parity (IdPs, policies, grants, Actions, providers)
5. Error handling, finalizer edge cases, migration script, docs

Shipped across v0.11.0 (17 CRDs), v0.12.0 (+9), v0.13.0 (+17, reaching 42); the catalog and per-release detail are in [CHANGELOG.md](../../CHANGELOG.md).

## Open questions at the time (since resolved)

1. `organizationRef` in addition to `organizationId`? → **Yes, either/or** (implemented).
2. Auto-discover the default org from SA membership? → Moot: default-org semantics were removed entirely in v0.18.
3. Operator in-cluster (pod) vs local process for integration tests? → In-process manager on envtest.
