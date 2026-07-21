# Zitadel Operator

[![CI](https://github.com/truvity/zitadel-operator/actions/workflows/ci.yaml/badge.svg)](https://github.com/truvity/zitadel-operator/actions/workflows/ci.yaml)
[![Release](https://github.com/truvity/zitadel-operator/actions/workflows/release.yaml/badge.svg)](https://github.com/truvity/zitadel-operator/actions/workflows/release.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/truvity/zitadel-operator)](https://goreportcard.com/report/github.com/truvity/zitadel-operator)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A Kubernetes operator that manages [Zitadel](https://zitadel.com) resources declaratively through Custom Resource Definitions — organizations, projects, OIDC/API/SAML applications, users, grants, policies, identity providers, and notification providers. Works with Zitadel Cloud and self-hosted instances.

## Why

- **GitOps for identity** — applications declare their own OIDC clients and service accounts next to their Deployments; credentials land in Kubernetes Secrets.
- **Explicit, fail-closed multi-tenancy** — `ScopeMap` objects route tenant namespaces to Zitadel org/project scopes, and each scope is reconciled with an internally minted, scope-limited credential ([internal delegation](docs/architecture/delegation.md)). A namespace that matches no rule is rejected, not defaulted.
- **Single writer with drift detection** — within a managed scope the operator owns the resources it declares: out-of-band edits are detected and reverted on the next reconcile.
- **43 CRDs** with structured `Ready` conditions, finalizer-based cleanup, idempotent reconciliation (no hot loops), and Server-Side Apply status writes that coexist with other controllers.

## Quickstart

```bash
# 1. Install the CRDs (cluster-scoped, install once)
helm install zitadel-operator-crds oci://ghcr.io/truvity/charts/zitadel-operator-crds

# 2. Create the credential Secret (JWT key of a Zitadel service account)
kubectl -n zitadel-operator create secret generic zitadel-admin-sa \
  --from-file=zitadel-admin-sa.json=/path/to/key.json

# 3. Install the operator
helm install zitadel-operator oci://ghcr.io/truvity/charts/zitadel-operator \
  --namespace zitadel-operator --create-namespace \
  --set config.domain=auth.example.com \
  --set config.binding=iam-owner \
  --set credentials.secretName=zitadel-admin-sa
```

Then declare an application:

```yaml
apiVersion: zitadel.truvity.io/v1alpha2
kind: OIDCApp
metadata:
  name: argocd
  namespace: argocd
spec:
  projectId: "376393790358824134"
  type: confidential
  authMethod: basic
  redirectUris:
    - https://argocd.example.com/auth/callback
  secretRef:
    name: argocd-oidc
```

The operator creates the application in Zitadel and writes `client_id` + `client_secret` into the `argocd-oidc` Secret. See the [Helm installation guide](docs/install/helm.md) for the full walk-through, including the `binding` choice (`iam-owner` vs `org-owner`) that determines what the operator can manage.

## Documentation

| Section | Contents |
| --- | --- |
| **Install** | [Helm installation](docs/install/helm.md) · [Configuration reference](docs/install/configuration.md) · [Binding levels: iam-owner vs org-owner](docs/install/binding-levels.md) |
| **Operations** | [Multi-operator topologies](docs/operations/multi-operator.md) · [Scope-map administration & delegation](docs/operations/scope-maps.md) · [Dual-serving one namespace](docs/operations/dual-serving.md) · [Large multi-tenant installations](docs/operations/large-installations.md) · [Metrics](docs/operations/metrics.md) · [Troubleshooting](docs/operations/troubleshooting.md) |
| **Architecture** | [Resource hierarchy & CRD map](docs/architecture/resource-hierarchy.md) · [Scope resolution](docs/architecture/scope-resolution.md) · [Internal delegation & credential lifecycle](docs/architecture/delegation.md) · [Dual-serving semantics](docs/architecture/dual-serving.md) · [Status conditions & SSA](docs/architecture/status-and-ssa.md) |
| **Reference** | [CRD API reference](docs/reference/api.md) (generated from the Go types) |
| **Development** | [Contributing](docs/development/contributing.md) · [Repository layout](docs/development/repo-layout.md) · [Integration-test architecture](docs/development/integration-tests.md) |
| **History** | [Changelog](CHANGELOG.md) · [Migration v0.17 → v0.18](docs/MIGRATION-0.18.md) · [Design records](docs/design/README.md) · [Research notes](docs/research/) |

## Resource model at a glance

```
Zitadel instance          ← operator deployment config (domain + binding), not a CRD
└── Organization          ← Organization CR, or a pre-existing org routed via scope maps
    ├── Project           ← Project CR (inline roles)
    │   ├── OIDCApp / APIApp / SAMLApp / ApplicationKey
    │   └── ProjectMember / ProjectGrant / ProjectGrantMember
    ├── MachineUser / HumanUser / PersonalAccessToken / UserGrant / OrgMember
    ├── IdentityProvider / org-scoped policies / MessageText / Domain / OrgMetadata
    └── (instance level) Default* policies, GoogleIdP/GitHubIdP, Email/SmsProvider,
        ActionTarget/ActionExecution, InstanceMember, DefaultMessageText
```

All CRDs are namespaced; namespaces are the RBAC and tenancy boundary. The full catalog with permission requirements lives in [Resource hierarchy](docs/architecture/resource-hierarchy.md); every spec/status field is documented in the [API reference](docs/reference/api.md).

## Key invariants

- **One operator = one Zitadel instance = one credential**, asserted at startup (`binding: iam-owner | org-owner`, verified against the credential's real memberships — mismatch crashes before any reconcile).
- **Routing authority lives with the operator**, never with the routed tenant: scope maps are admin-controlled objects in the operator's namespace; namespace labels are facts the maps interpret.
- **The binding credential only mints, rotates, and revokes** delegated service accounts; tenant resources are reconciled with the scope-limited delegate key.
- **Zero scope maps = passthrough**: without any `ScopeMap`, the operator reconciles directly with the binding credential (the pre-v0.18 behavior), so installing the CRD is not a flag day.

## License

MIT
