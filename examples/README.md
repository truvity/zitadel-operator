# Examples

Small, copy-paste-able bundles for the common v0.18 shapes. Namespaces,
labels, org/project IDs and Secret names are placeholders — adjust to your
environment. Each file is self-contained and ordered for `kubectl apply -f`.

| File | Shows |
| --- | --- |
| [`scope-mapped-tenant.yaml`](scope-mapped-tenant.yaml) | A `ScopeMap` routing a labeled namespace to a project scope, plus a tenant `OIDCApp` that inherits org and project from the scope (no `projectRef`/`organizationId` on the app). |
| [`onboarding-bundle.yaml`](onboarding-bundle.yaml) | Passthrough-style per-cluster onboarding: `Project` → confidential `OIDCApp` → `MachineUser` with scope roles, key rotation and the connection-bundle Secret. |
| [`dual-served-namespace.yaml`](dual-served-namespace.yaml) | Two CRs in one namespace pinned to two different operators via `spec.instance` aliases. |

Background reading: [scope maps](../docs/operations/scope-maps.md),
[dual-serving](../docs/operations/dual-serving.md),
[configuration](../docs/install/configuration.md).
