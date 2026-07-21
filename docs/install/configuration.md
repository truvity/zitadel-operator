# Configuration Reference

The operator reads a single YAML config file; the only meaningful CLI flag is the path to it. In the Helm chart the file is rendered from `.Values.config` into a ConfigMap and mounted at `/etc/zitadel-operator/config.yaml`.

```yaml
# /etc/zitadel-operator/config.yaml
domain: auth.example.com
binding: iam-owner
port: "443"
insecure: false
externalDomain: ""
keyFile: /etc/zitadel/key.json
operatorNamespace: zitadel-operator
watchNamespaces:
  - zitadel-operator
  - argocd
```

## Keys

| Key | Required | Default | Description |
| --- | --- | --- | --- |
| `domain` | yes | — | Zitadel instance domain the operator connects to. Also the operator's identity: the SSA field manager is `zitadel-operator/<domain>`, scope maps must assert this domain in `spec.instance`, and CR-level `spec.instance` pins compare against it. |
| `binding` | yes | — | Credential assertion: `iam-owner` or `org-owner`. Verified at startup via `AuthService.ListMyMemberships`; a mismatch in either direction is fatal. See [Binding levels](binding-levels.md). |
| `port` | no | `"443"` | Zitadel API port (string). |
| `insecure` | no | `false` | Connect over plain HTTP (no TLS). Local development only. |
| `externalDomain` | no | — | Split-horizon mode: connect to `domain:port` internally while Zitadel is configured with a different canonical external domain. The operator sends the `x-zitadel-instance-host` header and signs its JWT for the `https://<externalDomain>` audience. |
| `keyFile` | yes | — | Path to the mounted JWT key JSON of the operator's service account. The Helm chart sets this to `credentials.mountPath`/`credentials.key`. |
| `operatorNamespace` | no | `$POD_NAMESPACE` | Namespace holding `ZitadelScopeMap` objects and delegation Secrets. The Helm chart defaults it to the release namespace. If neither the key nor `POD_NAMESPACE` resolves, **scope maps are disabled entirely** and the operator runs in passthrough mode. |
| `watchNamespaces` | no | all namespaces | Coarse informer filter: the operator only caches/watches the listed namespaces. This is *not* semantic routing (that is scope maps) — it bounds visibility and memory. When set, it must include `operatorNamespace`. |

### Removed keys (fail-fast since v0.18)

The operator refuses to start when either of these appears in the config, with an error pointing at [MIGRATION-0.18.md](../MIGRATION-0.18.md):

| Removed key | Replacement |
| --- | --- |
| `defaultOrganizationId` | No default scope exists. Set `organizationId`/`organizationRef` explicitly on org-scoped CRs, or route their namespaces through a [`ZitadelScopeMap`](../operations/scope-maps.md). |
| `projectScopeLabel` | Scope-map rules (`namespaceSelector`/`namespaces` + `project`). |

## CLI flags

Flags cover process wiring only; everything about the Zitadel connection lives in the config file.

| Flag | Default | Description |
| --- | --- | --- |
| `--config` | `~/.config/zitadel-operator/config.yaml` | Config file path. The Helm chart passes `/etc/zitadel-operator/config.yaml`. |
| `--leader-elect` | `true` | Leader election. Keep on in any real deployment — a rolling update always runs two pods at once. |
| `--leader-election-id` | `zitadel-operator.truvity.io` | Lease name. The Helm chart sets it to the release fullname so co-located deployments get distinct leases. |
| `--metrics-bind-address` | `:8080` | Metrics endpoint. |
| `--health-probe-bind-address` | `:8081` | `/healthz` + `/readyz` endpoint. |

## Environment

| Variable | Used for |
| --- | --- |
| `POD_NAMESPACE` | Fallback for `operatorNamespace` (the chart does not rely on it — it renders the key explicitly). |

## Why a config file and not flags or a CRD

One source of truth with no flag/env/file precedence rules, the same file shape for local development and production, and no reconciliation complexity for connection lifecycle: the operator is always 1:1 with an instance, so "which instance" is deployment config, not a reconciled resource. The full rationale is recorded in the [v1alpha2 design record](../design/2026-06-v1alpha2-redesign.md).
