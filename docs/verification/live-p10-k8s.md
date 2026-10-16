# Live backend verification — p10-k8s

Follow-up: [expanded live verification](live-p10-k8s-full.md) covers CRDs, pod streams, scoped RBAC, watch recovery and actual PTY interaction. Its initially open SPDY failure is now resolved in [repair and verification closure](portforward-repair.md); the passing narrow matrix here is not an overall acceptance verdict.

Follow-up to [integrated offline verification](correctness-hardening.md). The user explicitly selected context `p10-k8s` and authorized an isolated test namespace and fixtures, including edit/delete/conflict and cleanup. Existing workloads and node drain were excluded.

## Target and safety boundary

- Explicit context: `p10-k8s`; API server: `https://192.168.20.152:6443`.
- `/readyz` returned `ok` before mutation. The harness verified both context and expected server before creating anything. It did not change the kubeconfig current-context.
- Each run used a server-generated `k10s-live-` namespace, a random ownership label and recorded namespace UID. Cleanup checked the owner label and UID and deleted with a UID precondition, then waited for NotFound.
- Only synthetic ConfigMaps in owned namespaces were edited/deleted. No existing workload, node, CRD, role or cluster configuration was changed. No pods were launched.

## Results

Final harness exit: **0**. Real calls exercised the current `internal/k8s` Store methods, not just equivalent kubectl commands:

1. Scoped `Rows`, `SyncedFor` and `RowCount` converged on real informer data.
2. `YAML` fetched an edit seed; an independent client updated the same fixture. `Apply` using the stale seed returned a real Kubernetes Conflict, and readback confirmed the concurrent winner remained intact.
3. `Apply` with a fresh seed persisted the new data; direct API readback confirmed it.
4. Changing the manifest identity was rejected. Sibling UID, resourceVersion and data remained unchanged.
5. The informer observed fixture creation and deletion through Store.Delete.
6. `DeleteResource` deleted the selected ConfigMap using its UID. After same-name replacement, repeating the old reference returned Conflict; the replacement UID/data remained intact.
7. Both test namespaces were deleted and independently checked absent with explicit-context kubectl.

## Harness correction, not an application fix

The first run assumed that two created ConfigMaps meant exactly two total rows. It timed out and cleaned up. The successful run observed three total rows for two owned fixtures (and four for three): the cluster adds a ConfigMap to a new namespace. The corrected harness checks exact fixture names and compares RowCount against all displayed rows, without modifying or deleting the cluster-generated object to satisfy the test. No production source changed during this verification.

Namespaces, both confirmed absent:

- `k10s-live-65j4x` — first harness attempt.
- `k10s-live-dbzmf` — successful attempt.

## Evidence and limits

Harness source, both attempt logs and a source SHA-256 manifest:

`/Users/pierreneter/.hermes/backups/k10s-hardening-1cuy2_kw/live-p10-k8s/`

This establishes live backend evidence for ConfigMap list/watch/count, optimistic concurrency, identity drift rejection, deletion and UID preconditions. It does **not** establish interactive TUI behavior, CRD/GVR collision behavior on real custom resources, live log/shell/forward behavior, restricted-RBAC behavior, watch reconnection after a real network fault, or drain semantics. Those remain covered only by the previously recorded offline fixtures where applicable. No claim of full production certification is made.
