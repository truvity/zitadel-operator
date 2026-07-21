# Installing with Helm

Two OCI charts are published to GHCR on every release:

| Chart | Contents | Scope |
| --- | --- | --- |
| `oci://ghcr.io/truvity/charts/zitadel-operator-crds` | All 43 CRDs | Cluster-scoped — install **once** per cluster, shared by every operator deployment |
| `oci://ghcr.io/truvity/charts/zitadel-operator` | Deployment, ServiceAccount, RBAC, ConfigMap | One release **per Zitadel instance** you manage |

## Before you install: two decisions

### 1. Binding level (`config.binding`, required)

The operator mounts exactly one Zitadel service-account credential and asserts its privilege level in config. This is a first-class deployment choice — it decides what the operator can manage. Read [Binding levels](binding-levels.md) before picking:

- **`iam-owner`** — instance-level deployment. Full CRD surface: instance defaults, instance IdPs, notification providers, Actions, plus everything org- and project-level in any organization.
- **`org-owner`** — org-level deployment. Everything inside exactly one organization; instance-level CRs degrade cleanly to `Ready=False / NotSupportedAtBindingLevel`, and scope maps for foreign organizations are rejected.

The assertion is verified at startup against the credential's actual memberships; a mismatch in either direction crashes the operator before any reconcile.

### 2. RBAC mode (`rbac.namespaces`)

- **Cluster mode** (default, `rbac.namespaces: []`) — ClusterRole/ClusterRoleBinding; the operator watches all namespaces.
- **Namespaced mode** (`rbac.namespaces: [...]`) — a Role/RoleBinding per listed namespace; the Kubernetes API server enforces the boundary. Pair it with `config.watchNamespaces` (informer filter) listing the same namespaces. The release namespace **must be included** — scope maps and delegation Secrets live there. A small ClusterRole for reading Namespaces is always created in this mode (scope-map selectors evaluate namespace labels).

## Steps

### 1. Install the CRDs

```bash
helm install zitadel-operator-crds oci://ghcr.io/truvity/charts/zitadel-operator-crds
```

CRDs are cluster-scoped: with multiple operator deployments, install this chart once and never per operator.

### 2. Provide the credential

Create a Secret holding the JWT key JSON of the Zitadel service account (Zitadel Console → Service Users → Keys, or provisioned by IaC). In production this is typically synced by External Secrets Operator from a cloud secret store.

```bash
kubectl -n zitadel-operator create secret generic zitadel-admin-sa \
  --from-file=zitadel-admin-sa.json=/path/to/key.json
```

The key is mounted as a file (kubelet keeps it in sync on rotation); the operator never needs Secret-read RBAC for its own credential.

### 3. Install the operator

```yaml
# values.yaml
config:
  domain: auth.example.com
  binding: iam-owner            # required: iam-owner | org-owner
  # port: "443"                 # default
  # watchNamespaces: []         # empty = watch all namespaces
  # operatorNamespace: ""       # default: the release namespace

credentials:
  secretName: zitadel-admin-sa
  key: zitadel-admin-sa.json
  mountPath: /etc/zitadel
```

```bash
helm install zitadel-operator oci://ghcr.io/truvity/charts/zitadel-operator \
  --namespace zitadel-operator --create-namespace -f values.yaml
```

The chart renders the operator config into a ConfigMap (see the [configuration reference](configuration.md) for every key) and mounts the credential Secret at `credentials.mountPath`.

### 4. Verify

```bash
kubectl -n zitadel-operator logs deploy/zitadel-operator | head
# expect: "config loaded", "Zitadel client initialized successfully",
#         "binding verified" — and with an operator namespace set,
#         "v0.18 scope maps enabled"
```

A crash at startup with `binding verification failed` or `"...was removed in v0.18"` is deliberate fail-fast — see [Troubleshooting](../operations/troubleshooting.md#startup-failures).

## Namespaced-mode example

```yaml
config:
  domain: auth.example.com
  binding: org-owner
  watchNamespaces:
    - zitadel-operator      # the release namespace — always required here
    - billing
    - storefront

rbac:
  namespaces:
    - zitadel-operator
    - billing
    - storefront
```

`watchNamespaces` and `rbac.namespaces` should list the same set: the first filters the informer cache, the second is what the API server actually allows. Adding a namespace later means updating both and upgrading the release.

## Leader election

Leader election is **on by default** and should stay on: even with `replicas: 1`, a rolling update or node drain runs two pods simultaneously, and without a lease both would reconcile at once (double-minted delegated service accounts, racing Secret writes). The chart derives the lease ID from the release fullname, so two operator deployments in one namespace hold distinct leases. `leaderElection.enabled: false` exists for development only.

## Multiple operators

Deploy one release per Zitadel instance (separate values, separate credential Secrets, one shared CRD chart). Topologies, isolation layers, and deployment order: [Multi-operator topologies](../operations/multi-operator.md).

## Upgrading

- Upgrade the CRD chart first, then operator releases (`helm upgrade` each).
- Upgrading across a breaking version? Check [CHANGELOG.md](../../CHANGELOG.md); for v0.17 → v0.18 follow [MIGRATION-0.18.md](../MIGRATION-0.18.md) — two config keys were removed and the operator fails fast if they are present.
