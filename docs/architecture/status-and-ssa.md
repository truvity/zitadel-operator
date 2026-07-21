# Status Conditions & Server-Side Apply

## Status shape

Every CRD's status follows one pattern:

- `ready` (bool) plus printer columns for `kubectl get`;
- `conditions` — `metav1.Condition` list, `listType=map` keyed by `type` (`Ready`, `Synced`, and where applicable `ScopeResolved`, `InstanceResolved`, `InstanceMatch`, `OrganizationResolved`);
- resource identity fields (`organizationId`, `projectId`, `clientId`, `userId`, …) recording what the operator resolved or created — these make deletion and adoption independent of the routing that originally produced them;
- `lastSyncTime`, `observedGeneration` where relevant.

Condition reasons are stable, documented strings (see the [troubleshooting taxonomy](../operations/troubleshooting.md)); messages are human-readable detail.

## All status writes are Server-Side Apply

Every controller writes status exclusively via SSA (`applyStatus`), with the field manager

```
zitadel-operator/<instance identity>   # instanceAlias, default: domain
```

so each operator deployment owns only the fields (and condition entries) it writes. Because `conditions` is a `listType=map` keyed by `type`, two managers can each own their own condition entries on the same object without touching the other's.

### Why (the condition-wipe bug)

The pre-v0.18 model — read-modify-write `Status().Update()` — was proven to **silently wipe conditions**: a full-object update refreshes the object from the server and drops in-memory condition edits made by anyone else (the prototype lost its own `ScopeResolved` condition to an intervening finalizer update; a foreign controller's condition would die the same way). That model cannot support two writers at all, and two-writer status is exactly what [dual-serving](dual-serving.md) requires. SSA landed as the first commit of the v0.18 batch, before any feature work, and the regression is pinned by integration scenario S-210 (`TestSSA_ForeignManagerConditionSurvives`): a foreign field manager's condition survives the operator's own writes.

### Consequences

- **Third-party controllers can safely annotate operator-managed CRs** with their own conditions; the operator will not clobber them.
- **Field managers are identity.** Dual-serving detection scans `managedFields` for other `zitadel-operator/*` managers — renaming the domain in config changes the operator's identity on every CR it has touched.
- **Spec writes are unaffected** — finalizer updates still use regular updates; only the status subresource moved to SSA.

## Reconcile-trigger discipline (no hot loops)

- `GenerationChangedPredicate` on all controllers: status-only writes never trigger re-reconciliation; spec changes (generation bumps) do.
- Conditional writes: status is only applied when something actually changed.
- Periodic requeue every 5 minutes drives drift detection; transient waits (parent not ready, Secret missing) requeue at 10s; scope-informer sync waits at 2s.

## Leader election

Leader election is unconditional in design (default-on flag): even `replicas: 1` runs two pods during a rolling update, and two active reconcilers would double-mint delegated SAs and race Secret writes. All controllers, the delegation warm-start/GC, and the scope-map loader are leadership-gated. The lease ID comes from `--leader-election-id` (Helm: the release fullname) so co-located deployments hold distinct leases.
