# Contributing

## Checks

```bash
just check   # build + test + lint + govulncheck + generated-files drift
```

Green `just check` before pushing; CI runs the same recipes on PRs.
Integration tests against a live Zitadel instance: `just test-integration`
(30 min timeout; needs the test-instance credentials).

## Merging

The repository accepts **rebase merges only** — squash and merge-commit
are disabled in the repo settings. `gh pr merge <n> --rebase`.

## Release flow

Releases are tag-driven (`release.yaml`): goreleaser publishes the
image, then the two Helm charts (`zitadel-operator`,
`zitadel-operator-crds`) are pushed to `oci://ghcr.io/truvity/charts`
at the bare version (tag `v0.19.1` → chart/app version `0.19.1`).

1. Retitle the `[Unreleased]` CHANGELOG section to `[x.y.z] — YYYY-MM-DD`
   (leave a fresh empty `[Unreleased]` above it).
2. Commit as `chore(release): vx.y.z` on master.
3. Tag **annotated** — a lightweight tag fails the push hook:
   `git tag -a vx.y.z -m "vx.y.z — <one-liner>"`.
4. `git push && git push origin vx.y.z`, then watch the Release workflow.

Consumers pin the chart version in gitops `cfg/versions.yaml`
(`zitadelOperator`); bumping that pin rolls the fleet.
