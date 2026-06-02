# Zitadel Operator

A Kubernetes operator for managing [Zitadel](https://zitadel.com) resources declaratively via Custom Resource Definitions.

## CRDs

The operator manages 13 CRDs under `zitadel.truvity.io/v1alpha1`:

| CRD                      | Scope      | Description                       |
| ------------------------ | ---------- | --------------------------------- |
| Organization             | Cluster    | Zitadel organizations             |
| Project                  | Cluster    | Projects with roles               |
| IdentityProvider         | Cluster    | External IdP federations          |
| LoginPolicy              | Cluster    | Login policy configuration        |
| PasswordComplexityPolicy | Cluster    | Password complexity rules         |
| LockoutPolicy            | Cluster    | Account lockout rules             |
| OIDCApp                  | Namespaced | OIDC application registrations    |
| MachineUser              | Namespaced | Service accounts with key management |
| ProjectGrant             | Namespaced | Cross-org project grants          |
| UserGrant                | Namespaced | User role assignments             |
| ProjectMember            | Namespaced | Project membership                |
| ApplicationKey           | Namespaced | Application key management        |
| PersonalAccessToken      | Namespaced | PAT lifecycle                     |

## Modes

| Mode              | Use case                               | Access level                        |
| ----------------- | -------------------------------------- | ----------------------------------- |
| `iam-owner`       | Kernel cluster (central Zitadel admin) | Full instance control               |
| `project-owner`   | Child clusters                         | Scoped to a single project          |

## Split-Horizon Connectivity

The operator connects to Zitadel's internal service address (e.g. `zitadel.zitadel.svc.cluster.kernel:8080`) while authenticating against the external domain. No DNS hacks needed:

- `x-zitadel-instance-host` header on both gRPC and HTTP (token) requests tells Zitadel which instance to route to.
- `profile.WithStaticTokenEndpoint` skips OIDC discovery (which would fail against the internal hostname).
- JWT audience is `https://<external-domain>` — what Zitadel expects.

Set `--external-domain` to enable split-horizon mode.

## Authentication

JWT Profile via a Kubernetes Secret containing the Zitadel service account key JSON. The operator reads the secret at startup using a direct (non-cached) client.

## Installation

### Helm

```bash
# Install CRDs
helm install zitadel-operator-crds oci://ghcr.io/truvity/charts/zitadel-operator-crds

# Install operator
helm install zitadel-operator oci://ghcr.io/truvity/charts/zitadel-operator \
  --set zitadel.domain=zitadel.zitadel.svc.cluster.kernel \
  --set zitadel.externalDomain=zitadel.example.com \
  --set zitadel.jwtSecretName=zitadel-admin-sa
```

### Container Image

```
ghcr.io/truvity/zitadel-operator/operator
```

## Flags

| Flag                     | Default            | Description                                      |
| ------------------------ | ------------------ | ------------------------------------------------ |
| `--zitadel-domain`       | (required)         | Internal address of Zitadel                      |
| `--zitadel-port`         | `8080`             | Zitadel API port                                 |
| `--zitadel-insecure`     | `true`             | Connect without TLS (in-cluster)                 |
| `--external-domain`      | (empty)            | External domain — enables split-horizon          |
| `--jwt-secret-name`      | `zitadel-admin-sa` | Secret name containing JWT key                   |
| `--jwt-secret-namespace` | `zitadel`          | Namespace of the JWT secret                      |
| `--jwt-secret-key`       | `<name>.json`      | Data key within the secret                       |
| `--mode`                 | `iam-owner`        | `iam-owner` or `project-owner`                   |
| `--project`              | (empty)            | Project name (required for `project-owner` mode) |

## Development

### Prerequisites

[devbox](https://www.jetify.com/devbox) provides the full toolchain: Go, golangci-lint (v2), gopls, goreleaser, ko, helm, govulncheck, controller-gen, just.

### Commands

```bash
devbox shell

just generate       # deepcopy + CRD manifests → charts/
just build          # compile operator binary
just test           # go test with coverage
just lint           # golangci-lint v2
just vuln           # govulncheck
just check          # all of the above
just verify-generate # fail if generated files are stale (used in CI)
just snapshot       # local goreleaser snapshot
just helm-push <v>  # push charts to oci://ghcr.io/truvity/charts/
```

### CI

GitHub Actions runs `just check` and `just verify-generate` on every push/PR to master. The `verify-generate` step ensures CRD manifests and deepcopy files are committed and up to date.

## License

MIT — see [LICENSE](LICENSE).
