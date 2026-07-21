# Integration-Test Architecture

The integration suite is the operator's primary correctness gate: full reconcile loops against a **real Zitadel instance**, with the Kubernetes side simulated by **envtest** (a real kube-apiserver + etcd, no kubelet). It is deliberately not in CI — it needs credentials for a live test instance and takes ~8 minutes; contributors run it locally.

## The split: envtest vs live Zitadel

| Layer | Real or simulated | Provided by |
| --- | --- | --- |
| Kubernetes API (CRs, Secrets, SSA, finalizers, managedFields) | Real API server + etcd | `sigs.k8s.io/controller-runtime/pkg/envtest` (binaries via `setup-envtest`, auto-detected by `TestMain`) |
| Controllers | Real — all reconcilers registered on one shared manager, in-process | `tests/integration/main_test.go` |
| Zitadel | **Real** — a dedicated test instance | `~/.config/zitadel-operator/config.yaml` + JWT key from the system keyring |

This is the sweet spot deliberately chosen over Kind: envtest boots in seconds and gives the full API-server semantics the operator depends on (SSA field managers, subresources, finalizers), while the Zitadel side — the part where mocks would prove nothing — is never simulated. What envtest does *not* provide (pods, image pulls, leases-under-crash) is not what these tests assert; the leader-election handoff scenario runs two managers in-process instead.

Unit tests (`go test ./...`, no build tag, CI) cover pure logic: config validation, resolver rule evaluation, drift/normalization helpers.

## One-time local setup

```bash
# 1. Store the test instance's JWT key in the system keyring (never on disk)
secret-tool store --label='zitadel-operator jwt-key' \
  service zitadel-operator username jwt-key < /path/to/key.json
rm /path/to/key.json

# 2. Config (same schema as production operator config)
mkdir -p ~/.config/zitadel-operator
cat > ~/.config/zitadel-operator/config.yaml << 'EOF'
domain: <your-test-instance>.eu1.zitadel.cloud
port: "443"
insecure: false
binding: iam-owner
EOF
```

The suite verifies the `binding` assertion at startup exactly as production does, and resolves the credential's own org for tests that need a pre-existing organization.

## Running

```bash
just test-integration                                            # whole suite
go test -tags=integration -v -run TestScopeMap ./tests/integration/...   # one group
go test -tags=integration -v -run 'TestOIDCApp/S-030' ./tests/integration/...
```

Everything is behind the `//go:build integration` tag; without the tag the package does not compile into normal builds.

## The S-numbering system

[docs/TEST-SCENARIOS.md](../TEST-SCENARIOS.md) is the scenario catalog: every end-to-end behavior has a stable ID (`S-XXX`), grouped in numbered sections (S-0xx core resources … S-2xx v0.18). The catalog is the contract; test functions cite their scenario (`// S-227: Scope map — no-match fail-closed`), and reviews check the catalog, not the test names. IDs are never reused.

Ranges in use: S-001–S-19x per-resource lifecycle and error handling; S-200–S-204 planned GitOps composition scenarios; S-210–S-270 the v0.18 batch (SSA, binding levels, scope maps, delegation, dual-serving, leader election).

## Adding a scenario

1. **Allocate an S-number** in `TEST-SCENARIOS.md` — next free ID in the matching section (new section for a new area). Describe: steps, expectations (`**Expect:**`), and the test function name.
2. **Pick the file**: `tests/integration/<resource>_test.go`, or a `v018_*`-style file for cross-cutting behavior.
3. **Write the test** against the shared harness globals — `k8sClient` (envtest), `zitadelClient` (direct API access for out-of-band verification/mutation), `testOrgID`, `scopeResolver`/`delegationMgr` where relevant. Use the helpers in `helpers_test.go` (unique names, condition waits).
4. **Prefix Zitadel-visible names** with the batch prefix (v0.18 uses `v018-`) so leftovers on the shared instance are attributable, and clean up with `t.Cleanup` — deletion order: tenant CRs → maps → anything pre-created directly in Zitadel.
5. Reference the S-number in a comment above the test function.

Conventions that keep the suite stable:

- Tests share one manager and one Zitadel instance — never assume exclusive global state (e.g. instance defaults: mutate and restore, or use the singleton scenarios' patterns).
- Assert on **conditions and reasons**, not log output.
- Out-of-band assertions go through `zitadelClient` (e.g. the actor proof lists Zitadel events); out-of-band mutations (deleting an app behind the operator's back) are how drift/self-healing scenarios are built.
- Anything that needs a second SSA writer or a second manager builds it from `testRestCfg` (see S-241 / S-270 for the pattern).
