# Zitadel Operator

A Kubernetes operator for managing [Zitadel](https://zitadel.com) resources declaratively via Custom Resource Definitions (CRDs).

## Overview

The Zitadel Operator watches six CRDs under API group `zitadel.truvity.io/v1alpha1` and reconciles them against the Zitadel API using the official `github.com/zitadel/zitadel-go/v3` SDK:

- **OIDCApp** (namespaced) — OIDC application registrations
- **Project** (cluster-scoped) — Zitadel projects with roles
- **IdentityProvider** (cluster-scoped) — External IdP federations
- **Organization** (cluster-scoped) — Zitadel organizations
- **MachineUser** (namespaced) — Service accounts with key-based auth
- **LoginPolicy** (cluster-scoped) — Login policy configuration

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

## Development

### Prerequisites

- [devbox](https://www.jetify.com/devbox) (provides Go, golangci-lint, gopls, goreleaser)

### Getting Started

```bash
devbox shell
make generate   # Generate CRD manifests
make build      # Build the operator binary
make test       # Run tests
make lint       # Run linters
```

## License

MIT — see [LICENSE](LICENSE).
