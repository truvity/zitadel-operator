# Design Record: Ecosystem Repositories & Shared Conventions

**Date:** 2026-06
**Status:** Implemented (all repos exist and follow the conventions)

> Historical record: the plan that established the Zitadel identity-automation
> ecosystem around this operator, and the implementation conventions shared by
> its repositories. Each repo's own README/docs are authoritative for its
> current state.

## Repository split

| Repository | Purpose | Knows about |
| --- | --- | --- |
| `truvity/zitadel-operator` | K8s operator (CRDs, reconcilers); ecosystem master plan | Zitadel only |
| `truvity/zitadel-rbac-mapper` | Groups→Zitadel grants mapping webhook (Actions v2 target); token-enrichment and event-driven sync modes | Zitadel (groups come as input) |
| `truvity/zitadel-notify-relay` | HTTP notification provider relay (Email/SMS → AWS SES/SNS, extensible) | Zitadel notification payloads + delivery providers |
| `truvity/google-group-sync` | Google Workspace group resolver (`POST /resolve` → groups) | Google only — no Zitadel dependency |

Composition: Google Workspace → google-group-sync → rbac-mapper (invoked by Zitadel Actions v2 on preuserinfo/preaccesstoken) → claims/grants. The operator's `ActionTarget`/`ActionExecution` CRDs declare the webhook wiring declaratively.

## Shared conventions (established here, adopted by all repos)

- **Environment:** devbox-pinned toolchain + direnv; Justfile recipes (`build`, `test`, `lint`, `vuln`, `check`, `snapshot`); golangci-lint with a shared linter set.
- **Service architecture** (the three non-operator repos): bare Go `main` with `signal.NotifyContext`, `pkg/app` wiring, env-only config, `slog`, fiber/v3 HTTP, RFC 9457 `application/problem+json` errors; no auth in-binary (platform-delegated: Lambda Function URL `AWS_IAM`, or NetworkPolicy in-cluster).
- **Release:** GoReleaser — multi-arch binaries, ko container images (distroless, GHCR), Helm charts as OCI (version injected from the git tag at package time), Lambda ZIPs with the binary named `bootstrap`; deploy examples as Pulumi Go programs consuming public release assets.
- **CI:** GitHub Actions with devbox (`devbox run -- just check`); integration tests never in CI — they need real external services; local secrets in the system keyring (`go-keyring` / `secret-tool`), config under `~/.config/<service>/config.yaml`; LocalStack for AWS-dependent testing.

## Coordination

The operator repo is the ecosystem's master plan (`docs/`), with each repo carrying its own README and plan. Integration points were validated together in the operator's Actions v2 scenarios (rbac-mapper webhook end-to-end).
