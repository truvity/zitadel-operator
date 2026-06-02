# Zitadel Operator

A Kubernetes operator for managing [Zitadel](https://zitadel.com) resources declaratively via Custom Resource Definitions (CRDs).

## Overview

The Zitadel Operator watches six CRDs under API group `zitadel.truvity.io/v1alpha1` and reconciles them against the Zitadel API using the official `zitadel-go/v3` SDK (gRPC, v2 APIs):

| CRD                | Scope     | Description                          |
| ------------------ | --------- | ------------------------------------ |
| OIDCApp            | Namespaced | OIDC application registrations       |
| Project            | Cluster    | Zitadel projects with roles          |
| IdentityProvider   | Cluster    | External IdP federations             |
| Organization       | Cluster    | Zitadel organizations                |
| MachineUser        | Namespaced | Service accounts with key-based auth |
| LoginPolicy        | Cluster    | Login policy configuration           |

## Features

- Declarative OIDC configuration as Kubernetes resources
- Idempotent reconciliation — no duplicate API calls for unchanged resources
- JWT Profile authentication (private key in K8s Secret)
- Two deployment modes:
  - **IAM_OWNER** — full instance admin (for central/kernel clusters)
  - **PROJECT_OWNER** — limited to one project (for child clusters)
- Status subresource with ready conditions, lastSyncTime, and error reporting
- Finalizer-based cleanup on CR deletion
- Generic and reusable — no cloud-provider-specific code

## Installation

### Helm (recommended)

```bash
# Install CRDs
helm install zitadel-operator-crds oci://ghcr.io/truvity/zitadel-operator/charts/zitadel-operator-crds

# Install operator
helm install zitadel-operator oci://ghcr.io/truvity/zitadel-operator/charts/zitadel-operator \
  --set zitadel.endpoint=https://your-zitadel-instance.example.com \
  --set zitadel.jwtSecretName=zitadel-admin-key
```

## Publishing

- Container image: built with [ko](https://ko.build), pushed to `ghcr.io/truvity/zitadel-operator`
- Helm charts: pushed to `oci://ghcr.io/truvity/charts` via OCI

## Development

### Prerequisites

- [devbox](https://www.jetify.com/devbox) (provides Go, golangci-lint, gopls, goreleaser, ko, helm, govulncheck, just)

### Getting Started

```bash
devbox shell
just generate   # Generate deepcopy methods and CRD manifests
just build      # Build the operator binary
just test       # Run tests
just lint       # Run linters
just vuln       # Run Go vulnerability check
just check      # Run all checks (generate + build + test + lint + vuln)
```

### Available Recipes

| Recipe         | Description                                        |
| -------------- | -------------------------------------------------- |
| `generate`     | Generate deepcopy methods and CRD manifests        |
| `build`        | Build the operator binary                          |
| `test`         | Run tests                                          |
| `lint`         | Run linters                                        |
| `vuln`         | Run Go vulnerability check (govulncheck)           |
| `check`        | Run all checks (generate + build + test + lint + vuln) |
| `snapshot`     | Build a snapshot release locally                   |
| `release`      | Create a release tag and push                      |
| `helm-package` | Package Helm charts locally                        |
| `helm-push`    | Push Helm charts to GHCR                           |
| `clean`        | Clean build artifacts                              |
| `tidy`         | Run go mod tidy                                    |

## License

MIT — see [LICENSE](LICENSE).
