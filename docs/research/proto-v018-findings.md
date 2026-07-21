# v0.18 "Scope Maps" Prototype — Findings

Branch: `proto/v018-scope-maps` (prototype; not for merge as-is).
Test target: real Zitadel (`zitadel-operator-tests-pylwi3.eu1.zitadel.cloud`),
org "Truvity Testing" (fallback: config default org), all resources prefixed
`proto018-`. All evidence below is from tests that ran green on 2026-07-21:

```
go test -tags=integration ./tests/integration/ -run 'TestScopeMap|TestOIDCApp'   # 12/12 PASS
go test ./...                                                                    # unit PASS
golangci-lint run ./...                                                          # 0 issues
```

## Goal 1 — ScopeMap CRD + resolver + OIDCApp wiring: WORKS

Implemented:

- `api/v1alpha2/scopemap_types.go` — namespaced `ScopeMap`
  (instance, organization + optional organizationId, rules[] with
  namespaceSelector XOR namespaces[], optional project/projectId).
- `internal/scopemap/resolver.go` — first-match top-down per map, evaluated
  across all maps in the operator namespace (maps sorted by name for
  determinism); typed error taxonomy: `ErrMapsNotSynced`, `NoMatchError`,
  `ConflictError`, `InstanceMismatchError`, `MapNotReadyError`.
- `internal/controller/scopemap_controller.go` — validates instance
  match (InstanceMatch condition), rule invariants, resolves org by
  name when no ID (`status.resolvedOrganizationId`), emits
  `OrganizationNameDrift` Event when spec ID and name disagree.
- OIDCApp reconciler wired through `resolveScopeAndClient` +
  `ScopeResolved` condition; reasons distinguish `MapsNotSynced` (2s requeue)
  from `NoMatchingRule` / `ScopeConflict` / `InstanceMismatch` /
  `ScopeMapNotReady` (steady-state fail-closed, 10s requeue).

Evidence (integration, real Zitadel + envtest):

| Test | Proves |
|---|---|
| `TestScopeMap_SelectorRuleMatch` | selector rule → delegated project scope, app Ready, `ScopeResolved=True`, delegation Secret consistent with status |
| `TestScopeMap_LiteralMatch_DelegatedActorProof` | literal rule match (+ goal 2 proof below) |
| `TestScopeMap_NoMatch_FailClosed` | maps exist, no rule matches → `ScopeResolved=False/NoMatchingRule`, no finalizer-side app creation, never Ready |
| `TestScopeMap_CrossMapConflict` | ns matched by rules in two maps → fail-closed `ScopeConflict`, Warning Events present on BOTH maps |
| `TestScopeMap_InstanceMismatch` | map with foreign `spec.instance` → map `InstanceMatch=False/InstanceMismatch` + Ready=false, OIDCApp fail-closed `InstanceMismatch` |
| unit `TestResolve_MapsNotSynced` / `TestResolve_LiteralMatch_FirstMatchTopDown` / `TestValidateRule` / `TestScopeHash_StableAndDistinct` | sync-vs-no-match distinction, top-down ordering, rule invariants, hash identity |

### Design corrections forced by the prototype

1. **Rollout gate: zero maps = passthrough.** With strict "no rule =
   fail-closed", installing the CRD is a flag-day for every namespace the
   operator serves. Prototype behavior: no `ScopeMap` objects in the
   operator namespace → legacy binding-client path (all pre-v0.18 tests pass
   unchanged with the resolver wired in). v0.18 needs an explicit
   enablement story (config flag or this gate) — decide before GA.
2. **Scope-defaulted project.** A project-scope rule creates/pins a project;
   an OIDCApp then needs no `projectRef`/`projectId` — the resolved scope
   supplies it (`resolveAppProject`). This falls out naturally and is a
   better UX than requiring every tenant CR to name its project; recorded in
   `status.projectId` so deletion still works after maps are removed.
3. **Condition wiping by full-object Update.** `ensureFinalizer`'s Update
   refreshes the object from the server and silently drops in-memory
   condition edits (`ScopeResolved` vanished; found via failing test).
   Prototype re-applies the condition post-Update. Real fix for v0.18:
   status writes must move to SSA or a single end-of-reconcile status commit.
4. **Deletion needs a fallback.** If maps/delegates are gone while a CR still
   has a finalizer, strict fail-closed deadlocks deletion. Prototype: during
   deletion, resolution/delegation failure falls back to the binding client.
   The delegate must outlive the tenant CRs it serves — sequencing matters
   for de-provisioning (test cleanup hit exactly this: deleting the delegate
   user before app deletion wedged the finalizer).
5. **Org name→ID resolution belongs in the map controller** (writes
   `status.resolvedOrganizationId`), keeping the resolver free of Zitadel
   calls. A matched map without a resolved org is `ScopeMapNotReady`
   (transient), not `NoMatchingRule`.

## Goal 2 — Delegated-SA mint + ACTOR PROOF: DEMONSTRATED

`internal/delegation/manager.go`: per scope (hash of canonical JSON
`{instance, organizationId, project, projectId}`, sha256[:10]):

- explicit `AddMachineUser` (`proto018-delegate-<hash>`) — never relies on
  ORG_PROJECT_CREATOR self-grant;
- explicit membership grant: org-scope → `AddOrgMember(ORG_OWNER)`,
  project-scope → project created if missing (binding credential) +
  `AddProjectMember(PROJECT_OWNER)`;
- `AddMachineKey` → key JSON persisted in Secret
  `zitadel-delegation-<hash>` (label `zitadel.truvity.io/delegation: "true"`)
  in the operator namespace **before** caching (crash cannot lose the key);
- restart warm path: `Ensure` reads the Secret before minting
  (+ `WarmFromSecrets` bulk loader);
- separate `zitadel.Client` per scope, built from the delegated key.

**Actor proof** (`TestScopeMap_LiteralMatch_DelegatedActorProof`), via
`AdminService.ListEvents` on the project aggregate of a real reconcile:

```
event project.application.added             editor=382776312986605347  (delegate)
event project.application.config.oidc.added editor=382776312986605347  (delegate)
ACTOR PROOF: 2 application events all authored by delegate 382776312986605347
             (binding 376735711648289607 authored project.added: true)
```

The test asserts: delegate userId ≠ binding userId; **all** `*application*`
events on the aggregate authored by the delegate; **zero** authored by the
binding credential. The `project.added` event is authored by the binding
credential — expected and correct: minting (project creation + grants) is the
binding's job; tenant-resource reconciliation is the delegate's. That is the
trust boundary.

PROJECT_OWNER was sufficient for the delegated client to list/create/update/
delete applications via the v2 Application API; no extra org role needed.

## Goal 3 — SDK surface (zitadel-go v3.29.2): ALL PRESENT, NO RAW HTTP NEEDED

(a) **Machine-user key add/list/remove (dual-key rotation)** — YES, twice over:
  - v2: `pkg/client/zitadel/user/v2` `UserService.AddKey` (optional
    `PublicKey` for BYO-key; response `KeyContent` = full key JSON),
    `ListKeys` (with `KeysSearchFilter_UserIdFilter` +
    `filter/v2.IDFilter`), `RemoveKey`.
    Proven live by `TestScopeMap_SDKSurface_DualKeyRotation`:
    add 2 keys → list shows 2 → remove old → list shows 1.
  - v1: `management.AddMachineKey/ListMachineKeys/RemoveMachineKey`
    (deprecated; the operator already uses `AddMachineKey`, whose
    `KeyDetails` is the full key JSON).

(b) **Authenticated user's own memberships (startup `binding:` check)** — YES:
  `pkg/client/zitadel/auth` `AuthService.ListMyMemberships` (also
  `ListMyZitadelPermissions`). The SDK client exposes it (`AuthService()`);
  the operator wrapper now surfaces it as `Client.Auth()` (trivial addition,
  `internal/zitadel/client.go`). Proven live by
  `TestScopeMap_SDKSurface_OwnMemberships` — binding reported
  `roles=[IAM_OWNER] iam=true`. `Membership` carries the oneof
  Iam/OrgId/ProjectId/ProjectGrantId needed to distinguish
  `iam-owner` from `org-owner`.

(c) **Org/project membership grant APIs** — YES (used by the delegation
  manager and already by OrgMember/ProjectMember reconcilers):
  `management.AddOrgMember`, `management.AddProjectMember`
  (v1 Management, deprecated but functional). v2 equivalents exist under
  `internal_permission`/`authorization` services — migration is a separate
  chore, not a blocker.

Bonus for the actor proof: `admin.AdminService.ListEvents`
(`EditorUserId`/`AggregateId`/`AggregateTypes` filters; `Event.Editor.UserId`)
— exactly what a production audit check needs.

Gaps: none for v0.18's needs. Caveats: the v1 Management/Auth/Admin services
are deprecated upstream; v2 `AddKey` is the forward path for rotation, and the
event API remains v1-only (`admin.ListEvents`).

## Goal 4 — Dual-serving smoke: PARTIAL

Implemented + proven: optional `spec.instance` on OIDCApp with strict owner
filtering — a CR pinned to a foreign instance is left completely untouched
(no finalizer, no status writes, no Zitadel calls); repinning to this
operator's domain hands the CR over and it reconciles through the delegated
scope. `TestScopeMap_InstancePin_ForeignInstanceIgnored` (single operator
process).

NOT run: the two-process experiment (unset `spec.instance` → both operators
set `AmbiguousInstance` via SSA with distinct field managers). Analysis from
what the prototype did hit:

- Today's controllers use read-modify-write `Status().Update()` plus
  full-object `Update()` for finalizers. Goal-1 finding #3 (condition wipe)
  is direct evidence this model cannot support two writers: they would
  overwrite each other's conditions wholesale. **SSA per-condition-owner is a
  hard prerequisite** for the AmbiguousInstance design; `Status().Update`
  cannot express "my conditions only".
- Both operators would also race the shared finalizer and the credential
  Secret. The unset-instance case therefore needs "neither reconciles
  externally, both mark ambiguity via SSA" — the external-action part is
  already satisfied by the fail-closed structure of the scope resolver.
- Recommendation: implement in v0.18 only after moving OIDCApp status writes
  to SSA (`client.Apply` with field manager `zitadel-operator/<domain>`);
  re-run the two-manager envtest smoke then.

## Files

- `api/v1alpha2/scopemap_types.go` (+ generated deepcopy/CRD/chart copy)
- `internal/scopemap/resolver.go`, `internal/scopemap/resolver_test.go`
- `internal/delegation/manager.go`
- `internal/controller/scopemap_controller.go`
- `internal/controller/scope_resolution.go`
- `internal/controller/oidcapp_controller.go` (wired), `internal/config/config.go`
  (`operatorNamespace` gate), `cmd/operator/main.go` (wiring)
- `internal/zitadel/client.go` (`Auth()` accessor)
- `tests/integration/scopemap_test.go`, `tests/integration/main_test.go` (harness wiring)

---

## v0.18 Implementation Addendum (feat/v018-scope-maps, 2026-07-21)

The batch implementing this design landed on `feat/v018-scope-maps`
(INF-422). Deltas vs. the prototype:

- **SSA discipline implemented for real** (finding #3): every controller's
  status writes go through `applyStatus` (SSA, field manager
  `zitadel-operator/<domain>`); conditions are `listType=map`. The two-writer
  AmbiguousInstance smoke (S-241) passes on top of it.
- **Rollout gate kept** (finding #1): zero maps = passthrough. GA enablement
  is the gate itself plus `operatorNamespace` (unset disables scope maps
  entirely); no extra config flag was added.
- **Scope-defaulted project** (finding #2) generalized from OIDCApp to all
  project-level tenant kinds via `resolveScopedProjectId`.
- **Deletion fallback** (finding #4) kept as designed; delegates outlive
  their tenant CRs, and sweeps run only on map events + periodic GC.
- Delegate usernames use prefix `zitadel-operator-delegate-` in production;
  the integration harness overrides with `v018-delegate-`.

### INF-400 verdict: OPERATOR-SIDE BUG, ROOT-CAUSED AND FIXED

The targeted repro (`TestINF400_URIListUpdate_NoStaleState`, S-261) reproduced
the staleness immediately: the operator detected URI drift and called v2
`UpdateApplication`, but every call failed with
`FailedPrecondition: No changes (COMMAND-2m8vx)` — and the reconciler retried
forever while the server kept the OLD URI list.

Root cause: the update request always included `Name: cr.DisplayName()`.
Zitadel's v2 UpdateApplication treats a *set* name as a name-change command;
an unchanged name fails the whole request with "No changes" BEFORE the OIDC
configuration update is applied. The API contract states "If not set, the
name will not be changed" — so the fix is to include `Name` only when it
actually drifted (`internal/controller/oidcapp_controller.go`,
`updateOIDCAppIfNeeded`). With the fix, S-261's append/replace/shrink
mutations all converge. APIApp/SAMLApp were audited: OIDCApp was the only
caller of v2 `UpdateApplication` with this pattern.
