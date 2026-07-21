# Zitadel Operator

[![CI](https://github.com/truvity/zitadel-operator/actions/workflows/ci.yaml/badge.svg)](https://github.com/truvity/zitadel-operator/actions/workflows/ci.yaml)
[![Release](https://github.com/truvity/zitadel-operator/actions/workflows/release.yaml/badge.svg)](https://github.com/truvity/zitadel-operator/actions/workflows/release.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/truvity/zitadel-operator)](https://goreportcard.com/report/github.com/truvity/zitadel-operator)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Kubernetes operator for managing [Zitadel](https://zitadel.com) resources declaratively via Custom Resource Definitions.

Works with Zitadel Cloud and self-hosted Zitadel instances.

## Features

- **42 CRDs** covering organization, project, application, user, policy, and notification management
- **Structured status conditions** — `Ready=True/False` with reason codes and messages
- **Hierarchical references** — Organization → Project → App/User with automatic resolution
- **Config file driven** — single `--config` flag, no CLI argument wall
- **Namespace isolation** — multiple operators via `watchNamespaces` + K8s RBAC
- **Idempotent reconciliation** — no hot-loops, conditional status updates, drift detection
- **Finalizer-based cleanup** — resources deleted from Zitadel on CR deletion
- **Secret output** — OIDC client credentials and machine keys written to K8s Secrets

## CRD Catalog

### Project-Level Resources (PROJECT_OWNER)

These require the operator SA to have `PROJECT_OWNER` membership on the target project.

| CRD                    | Description                                           | Secret Output                 |
| ---------------------- | ----------------------------------------------------- | ----------------------------- |
| **OIDCApp**            | OIDC application (confidential or public)             | `client_id` + `client_secret` |
| **APIApp**             | API/M2M application (basic or private_key_jwt)        | `client_id` + `client_secret` |
| **SAMLApp**            | SAML application (metadata XML or URL)                | —                             |
| **ApplicationKey**     | JWT key for applications                              | `key.json`                    |
| **ProjectMember**      | Assign a user role on a project (e.g., PROJECT_OWNER) | —                             |
| **ProjectGrantMember** | Assign a user role on a project grant                 | —                             |

### Organization-Level Resources (ORG_OWNER)

These require the operator SA to have `ORG_OWNER` role in the organization.

| CRD                          | Description                                 | Secret Output |
| ---------------------------- | ------------------------------------------- | ------------- |
| **Organization**             | Create/manage a Zitadel organization        | —             |
| **Project**                  | Create/manage a project with role sync      | —             |
| **MachineUser**              | Create a service account with JWT key       | `key.json`    |
| **PersonalAccessToken**      | Personal access token for users             | `token`       |
| **UserGrant**                | Assign project roles to a user              | —             |
| **ProjectGrant**             | Share a project with another organization   | —             |
| **OrgMetadata**              | Key-value metadata on the organization      | —             |
| **Domain**                   | Register an org domain for domain discovery | —             |
| **IdentityProvider**         | Configure generic OIDC identity provider    | —             |
| **LoginPolicy**              | Org-scoped login policy                     | —             |
| **PasswordComplexityPolicy** | Org-scoped password complexity policy       | —             |
| **LockoutPolicy**            | Org-scoped lockout policy                   | —             |
| **HumanUser**                | Create/manage human users                   | —             |
| **OrgMember**                | Assign org-level roles to users             | —             |
| **LabelPolicy**              | Org-scoped branding/label policy            | —             |
| **NotificationPolicy**       | Org-scoped notification policy              | —             |
| **PasswordAgePolicy**        | Org-scoped password age policy              | —             |
| **PrivacyPolicy**            | Org-scoped privacy policy                   | —             |
| **MessageText**              | Org-scoped custom message text              | —             |

### Instance-Level Resources (IAM_OWNER)

These require the operator SA to have `IAM_OWNER` role (instance administrator).

| CRD                     | Description                                    | Notes                          |
| ----------------------- | ---------------------------------------------- | ------------------------------ |
| **ActionTarget**        | Webhook target for Actions v2                  | Requires instance-level access |
| **ActionExecution**     | Bind targets to trigger conditions             | Requires instance-level access |
| **DefaultLoginPolicy**  | Instance default login policy                  | Singleton per instance         |
| **DefaultDomainPolicy** | Instance default domain policy                 | Singleton per instance         |
| **GoogleIdP**           | Instance-scoped Google identity provider       | Exposes `status.idpID` for ref |
| **EmailProvider**       | Email delivery provider (SMTP or HTTP webhook) | Activated after creation       |
| **DefaultLockoutPolicy** | Instance default lockout policy | Singleton per instance |
| **DefaultPasswordComplexityPolicy** | Instance default password complexity | Singleton per instance |
| **DefaultPasswordAgePolicy** | Instance default password age | Singleton per instance |
| **DefaultNotificationPolicy** | Instance default notification policy | Singleton per instance |
| **DefaultLabelPolicy** | Instance default label/branding policy | Singleton per instance |
| **DefaultPrivacyPolicy** | Instance default privacy policy | Singleton per instance |
| **DefaultOIDCSettings** | Instance default OIDC settings (token lifetimes) | Singleton per instance |
| **GitHubIdP** | Instance-scoped GitHub identity provider | Exposes `status.idpID` for ref |
| **SmsProvider** | SMS delivery provider (Twilio or HTTP) | Activated after creation |
| **InstanceMember** | Instance-level role assignment | IAM_OWNER, IAM_ORG_MANAGER |
| **DefaultMessageText** | Instance default message text (per type+language) | One CR per type+language |

> **Note:** On Zitadel Cloud, instance-level resources require the SA to be an instance admin.
> For most deployments, use the org-level and project-level CRDs only.

## Permissions Guide

The operator authenticates to Zitadel using a JWT Profile (service account key). The SA's permissions determine which CRDs the operator can manage:

| SA Role                               | Can Manage                                      |
| ------------------------------------- | ----------------------------------------------- |
| `PROJECT_OWNER` on a specific project | OIDCApp, ProjectMember (for that project only)  |
| `ORG_OWNER` in the organization       | All org-level + project-level CRDs              |
| `IAM_OWNER` (instance admin)          | All CRDs including ActionTarget/ActionExecution |

### Minimum Viable Setup (per-service apps only)

For operators that only manage OIDCApps for K8s services:

```yaml
# SA needs PROJECT_OWNER on each project it manages apps for
config:
  domain: auth.example.com
  port: "443"
```

### Full Setup (all org resources)

For operators that manage the complete identity lifecycle:

```yaml
# SA needs ORG_OWNER
config:
  domain: auth.example.com
  port: "443"
  binding: org-owner   # v0.18: assert the credential level (verified at startup)
```

## Installation

### Helm (recommended)

```bash
# Install CRDs
helm install zitadel-operator-crds oci://ghcr.io/truvity/charts/zitadel-operator-crds --version 0.11.0

# Install operator
helm install zitadel-operator oci://ghcr.io/truvity/charts/zitadel-operator --version 0.11.0 \
  --set config.domain=auth.example.com \
  --set credentials.secretName=zitadel-sa-key
```

### Configuration

The operator uses a single YAML config file:

```yaml
# /etc/zitadel-operator/config.yaml
domain: auth.example.com          # Zitadel domain
binding: iam-owner                # v0.18 (required): iam-owner | org-owner,
                                  # verified against the credential at startup
port: "443"                        # Zitadel API port
insecure: false                    # Use TLS
keyFile: /etc/zitadel/key.json    # Path to JWT key file
operatorNamespace: zitadel-operator # Namespace holding ZitadelScopeMaps +
                                  # delegation Secrets (default: POD_NAMESPACE)
watchNamespaces:                   # Limit to these namespaces (optional)
  - zitadel-operator
  - argocd
```

> **v0.18 breaking:** `defaultOrganizationId` and `projectScopeLabel` were
> removed — the operator fails fast at startup if either is present. Namespace
> routing is now explicit via `ZitadelScopeMap` objects in the operator
> namespace (zero maps = legacy passthrough). See
> [docs/MIGRATION-0.18.md](docs/MIGRATION-0.18.md).

## Usage Examples

### OIDCApp (most common)

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
  postLogoutRedirectUris:
    - https://argocd.example.com
  accessTokenRoleAssertion: true
  idTokenRoleAssertion: true
  secretRef:
    name: argocd-oidc
    keys:
      clientId: oidc.clientID
      clientSecret: oidc.clientSecret
    extraData:
      OIDC_ISSUER_URL: "https://auth.example.com"
```

### Project with Roles

```yaml
apiVersion: zitadel.truvity.io/v1alpha2
kind: Project
metadata:
  name: my-project
  namespace: zitadel-operator
spec:
  organizationId: "376393772658861254"
  roles:
    - admin
    - viewer
    - editor
```

### MachineUser with Key

```yaml
apiVersion: zitadel.truvity.io/v1alpha2
kind: MachineUser
metadata:
  name: ci-bot
  namespace: zitadel-operator
spec:
  userName: ci-bot@example.com
  description: "CI/CD service account"
  accessTokenType: jwt
  keySecretRef:
    name: ci-bot-key
    key: key.json
```

### IdentityProvider (Generic OIDC)

```yaml
apiVersion: zitadel.truvity.io/v1alpha2
kind: IdentityProvider
metadata:
  name: corporate-idp
  namespace: zitadel-operator
spec:
  name: "Corporate SSO"
  issuer: "https://sso.corp.example.com"
  clientId: "zitadel-client"
  clientSecret: "secret"
  scopes: ["openid", "email", "profile"]
  isAutoCreation: true
  isAutoUpdate: true
  isLinkingAllowed: true
```

### Cross-Resource References

Resources can reference each other by name (resolved automatically):

```yaml
apiVersion: zitadel.truvity.io/v1alpha2
kind: OIDCApp
metadata:
  name: my-app
  namespace: default
spec:
  projectRef:
    name: my-project
    namespace: zitadel-operator
  # ...
```

### ActionTarget + ActionExecution (RBAC Webhook)

Fully declarative Actions V2 configuration — survives cluster rebuilds without manual intervention:

```yaml
apiVersion: zitadel.truvity.io/v1alpha2
kind: ActionTarget
metadata:
  name: rbac-mapper
  namespace: zitadel-operator
spec:
  endpoint: "https://rbac-mapper.example.com/webhook"
  timeout: "30s"
  interruptOnError: true
  targetType: restCall       # restCall | restWebhook | restAsync (default: restCall)
  payloadType: jwt           # json | jwt | jwe (default: json)
---
apiVersion: zitadel.truvity.io/v1alpha2
kind: ActionExecution
metadata:
  name: rbac-mapper-preuserinfo
  namespace: zitadel-operator
spec:
  condition:
    function: preuserinfo    # function | request | event (mutually exclusive)
  targets:
    - targetRef:
        name: rbac-mapper
---
apiVersion: zitadel.truvity.io/v1alpha2
kind: ActionExecution
metadata:
  name: rbac-mapper-preaccesstoken
  namespace: zitadel-operator
spec:
  condition:
    function: preaccesstoken
  targets:
    - targetRef:
        name: rbac-mapper
```

**Target types:**
- `restCall` — reads response body (required for `append_claims` in function executions)
- `restWebhook` — checks status code only, ignores response body
- `restAsync` — fire-and-forget, does not wait for response (use for event executions)

**Payload types:**
- `json` — JSON body with `X-ZITADEL-Signature` header for integrity verification
- `jwt` — signed JWT body (receiver verifies via Zitadel's JWKS endpoint)
- `jwe` — encrypted JWT body (requires providing an encryption public key to Zitadel)

## Development

```bash
# Enter dev shell
devbox shell

# Run all checks
just check

# Run integration tests (requires real Zitadel + config)
just test-integration

# Generate CRDs after type changes
just generate
```

### Integration Test Setup

```bash
# Store JWT key in system keyring
secret-tool store --label='zitadel-operator jwt-key' \
  service zitadel-operator username jwt-key < /path/to/key.json

# Delete the key file after storing (don't leave secrets on disk)
rm /path/to/key.json

# Verify it's stored correctly
secret-tool lookup service zitadel-operator username jwt-key | head -c 20

# Create config
cat > ~/.config/zitadel-operator/config.yaml << EOF
domain: your-instance.eu1.zitadel.cloud
port: "443"
insecure: false
binding: iam-owner
EOF

# Run tests
just test-integration
```

## Architecture

```
[K8s CRs] → [Operator Controllers] → [Zitadel APIs]
                    │
                    ├── v2 APIs: Project, Organization, OIDCApp, IDP
                    └── v1 Management API: MachineUser, UserGrant, ProjectMember,
                                           OrgMetadata, Domain, ProjectGrant, Roles
                    │
                    └── [K8s Secrets] (credentials output)
```

- **One operator per Zitadel instance** — config file binds to a specific domain
- **Namespace-scoped RBAC** — multiple operators in one cluster via `watchNamespaces`
- **No CRD for connection** — instance config is deployment config, not a reconciled resource

## Multi-Instance Deployment

For deployments that need multiple operators (e.g., separate employee and customer identity providers), see [Multi-Instance Deployment Guide](docs/GUIDE-MULTI-INSTANCE.md).

Key features enabling multi-instance:
- **`watchNamespaces`** — each operator watches only its namespaces (coarse filter)
- **`ZitadelScopeMap`** (v0.18) — explicit, fail-closed routing of tenant namespaces to Zitadel org/project scopes; reconciliation runs with internally minted, scope-limited delegate credentials
- **`spec.instance`** (v0.18) — per-CR instance pin for dual-served namespaces (unset + dual-served = fail-closed `AmbiguousInstance`)
- **Namespace-scoped RBAC** — K8s enforces isolation at the API server level

## License

MIT
