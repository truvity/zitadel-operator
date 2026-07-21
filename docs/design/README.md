# Design Records

Dated, decision-oriented records of why the operator is shaped the way it is. These are **historical documents**: they capture the state of a decision at its date and are not updated afterwards. The current-state description of the system lives in [`docs/architecture/`](../architecture/), which supersedes these records wherever they disagree.

| Record | Date | Status | Subject |
| --- | --- | --- | --- |
| [v1alpha2 redesign](2026-06-v1alpha2-redesign.md) | 2026-06 | Implemented (v0.11+) | INF-363: config-file-only operator, namespace+RBAC isolation, explicit resource hierarchy, no connection CRD, testing strategy |
| [Ecosystem repositories](2026-06-ecosystem.md) | 2026-06 | Implemented | Repo split (rbac-mapper, notify-relay, google-group-sync) and shared implementation/release conventions |
| [v0.18 scope maps & internal delegation](2026-07-v018-scope-maps.md) | 2026-07-21 | Implemented (v0.18) | INF-422: scope maps, binding levels, internal delegation, dual-serving, SSA status discipline, breaking config removal |

Exploratory evidence backing these decisions lives in [`docs/research/`](../research/):

- [Operator config patterns](../research/operator-config-patterns.md) — comparative survey (ArgoCD, Keycloak, ESO, Capsule, Crossplane) behind the ScopeMap CRD decision
- [v0.18 prototype findings](../research/proto-v018-findings.md) — prototype evidence, actor proof, SDK surface audit, INF-400 root cause
