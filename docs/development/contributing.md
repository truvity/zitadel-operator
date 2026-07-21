# Contributing

## Development environment

The toolchain is pinned by [devbox](https://www.jetify.com/devbox) (`devbox.json`: Go, golangci-lint, controller-gen, setup-envtest, just, helm, goreleaser, govulncheck, ko) and activated via direnv (`.envrc`). No globally installed Go required.

```bash
devbox shell          # or let direnv activate it on cd
just check            # build + unit tests + lint + govulncheck + verify-generate
```

Without a shell: prefix any command with `devbox run --`.

## Everyday commands (Justfile)

| Recipe | Does |
| --- | --- |
| `just generate` | controller-gen deepcopy + CRD manifests (synced into the CRD Helm chart) + the generated [API reference](../reference/api.md) (crd-ref-docs, version pinned in the Justfile) |
| `just build` | `generate` + `fmt` + build `bin/zitadel-operator` |
| `just test` | unit tests with coverage |
| `just test-integration` | integration suite — needs a real Zitadel + local config, see [Integration tests](integration-tests.md) |
| `just lint` / `just vuln` | golangci-lint / govulncheck |
| `just verify-generate` | fails if any generated file (CRDs, deepcopy, chart copies, API reference) is out of date |
| `just check` | the CI gate: build, test, lint, vuln, verify-generate |

## Making changes

### API types (`api/v1alpha2/`)

1. Edit the `*_types.go` file. Every field gets a doc comment — the generated CRD schema **and** the API reference are built from them, so write them for users.
2. `just generate` — deepcopy, CRD YAML (both `config/crd/bases/` and `charts/zitadel-operator-crds/templates/`), and `docs/reference/api.md` are all derived; commit them together with the type change.
3. Backward compatibility: v1alpha2 is serving traffic. Additive optional fields are fine; renames/removals are breaking and need a migration entry in the changelog plus, if config-visible, fail-fast handling.

### Controllers (`internal/controller/`)

- Reconcilers follow the lifecycle-helper pattern in `lifecycle.go` (`handleDeletionStrict`, `ensureFinalizer`, `markReady`, `waitForRef`) — implement only the business logic; target ≤15 cyclomatic complexity per `Reconcile`.
- Tenant reconcilers must run `tenantPreamble` first (dual-serving gate + scope resolution) and take their Zitadel client from `zclient(ctx, r.Zitadel)` — never call the binding client directly for tenant resources.
- **All status writes go through SSA** (`applyStatus`); never call `Status().Update()`. See [Status & SSA](../architecture/status-and-ssa.md).
- New behavior needs an integration scenario: allocate an S-number in [TEST-SCENARIOS.md](../TEST-SCENARIOS.md) and reference it from the test — process in [Integration tests](integration-tests.md#the-s-numbering-system).

### Documentation

Docs are plain Markdown under `docs/` (Diátaxis-ish: install / operations / architecture / reference / development). Rules of the house:

- `docs/reference/api.md` is generated — edit the Go doc comments, not the file.
- Guides link the reference instead of duplicating field tables.
- Architecture docs describe the **current** state only; historical decisions go to `docs/design/` as dated records, exploratory evidence to `docs/research/`.

## Pull requests

- CI runs `devbox run -- just check` — run it locally first.
- Integration tests are **not** run in CI (they need real Zitadel credentials); run the affected part locally and say so in the PR description, citing S-numbers.
- Conventional-commit style subjects (`feat:`, `fix:`, `docs:`, `chore:`) — the release changelog is grouped from them.
- User-visible changes get a `CHANGELOG.md` entry under `[Unreleased]`.

## Releases

Tag `vX.Y.Z` on master → the release workflow runs goreleaser: multi-arch binaries and container images (ko → GHCR), and both Helm charts pushed as OCI artifacts with the chart version set from the tag.
