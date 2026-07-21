# Troubleshooting

The operator communicates through three channels, in order of usefulness:

1. **Conditions** on the CR's status (`kubectl describe <kind> <name>` or `-o jsonpath='{.status.conditions}'`)
2. **Events** on scope maps and CRs (`kubectl -n <ns> get events --field-selector involvedObject.name=<name>`)
3. **Operator logs** (structured; startup failures land only here)

## Startup failures

The pod crash-looping right after deploy is almost always deliberate fail-fast. Check `kubectl logs`:

| Log error | Cause | Fix |
| --- | --- | --- |
| `"defaultOrganizationId" was removed in v0.18` / `"projectScopeLabel" was removed in v0.18` | Pre-v0.18 config keys present | Follow [MIGRATION-0.18.md](../MIGRATION-0.18.md) |
| `binding is required since v0.18` | No `config.binding` | Set `binding: iam-owner` or `org-owner` |
| `binding asserted iam-owner but the credential has no IAM_OWNER membership` | Assertion above the credential | Fix the credential or lower the assertion |
| `binding asserted org-owner but the credential holds instance-level IAM_OWNER` | Assertion below the credential — refused by design | Narrow the credential or assert `iam-owner` |
| `binding asserted org-owner but the credential is ORG_OWNER in N orgs` | Ambiguous org binding | Use a credential that is ORG_OWNER in exactly one org |
| `unable to read key file` | Credential Secret not mounted / wrong `keyFile` | Check `credentials.*` values and the Secret |
| `domain is required` | Empty `config.domain` | Set it |

## Condition taxonomy

### `Ready` (every CRD)

| Reason | Meaning | Action |
| --- | --- | --- |
| `Reconciled` (True) | In sync with Zitadel | — |
| `OrgNotReady` / `ProjectNotReady` / `UserNotReady` / `AppNotReady` / `IdpNotReady` / `GrantedOrgNotReady` | The referenced parent CR exists but is not Ready yet | Normal during bundle applies (requeues at 10s); persistent ⇒ inspect the parent |
| `SecretNotFound` | A referenced input Secret is missing | Create the Secret; the next reconcile proceeds |
| `InvalidSpec` | Mutually exclusive fields both set / required choice missing (e.g. `projectRef` **and** `projectId`) | Fix the spec |
| `NotSupportedAtBindingLevel` | Instance-level resource under an `org-owner` binding | Move the CR to an `iam-owner` operator, or upgrade credential + assertion. See [Binding levels](../install/binding-levels.md) |
| `DuplicateSingleton` | A second CR of a `Default*` kind; the earliest-created one wins | Delete the duplicate (or the winner, to hand over) |
| `CreateFailed` / `SyncFailed` / `KeyError` / `TokenError` | A Zitadel API call failed; message carries the API error | Permission / connectivity / conflict issue — read the message, check operator logs |
| `SmsFieldWarning` / `IgnoredFields` (warning conditions) | Fields not applicable to this type were set and ignored | Cosmetic; clean the spec |

### `ScopeResolved` (tenant CRs, scope maps active)

| Reason | Meaning | Action |
| --- | --- | --- |
| `Resolved` (True) | Namespace resolved; message names map/rule/org/delegate | — |
| `MapsNotSynced` | Informers still syncing (restart) | Transient (2s requeue). Persistent ⇒ operator cannot list maps: check that `operatorNamespace` is watched (it must be in `watchNamespaces`) and RBAC covers it |
| `NoMatchingRule` | Maps exist, none match this namespace | Add/extend a rule, or label the namespace. Expected for namespaces meant to be unserved |
| `ScopeConflict` | Rules in ≥2 maps match | Fix the overlapping selectors — check the Warning Events on **both** maps |
| `InstanceMismatch` | The only matching map asserts a different instance | Fix the map's `spec.instance`, or you applied a map to the wrong operator's namespace |
| `ScopeMapNotReady` | Matched map's organization not resolved yet | Transient. Persistent ⇒ inspect the map (org name typo? Zitadel unreachable?) |
| `DelegationFailed` | Delegate mint/refresh failed; message carries the cause | Usually permissions (binding cannot grant on that org/project) or Zitadel availability |

### `InstanceResolved` (tenant CRs, dual-serving)

| Reason | Meaning | Action |
| --- | --- | --- |
| `Pinned` (True) | `spec.instance` matches this operator | — |
| `Assumed` (True) | Unpinned, no other operator detected | — |
| `AmbiguousInstance` | Unpinned in a dual-served namespace; all operators stopped | Set `spec.instance`. See [Dual-serving](dual-serving.md) |

### Scope-map conditions (`ScopeMap`)

| Condition / Reason | Meaning |
| --- | --- |
| `InstanceMatch=False / InstanceMismatch` | Map's `spec.instance` ≠ operator domain; entire map fail-closed |
| `Ready=False / NotSupportedAtBindingLevel` | Map's org is foreign to an `org-owner` binding (see also the `ForeignOrganization` Event) |
| `OrganizationResolved` | Whether the org name/ID resolved; False is transient unless the name is wrong |

## Event taxonomy

| Event reason | On | Meaning |
| --- | --- | --- |
| `OrganizationNameDrift` | ScopeMap | `spec.organizationId` is authoritative and the org's real name differs from `spec.organization` — cosmetic drift, fix the name at leisure |
| `ForeignOrganization` | ScopeMap | Map for an org outside the `org-owner` binding — rejected |
| `ScopeConflict` (Warning) | both conflicting maps | A namespace matched rules in two maps |

## Symptom-driven

### A CR is not reconciled at all (no conditions, no finalizer, no Events)

1. **Foreign instance pin?** `spec.instance` set to another operator's instance identity ⇒ this operator ignores it entirely (by design).
2. **Namespace watched?** It must be in `watchNamespaces` (when set) — unwatched namespaces never enter the cache.
3. **RBAC?**
   ```bash
   kubectl auth can-i list oidcapps.zitadel.truvity.io -n <ns> \
     --as=system:serviceaccount:<operator-ns>:<operator-sa>
   ```
4. **Operator leader elected and running?** Check `kubectl -n <operator-ns> get lease` and the logs.

### CR stuck in `Terminating`

The finalizer blocks until Zitadel-side cleanup succeeds. Zitadel unreachable ⇒ it waits and retries; restore connectivity. To deliberately orphan the Zitadel resource, remove the finalizer manually. Deletion never deadlocks on missing scope maps/delegates — it falls back to the binding client.

### Everything in one namespace suddenly `NoMatchingRule`

Someone created the **first** scope map — that ends passthrough mode for every namespace the operator serves. Cover all served namespaces with rules before (or immediately after) the first map lands. See [rollout guidance](scope-maps.md#rollout-from-zero-maps-to-routed).

### Output Secret missing or stale

The Secret is created in the CR's namespace on first successful reconcile. Missing ⇒ the CR is not Ready (see its conditions). Deleted by accident ⇒ the operator re-creates it; for credentials it cannot read back (confidential client secrets, keys), it mints a fresh one exactly once.

### Zitadel-side edits keep reappearing

Working as intended: within a managed scope the operator is the single writer and reverts drift on the periodic (5-minute) reconcile. Change the CR spec, not the Zitadel console.

### Operator logs show delegation Secrets being re-minted repeatedly

The delegate SA in Zitadel and the Secret cache disagree — typically an out-of-band deletion loop (something in Zitadel deleting the delegate users, or something in the cluster pruning the labeled Secrets). Both are self-healing one-offs; recurring re-mints mean an external system is fighting the operator. Check `kubectl -n <operator-ns> get secrets -l zitadel.truvity.io/delegation` and the Zitadel event log for the deleter.

## Identifying delegation Secrets

Delegation Secrets are hash-named (`zitadel-delegation-<scope-hash>`) but
self-describing via annotations:

```bash
kubectl get secrets -n zitadel-operator -l zitadel.truvity.io/delegation \
  -o custom-columns='NAME:.metadata.name,ORG:.metadata.annotations.zitadel\.truvity\.io/scope-org,PROJECT:.metadata.annotations.zitadel\.truvity\.io/scope-project'
```

The full canonical scope identity is in the Secret's `scope.json` key.
