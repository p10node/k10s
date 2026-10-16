# Expanded live verification — p10-k8s

Historical snapshot. The SPDY failure and legacy lint debt below have since been repaired and verified; see [closure of verification findings](portforward-repair.md) for the current result and final live rerun.

## Verdict

**Live fixture matrix: PASS on the final rebuilt binary. Overall fault-hardening acceptance: NOT ALL PASS.** A reproducible port-forward startup hang remains open. Canonical tests passing must not be used to erase this separate failing fault probe.

The user authorized temporary uniquely named CRDs, CR fixtures, pods and namespace-local RBAC. No real node drain was authorized or performed. Context was explicitly pinned to `p10-k8s`, server `https://192.168.20.152:6443`; kubeconfig current-context was not changed.

## Live results

All rows below passed on the final post-fix build, harness exit 0:

| Boundary | Evidence |
| --- | --- |
| ConfigMap list/watch/count | Real Store informer rows and count converge; namespace-injected ConfigMaps are retained, not removed to satisfy assumptions. |
| Optimistic concurrency | Stale ConfigMap and CR manifests receive real API Conflict; concurrent data survives; fresh edits persist and are independently read back. |
| CR identity | Two generated API groups each expose a Widget called `shared` in the same namespace. Store snapshot references, YAML, Describe, Apply and Delete target Beta; Alpha's data remains unchanged. |
| UID preconditions | Same-name ConfigMap and CR replacements survive delete attempts using the old UID. |
| Logs | Three distinct live fixture log lines arrive; history contains fixture output; Close terminates the stream. |
| Embedded shell | Real exec produces a marker that is not present contiguously in the input command; resize is accepted and Close ends the output channel. This does not assert the exact remote terminal dimensions. |
| Pod port-forward | HTTP through the real forwarding tunnel returns the fixture's marker; stopping it closes the local listener. Faulting startup is a separate failing probe below. |
| Namespace-local RBAC | A namespace ServiceAccount with only read/list/watch access can list its fixture ConfigMaps; cluster-wide list is Forbidden. No ClusterRole/ClusterRoleBinding was created. |
| Watch outage/recovery | A loopback proxy using that restricted identity cancels only its own watch requests and returns 503. Store surfaces degraded health, clears it after recovery, and observes a newly created marker. Cluster networking/API availability is untouched. |
| Actual terminal UI | Fresh application binary runs in a PTY through the restricted proxy: pod row, YAML action, logs display, Escape back to table and clean quit all pass. |

The nonprivileged pod used `busybox:1.37`, a bounded writable tmp volume, no mounted service-account token, dropped capabilities, read-only root filesystem and CPU/memory limits. The harness never launched shell commands in an existing workload.

Client-go emitted closed-network messages during deliberate shell cancellation. The assertions checked actual command output and channel termination. This is recorded, not described as a warning-free harness run.

## Independent review and repaired regressions

The independent reviewer did not mutate the repo or access the cluster. It identified two concrete UI composition gaps and a separate startup-timeout risk.

Both UI gaps were confirmed with failing tests before minimal fixes:

1. Leaving logs retained `m.logSession`. History-only demo logs could reuse the retired backend session. Teardown now clears the session and a new accepted log result assigns its session before the history-only branch. The regression switches to the actual mock/demo source and checks that history loads without calling the retired session.
2. `:ns <existing namespace>` did not invalidate pending views. Four controlled regressions showed stale describe content, stale logs reopening, old editor seed opening and active logs remaining after namespace change. Namespace changes now return to the table through the shared teardown/generation boundary.

New/focused regression runs with race detection and count 20 passed. The independent review preceded the repairs; the parent reproduced and verified the fixes. This is not a second independent review of the post-fix diff.

## Open failure: unbounded SPDY startup

`internal/k8s/portforward.go` constructs the SPDY dialer and waits for readiness/error without an effective startup deadline.

An owned loopback HTTP server answers the pod GET and then receives but never answers the upgrade request. With a 250 ms configured REST timeout, Store.PortForward was still blocked after the probe's 2-second watchdog and only returned once the probe closed its connection. This is an offline fault reproduction, not an outage injected into p10-k8s.

Trying `http.Client.Timeout` alone did not stop the stalled transport either. That ineffective production edit was reverted. The underlying SPDY transport performs blocking upgrade I/O; a proper repair must bound it, close the owned connection on failure and prove established tunnels survive past the startup budget, while preserving TLS/auth/proxy behavior.

The failing test is retained as a Go-overlay probe, outside the canonical passing suite. It is intentionally **not** marked fixed or silently skipped:

```
KUBECONFIG=/nonexistent go test \
  -overlay /Users/pierreneter/.hermes/backups/k10s-hardening-1cuy2_kw/live-p10-k8s-full/probes/overlay.json \
  ./internal/k8s -run '^TestPortForwardStartupHonorsConfiguredTimeout$' -count=1
```

Pre-repair result: exit 1, `SPDY startup ignored configured timeout and hung until test closed connection`. The probe closes its own connections and requires no cluster. Its canonical successor now passes after the repair linked above.

## Gates and limits

On the final source after UI repairs:

- `go test -race -count=1 ./...`: PASS, using `KUBECONFIG=/nonexistent`.
- `go vet ./...`: PASS.
- `golangci-lint run --new-from-rev=HEAD ./...`: 0 issues.
- `git diff --check`: PASS.
- Fresh app and live harness builds: PASS.
- Final expanded live harness: exit 0.

Also run in this verification session: broad UI lifecycle/history/editor/read-only regressions with race/count 20; drain fake/HTTP tests with race/count 3. No node was cordoned, evicted or drained. Full-repository legacy lint debt remains as documented previously.

This is bounded coverage, not proof of every Kubernetes operation or terminal interaction. Interactive editor-save/conflict recovery and rapid context switching were tested through deterministic model tests plus live backend mutation tests, not an exhaustive real-PTY action matrix. Restricted-RBAC CRD discovery, service-target port-forward, sustained load/resource-leak measurement, all external plugins and every provider/version were not live-certified. Release-artifact installation was not rerun after these UI-only fixes; its earlier evidence remains historical.

## Harness issues and cleanup

The first expanded PTY run recognized a log marker inside the pod YAML before the logs screen had appeared, then raced Escape/quit. The corrected harness waits for observable table/log-view transitions rather than fixed sleeps. Subsequent full runs and the final rebuilt run passed. No application change was made to accommodate that harness issue.

Across three expanded runs, **three namespace fixtures and six CRDs were deleted and independently confirmed absent** with explicit-context kubectl. Role, RoleBinding, ServiceAccount, pod and ConfigMap fixtures were inside those deleted namespaces. Cleanup checked random ownership labels and UID preconditions before deletion.

Durable evidence, both earlier attempts, final live/PTY logs, independent review, final source hashes, cleanup receipts, gate commands/exit codes, harness source and failing overlay probe:

`/Users/pierreneter/.hermes/backups/k10s-hardening-1cuy2_kw/live-p10-k8s-full/`

Production edits from this verification are limited to the two UI lifecycle fixes and their regressions. No commit or push was performed.
