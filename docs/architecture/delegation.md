# Internal Delegation & Credential Lifecycle

Once a namespace resolves to a scope, the operator does **not** reconcile it with its own (binding) credential. Instead it mints a scope-limited Zitadel service account per scope — the **delegate** — and reconciles every tenant CR in that scope with the delegate's key. The binding credential's only jobs are to mint, rotate, and revoke delegates.

Why: isolation stops being a promise of the routing code and becomes API-enforced by the credential in hand. A bug that routed a CR into the wrong scope would still hit Zitadel with a key that cannot act outside its own org/project.

## Minting: explicit create + explicit grant

Per scope (identified by a hash of `{instance, organizationId, project, projectId}`):

1. `AddMachineUser` — username `zitadel-operator-delegate-<scope-hash>`.
2. Grant, by scope shape:
   - **org scope** → `AddOrgMember(ORG_OWNER)` on the scope org;
   - **project scope** → the project is created first if the rule names one that does not exist (by the binding credential), then `AddProjectMember(PROJECT_OWNER)`. `PROJECT_OWNER` is sufficient for full application CRUD via the v2 Application API.
3. `AddMachineKey` — the key JSON is the delegate's working credential.

The operator never uses Zitadel's `ORG_PROJECT_CREATOR` role: the creator role does not make the SA owner of the projects it creates ([zitadel#10561](https://github.com/zitadel/zitadel/issues/10561)), so the pattern only ever appeared to work behind over-granted credentials. Explicit create + explicit grant, always.

One deliberate exception: **MachineUser identity operations in project-scoped namespaces** go through the binding client, because a `PROJECT_OWNER` delegate cannot manage org-level users. Role grants for those MachineUsers still go through the delegate. This is the same trust boundary as delegation minting itself.

## Persistence: Secrets-backed cache

Each delegate lives in one labeled Secret in the operator namespace:

- name `zitadel-delegation-<scope-hash>`, label `zitadel.truvity.io/delegation`;
- data: `key.json`, `scope.json`, `user_id`, `org_id`, `project_id`, `key_id`, `key_created`, plus rotation markers (`old_key_id`, `old_key_revoke_after`).

The key is persisted to the Secret **before** the in-memory cache — a crash between mint and cache cannot lose a live credential. On restart the manager warm-starts by listing Secrets by label; the first use of each warm delegate lazily re-validates it against Zitadel (out-of-band SA deletion ⇒ drop the Secret, re-mint).

## Rotation

Delegate keys rotate on an internal 90-day cycle with dual-key overlap: mint a new key → swap it into the Secret → revoke the old key after a 5-minute grace. Two keys coexist during the overlap, so in-flight reconciles never race a revocation. Minted keys carry a 1-year expiry purely as a backstop.

The same dual-key mechanics power the user-facing `MachineUser` surface: `spec.key.rotateAfter` (+ optional `rotationGrace`) rotates a MachineUser's key with the old/new bookkeeping visible in `status.keyId` / `status.previousKeyId` / `status.previousKeyRevokeAt`, and the fresh key swapped into the output Secret. CRs without `key.rotateAfter` never rotate (pre-v0.18 behavior).

## Revocation and garbage collection

- **Eager revoke on unmatch** — when a scope stops matching (rule removed, map deleted, labels changed), the scope-map controller triggers a sweep: the delegate SA is revoked in Zitadel, then its Secret deleted.
- **Periodic orphan GC** — every 10 minutes, the GC recomputes the live scope set (resolving every namespace) and sweeps delegates outside it, and finishes any pending key rotations. The sweep aborts without revoking anything while scope-map informers are unsynced — never destroy credentials on a partial view.
- **Deletion fallback** — if a tenant CR still holds a finalizer after its maps/delegates are gone, resolution falls back to the binding client during deletion so finalizers cannot deadlock. Corollary: a delegate must outlive the tenant CRs it serves — decommission tenant CRs first, then rules, then maps.

## Auditability: the actor proof

Because tenant reconciliation runs as the delegate, Zitadel's event log is the audit trail: `AdminService.ListEvents` filtered by editor shows exactly who did what. The invariant, proven by integration test S-226 against a real instance:

- every `project.application.*` event on a reconciled aggregate is authored by the **delegate**;
- zero are authored by the binding credential;
- `project.added` is authored by the **binding** — minting is the binding's job.

Production can assert the same invariant the same way.

## Lifecycle summary

```
scope resolved ──► Ensure(scope)
                    ├─ in-memory cache hit ──────────────► delegated client
                    ├─ Secret hit (warm restart) ─ lazy validate ─► client (or re-mint)
                    └─ mint: AddMachineUser + grant + key
                             └─ persist Secret THEN cache ─► client
rotation clock (90d) ─► new key → swap Secret → grace → revoke old
scope unmatch ─► eager revoke SA + delete Secret
orphan (10m GC) ─► revoke + delete
CR deletion after teardown ─► binding-client fallback (finalizers never deadlock)
```

Code: `internal/delegation/manager.go` (mint/persist/rotate), `internal/delegation/gc.go` (sweeps). Prototype evidence including the actor proof transcript: [research: v0.18 prototype findings](../research/proto-v018-findings.md).
