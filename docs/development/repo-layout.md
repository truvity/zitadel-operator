# Repository Layout

```
zitadel-operator/
├── cmd/operator/main.go            # Entry point: config load, binding verification,
│                                   # manager + controller wiring, leader election
├── api/v1alpha2/                   # CRD Go types (single API version)
│   ├── *_types.go                  # One file per kind; doc comments feed the CRD
│   │                               # schema and the generated API reference
│   ├── common_types.go             # ResourceRef, SecretRefSpec, shared shapes
│   ├── policy_fields.go            # Field structs shared by Default*/org policy pairs
│   └── zz_generated.deepcopy.go    # GENERATED (controller-gen)
├── internal/
│   ├── config/                     # Config file loader + v0.18 fail-fast validation
│   ├── zitadel/                    # Zitadel client wrapper (v1/v2 SDK services,
│   │                               # split-horizon support) + binding verification
│   ├── scopemap/                   # Namespace→scope resolver + typed error taxonomy
│   ├── delegation/                 # Delegate mint/persist/rotate/revoke + orphan GC
│   └── controller/                 # One reconciler per CRD, plus shared machinery:
│       ├── lifecycle.go            #   finalizers, deletion, markReady/waitForRef
│       ├── common.go               #   conditions, SSA applyStatus, constants
│       ├── scope_resolution.go     #   tenantPreamble: instance gate + scope resolve
│       └── adoption.go             #   adopt-existing + secret regeneration
├── config/crd/bases/               # GENERATED CRD manifests (controller-gen)
├── charts/
│   ├── zitadel-operator/           # Operator chart (Deployment, RBAC, ConfigMap)
│   └── zitadel-operator-crds/      # CRD chart (templates = GENERATED copies)
├── tests/integration/              # envtest + real-Zitadel suite (build tag
│                                   # `integration`) — see integration-tests.md
├── docs/                           # This documentation tree
│   ├── install/  operations/  architecture/  development/
│   ├── reference/api.md            # GENERATED (crd-ref-docs)
│   ├── design/                     # Dated design records (ADR-style archive)
│   ├── research/                   # Dated research/evidence notes
│   ├── MIGRATION-0.18.md           # Referenced by the config fail-fast error
│   └── TEST-SCENARIOS.md           # S-numbered scenario catalog
├── Justfile                        # Dev commands (generate, build, test, check…)
├── devbox.json                     # Pinned toolchain
├── .goreleaser.yaml                # Release: binaries, ko images, charts
└── .github/workflows/              # ci (devbox + just check), release, security
```

## Orientation rules

- **Generated files** (`zz_generated.deepcopy.go`, `config/crd/bases/`, CRD chart templates, `docs/reference/api.md`) are committed and gated by `just verify-generate` — regenerate with `just generate`, never edit by hand.
- **`internal/` is not importable** — the operator deliberately exposes no Go API surface beyond the CRD types in `api/`.
- **One controller file per kind** in `internal/controller/`, named `<kind>_controller.go`; cross-cutting behavior belongs in the shared files, not copied per controller.
- **Unit tests** sit next to their code (`internal/**/**_test.go`, no build tag, run in CI); **integration tests** live only under `tests/integration/` behind the `integration` build tag (not run in CI).
