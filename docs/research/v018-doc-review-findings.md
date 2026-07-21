# v0.18 Documentation Pass — Inconsistencies & Improvement Ideas

**Date:** 2026-07-21
**Context:** collected while writing the enterprise documentation set on top of
`feat/v018-scope-maps` (PR #38). For review **before** the v0.18 release —
items marked *(pre-release)* get cheaper the earlier they land.

## A. Real inconsistencies found (docs already fixed where possible)

1. **`just test-integration` cannot pass as written.** The recipe sets
   `-timeout=120s`, but PR #38 reports the suite at ~490s. Everyone apparently
   runs `go test -tags=integration ...` directly. Fix the Justfile timeout
   (e.g. `-timeout=20m`) — one-line change, deliberately not done in the docs
   PR because the task was docs-only.
2. **S-161 contradicts the implemented singleton semantics.**
   `TEST-SCENARIOS.md` S-161 ("duplicate creation — last-writer-wins, both
   Ready=true") describes pre-0.13 behavior; since 0.13 the earliest CR wins
   and duplicates get `DuplicateSingleton` (and v0.18 made the tie-break
   deterministic). The scenario text (and possibly the test) should be
   updated to assert the current contract.
3. **README/ROADMAP counted 42 CRDs; there are 43** (ZitadelScopeMap). Fixed
   in the rewritten docs.
4. **DESIGN.md testing section described Kind + `e2e-framework`; the
   implementation uses envtest.** Long-standing drift, now recorded as a
   *Deviation* note in the design archive and documented as-built in
   `docs/development/integration-tests.md`.
5. **`docs/TEST-SCENARIOS.md` began with a stray `b#`** (typo in the H1) and
   S-005 still described the removed `defaultOrganizationId` behavior. Both
   fixed (S-005 marked RETIRED, superseded by S-225).
6. **Stale chart version examples.** The old README pinned
   `--version 0.11.0` in install commands while the latest release is 0.16.0.
   New docs avoid pinning versions in prose entirely.

## B. API / naming observations *(pre-release — API churn is cheapest now)*

1. **`ZitadelScopeMap` is the only CRD with a `Zitadel` prefix** (`OIDCApp`,
   not `ZitadelOIDCApp`). Defensible (it configures the operator rather than
   a Zitadel resource), but if the prefix is meant to signal "operator
   surface", consider making that a stated convention — or rename to
   `ScopeMap` before GA while the CRD is days old.
2. **`spec.organization` is required even when `organizationId` is set**, yet
   the ID is authoritative and the name is only drift-checked. A
   required-but-non-authoritative field is odd ergonomics; consider making
   `organization` optional when `organizationId` is present.
3. **`ScopeMapRule.projectId` requires a literal `project` name** — same
   shape as (2): the pinned ID wins but a name must still be supplied. If the
   intent is "names are for humans, IDs for machines", saying so in the field
   docs (or relaxing the requirement) would help.
4. **`spec.instance` pins are raw domain strings.** Repointing an operator to
   a new domain (rare, but instance migrations happen) silently orphans every
   pinned CR and changes the operator's SSA field-manager identity
   (`zitadel-operator/<domain>`), leaving stale managed-field entries that
   dual-serving detection will read as a foreign operator. Worth a follow-up:
   documented rename procedure, or an instance-alias layer.
5. **Dual status idiom** — every CRD carries both `status.ready` (bool,
   printer column) and a `Ready` condition. Harmless but a permanent
   two-places-to-update invariant; a kubebuilder printer column on the
   condition could retire the bool eventually (breaking, so: note only).

## C. Config / chart ergonomics

1. **Silent scope-map disable.** `operatorNamespace` unset **and**
   `POD_NAMESPACE` unset ⇒ scope maps are disabled and the operator runs
   passthrough — silently permissive for a security-relevant feature. The
   Helm chart always sets the key, so this bites raw-manifest and local-run
   users precisely when misconfigured. Suggest: log loudly at startup, or
   require an explicit `scopeMaps: disabled` opt-out. *(pre-release)*
2. **No startup validation that `operatorNamespace` ∈ `watchNamespaces`.**
   When `watchNamespaces` is set but omits the operator namespace, map
   informers cannot sync and every scoped CR sits at `MapsNotSynced`
   (surfaced only as a condition, cause non-obvious). The invariant is
   documented in three places; a fail-fast check in `config.Load`/`main` (or
   auto-appending the namespace) would delete a whole troubleshooting class.
   *(pre-release)*
3. **`keyFile` is not validated at config load** — an empty value surfaces as
   `open : no such file or directory` from `os.ReadFile`. Trivial fail-fast
   candidate alongside the `domain`/`binding` checks.
4. **`leaderElection.enabled: false` is a chart-exposed footgun** given v0.18
   semantics (delegate minting under split-brain). Consider dropping the
   toggle or gating it behind a `developmentMode` style flag.
5. **`config.port` is a string** (`"443"`); a bare `port: 443` in values
   renders fine but a raw config file with an int fails YAML unmarshal into
   `string`. Either accept both (custom unmarshal) or document string-only —
   currently undocumented.
6. **`externalDomain` (split-horizon) has no user-facing docs beyond one
   table row** — it changes JWT audiences and headers, deserving a short
   how-to (when Zitadel sits behind an internal LB with a different external
   name).

## D. Behavior / conditions

1. **Steady-state fail-closed requeues every 10s** (`NoMatchingRule`,
   `ScopeConflict`, `InstanceMismatch`). One namespace with a dozen orphaned
   CRs reconciles ~6×12 times/minute forever. Map/namespace changes already
   trigger event-driven reconciles for scope maps themselves; tenant CRs
   could back off to the 5-minute periodic interval once the reject is
   confirmed steady-state.
2. **`DelegationFailed` is surfaced as a condition + returned error** — the
   error path re-queues with exponential backoff (good) but also counts as a
   controller error in metrics for what is often a config problem
   (`org-owner` binding cannot grant). Consider classifying permission-shaped
   delegation failures as fail-closed conditions rather than errors.
3. **`OrganizationNameDrift` and `ForeignOrganization` exist as Events only**
   — Events expire (~1h). For maps that stay drifted/rejected long-term, a
   condition on the map would make the state visible in GitOps dashboards.
   (`ForeignOrganization` does pair with `Ready=False`, so this is mostly
   about the drift case.)
4. **Delegation Secrets are hash-named with no human-readable scope hint**
   (`zitadel-delegation-3f9a…`). `scope.json` inside the Secret has the
   answer, but a `zitadel.truvity.io/scope-org` +
   `…/scope-project` annotation would make `kubectl get secrets -l
   zitadel.truvity.io/delegation` self-describing for operators on call.
5. **Binding verification requires ORG_OWNER in exactly one org** for
   `org-owner` bindings. Legitimate setups (one SA administering two sibling
   orgs from two operator deployments) are impossible without two SAs.
   Fine as a v0.18 constraint — worth an explicit statement in release notes,
   plus a possible future `boundOrganizationId` selector in config.

## E. Documentation follow-ups (not done in this pass)

1. **Examples directory** — DESIGN referenced an `examples/` folder that
   never materialized; a curated `examples/` (per-cluster onboarding bundle,
   scope-mapped tenant, dual-served namespace) would pair well with S-200…204
   when those are implemented.
2. **Metrics reference** — the operator exposes controller-runtime metrics on
   `:8080`, but no doc lists which metrics are useful for the alerts
   suggested in `docs/operations/large-installations.md`.
3. **CRD doc comments are the API reference now** — a handful of spec fields
   (`OIDCApp` URI semantics, policy field units) have terse comments that
   read poorly in the generated `docs/reference/api.md`. Worth a sweep of
   `api/v1alpha2/*_types.go` doc comments; the reference regenerates for
   free.
4. **`docs/research/` vs `docs/design/`** — research notes stayed in place
   (they are already dated evidence records, linked from the design index).
   If Oleg prefers a single archive tree, moving them under `docs/design/`
   is a rename-only change.
