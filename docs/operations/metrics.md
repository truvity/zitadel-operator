# Metrics Reference

The operator serves standard controller-runtime Prometheus metrics on
`--metrics-bind-address` (default `:8080`, path `/metrics`). No custom metrics
are registered in v0.18 — the useful signals are the controller-runtime
families below, keyed by the `controller` label (one per CRD kind, e.g.
`oidcapp`, `scopemap`, `machineuser`).

## Families worth alerting on

| Metric | Type | What it tells you |
| --- | --- | --- |
| `controller_runtime_reconcile_errors_total` | counter | Reconcile attempts that returned an error. Permission-shaped delegation failures and fail-closed scope states are deliberately **not** errors (they are conditions with periodic re-checks), so a rising rate here means real trouble: Zitadel unreachable, unexpected API rejections, Secret write conflicts. |
| `controller_runtime_reconcile_total{result=...}` | counter | Reconcile volume by outcome (`success`, `error`, `requeue`, `requeue_after`). A sudden `requeue_after` surge on tenant controllers usually means many CRs sitting in a fail-closed state. |
| `controller_runtime_reconcile_time_seconds` | histogram | Reconcile latency; Zitadel API slowness shows up here first. |
| `workqueue_depth{name=...}` | gauge | Backlog per controller. Sustained non-zero depth = the operator cannot keep up (see [large installations](large-installations.md)). |
| `workqueue_queue_duration_seconds` | histogram | How long items wait before processing — early saturation signal. |
| `workqueue_retries_total` | counter | Rate-limited retries; pairs with reconcile errors. |
| `leader_election_master_status` | gauge | 1 on the active leader, 0 on standbys. Alert when the sum across pods of one deployment is ≠ 1 for more than a lease duration. |
| `certwatcher_read_certificate_errors_total` | counter | Only relevant if webhooks/TLS are added later. |
| `rest_client_requests_total{code=...}` | counter | Kubernetes API traffic; 409 spikes indicate write conflicts, 403 indicates chart RBAC drift. |

Standard Go process metrics (`go_goroutines`, `process_resident_memory_bytes`,
`go_gc_duration_seconds`) are exposed too and matter for the cache-memory
sizing discussed in [large installations](large-installations.md).

## Suggested starting alerts

```promql
# Real reconcile errors (fail-closed states are conditions, not errors)
sum(rate(controller_runtime_reconcile_errors_total[5m])) by (controller) > 0.1

# Controller backlog not draining
max_over_time(workqueue_depth[10m]) > 10

# Split or missing leadership per deployment
sum(leader_election_master_status) != 1
```

Scope-state visibility (how many CRs are fail-closed and why) is intentionally
a **conditions** concern, not metrics: `kubectl get <kind> -A -o json | jq`
over `status.conditions[] | select(.type=="ScopeResolved")`, or a
kube-state-metrics custom-resource-state config if you want it in Prometheus.
