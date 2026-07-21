# Migration Guide: v0.17 → v0.18

v0.18 replaces the two implicit routing mechanisms (`defaultOrganizationId`,
`projectScopeLabel`) with an explicit, delegable, fail-closed model:
**scope maps** (`ScopeMap`) route tenant namespaces to Zitadel scopes,
and the operator reconciles each mapped namespace with an internally minted,
scope-limited credential (**internal delegation**). How it works:
[Scope resolution](architecture/scope-resolution.md) and
[Internal delegation](architecture/delegation.md); decision history:
[v0.18 design record](design/2026-07-v018-scope-maps.md).

## Breaking configuration changes

The operator **fails fast at startup** when a removed key is present.

| v0.17 key | v0.18 replacement |
| --- | --- |
| `defaultOrganizationId` | **Removed — no default scope exists.** Either set `organizationId`/`organizationRef` explicitly on org-scoped CRs, or route the namespace through a `ScopeMap` (the scope supplies the organization). |
| `projectScopeLabel` | **Removed.** Label-value-as-project-name routing is superseded by scope-map rules (`namespaceSelector`/`namespaces` + `project`). |
| — | `binding: iam-owner \| org-owner` is **required**. It asserts what the mounted credential is; the operator verifies it via `AuthService.ListMyMemberships` at startup and crashes on mismatch. |
| — | `operatorNamespace` (optional; falls back to `POD_NAMESPACE`). Namespace holding `ScopeMap` objects and delegation Secrets. Unset ⇒ scope maps disabled (passthrough). |

`watchNamespaces` survives unchanged as the optional coarse informer filter —
semantic routing is scope maps only. The operator's own namespace must be
included when `watchNamespaces` is set.

## Migration steps

1. **Update the config/Helm values**:

   ```yaml
   # values.yaml
   config:
     domain: zitadel.example.com
     binding: iam-owner            # NEW, required — must match the credential
     # defaultOrganizationId: ...  # REMOVE
     # projectScopeLabel: ...      # REMOVE
   ```

2. **Make organizations explicit** on org-scoped CRs that relied on
   `defaultOrganizationId` (MachineUser, HumanUser, policies, Domain, …):
   either add `spec.organizationId` / `spec.organizationRef`, **or** create a
   scope map covering their namespaces (step 3) and remove nothing — the
   scope's organization is inherited.

3. **Create scope maps** (admin-only; in the operator's namespace) for each
   organization the operator serves:

   ```yaml
   apiVersion: zitadel.truvity.io/v1alpha2
   kind: ScopeMap
   metadata:
     name: acme-corp
     namespace: zitadel-operator          # operator's own namespace
   spec:
     instance: zitadel.example.com        # must match the operator binding
     organization: Acme Corp
     organizationId: "325908855630427886" # authoritative when set
     rules:
       - name: acme-product-namespaces
         namespaceSelector:
           matchLabels:
             tenancy.example.com/company: acme
         project: acme-platform           # project scope; omit for org scope
   ```

4. **Rollout gate**: with **zero** `ScopeMap` objects the operator
   behaves exactly like v0.17 minus the removed keys (passthrough). Strict
   fail-closed routing begins when the first map appears — from that moment a
   namespace matching no rule is rejected (`ScopeResolved=False /
   NoMatchingRule`).

5. **De-provisioning order matters**: delete tenant CRs first, then rules,
   then maps. Delegates are revoked eagerly when their scope stops matching;
   during deletion the operator falls back to the binding credential so
   finalizers never deadlock.

## Behavioral changes to be aware of

- **All status writes use Server-Side Apply** with field manager
  `zitadel-operator/<instance identity>` (`instanceAlias`, default `domain`);
  conditions are `listType=map`. Third-party
  conditions on operator-managed CRs now survive operator writes.
- **Leader election is on by default** (`--leader-elect=true`); the Helm chart
  passes `--leader-election-id=<fullname>`.
- **Dual-serving**: tenant CRs gained optional `spec.instance`. Pinned to a
  foreign domain ⇒ this operator ignores the CR entirely. Unset while two
  operators serve the namespace ⇒ both fail closed with `AmbiguousInstance`.
- **MachineUser**: optional `spec.roles` (grants on the scope project) and
  `spec.key.rotateAfter` (dual-key rotation with grace); the key Secret is now
  a connection bundle (`key.json`, `instanceUrl`, `issuer`, `orgId`,
  `projectId`, best-effort `instanceId`). CRs without the new fields behave
  exactly as before.
