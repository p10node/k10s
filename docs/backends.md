# Backends

Two implementations of `domain.Source` (see
[architecture.md](architecture.md#the-source-boundary-internaldomain)). The
UI cannot tell them apart.

`main.go` tries the live one first and falls back:

```go
store, err := k8s.NewStore("", ctx)   // ctx == "" → current kubeconfig context
if err != nil {
    return mock.New(""), "mock mode — " + err.Error()
}
```

so k10s always starts, and says in the status bar why it's offline.

That call is never made before the program starts. `main.go` hands it to
`ui.NewStartup` as `Startup.Connect`, and the UI runs it as a background
command once the event loop is up:

```go
ctxNames, curCtx := k8s.KubeContexts("")   // kubeconfig only — no request
m := ui.NewStartup(ui.Startup{
    Kinds: k8s.Kinds(), Contexts: ctxNames, Context: curCtx, Connect: newSource,
})
```

Until it lands, the model runs against `pendingSource` (`internal/ui/connect.go`),
a `domain.Source` that serves what kubeconfig already knows — the kind list,
the context list — and answers `errConnecting` to everything else. The main
panel shows a spinner meanwhile, so an API server behind a downed VPN, or an
exec credential plugin that stalls, leaves a working UI on screen instead of
a blank terminal. Picking a context in `:ctx` during that wait retargets
the connection rather than switching from a backend that isn't there yet;
each attempt carries a generation, so a slow first one landing later is
dropped instead of overwriting the newer one.

## Live cluster — `internal/k8s`

| File                 | Contents                                                |
|----------------------|---------------------------------------------------------|
| `client.go`          | kubeconfig loading, clientset/dynamic/discovery/metrics |
| `store.go`           | the Store, lazy informer registry, metrics polling      |
| `listers.go`         | lazy per-kind lister accessors                          |
| `kinds.go`           | the 30 kinds, their columns and allowed actions         |
| `rows.go`            | object → row formatting, sorting, namespace filtering   |
| `counts.go`          | cheap background counts for unopened kinds              |
| `describe.go`        | kubectl's own describers, generic fallback for CRs      |
| `yaml.go`            | live YAML get + apply (used by Edit)                    |
| `actions.go`         | delete / restart / scale / cordon / drain               |
| `logs.go`            | log tail/paging and `-f` follow                         |
| `shell.go`           | interactive exec streamed for in-panel rendering        |
| `exec.go`            | interactive exec as a `tea.ExecCommand`                 |
| `portforward.go`     | SPDY port-forward on an ephemeral local port            |
| `metrics_top.go`     | `kubectl top`-equivalent output                         |
| `customresources.go` | resolving a CR instance back to its GVR                 |

### Connection

`KubeconfigPath()` honours `$KUBECONFIG`, else `~/.kube/config`. Discovery
and version calls get a 10s timeout on a **copy** of the rest config —
applying it to the shared config would also cap the informers' long-lived
watch connections and kill them.

### Data

Table data comes from client-go informers, which keep a local cache warm off
a watch. Informers are started **lazily, per kind** — see
[performance.md](performance.md).

Adding a kind means touching five places, all of them switch-shaped so a
missing one fails loudly: `kinds.go` (entry, columns, allowed actions),
`store.go` (informer registration + its GroupVersionResource, which is what
YAML/edit/delete/badge counts all resolve through), `listers.go` (the lazy
lister accessor), `rows.go` (object → row, plus the `RowCount` case), and
`describe.go` (the GroupKind kubectl's describer is registered under). Then
mirror it in `internal/mock` — `TestBackendsServeTheSameKinds` fails if you
don't — and give it aliases in `internal/ui/commands.go`.

`Describe` uses kubectl's own describers (`kubectl/pkg/describe`) so output
matches `kubectl describe`, with a generic unstructured describer for CRDs
and custom resources. `YAML` goes through the dynamic client and strips
`managedFields`, like `kubectl get -o yaml` does by default.

### Actions

- **Delete** — dynamic client delete, namespaced or not as the kind requires.
- **Restart** — patches `kubectl.kubernetes.io/restartedAt` on the pod
  template, exactly what `kubectl rollout restart` does.
- **Scale** — patches `spec.replicas` directly rather than using the scale
  subresource: one request instead of two, and no dependency on that
  subresource being wired up.
- **Cordon** — patches `spec.unschedulable`.
- **Drain** — cordons, then evicts every pod on the node except DaemonSet-
  and mirror-pods, via the eviction API; reports which refused.
- **Apply** — used by Edit: reads the edited YAML, carries the live
  `resourceVersion` over, and updates.

### Streams

- **Logs** — `LogsTail(n)` returns the last n lines plus whether older ones
  remain, which is how the viewer pages backwards: the Kubernetes API has no
  backwards cursor, so "older" means re-reading with a larger tail.
  `LogsFollow` then streams new lines over a channel; the UI tags each with a
  generation number so a stale stream can't append into a view you've left.
  Workloads (Deployment, StatefulSet, DaemonSet, Job, ReplicaSet) resolve to
  one of their pods via the workload's label selector, preferring a running
  one. Kinds with no logs return `domain.ErrNoLogs`,
  which the UI treats as "show describe instead", not as a failure.
- **Exec** — `ShellSession` opens an interactive exec and hands back a
  writer for keystrokes plus a channel of raw output bytes, so the UI can
  render it inside a panel through a terminal emulator instead of taking
  over the whole terminal. `Shell` (the `tea.ExecCommand` form) is still
  there for a full-screen takeover. No `kubectl` binary is involved either
  way. Backends without one return `domain.ErrNoShell`.
- **Port-forward** — SPDY dialer on a free local port; returns the address
  and a stop function, so pressing `p` again tears it down.

### Custom resources

CRDs are informer-backed like anything else. Their *instances* are not:
their GVRs are only known at runtime, and listing them costs one API call per
CRD. That sweep runs on a background timer and the render path only ever
reads its cache.

## Offline demo — `internal/mock`

Static, in-memory fake cluster: the same 30 kinds, three nodes (one
NotReady), and
rows seeded with deliberate trouble — `billing-worker` in CrashLoopBackOff
×17, a `payment-api` pod Pending on insufficient CPU, a Terminating pod — so
status colors and events have something real to point at.

It backs:
- `cmd/shot`, the headless renderer, which never needs a cluster;
- every UI test;
- k10s itself when no cluster is reachable.

A resource's base `Rows` are implicitly namespace `default`; `Extra` holds
rows tagged with another namespace, which is what makes `:ns` switching show
genuinely different data. `argocd` / `cert-manager` only carry Custom
Resource instances.

Mutating methods change the in-memory data (delete removes the row, scale
rewrites READY, cordon flips the node's STATUS), so the UI's optimistic
paths can be exercised without a cluster. `Shell` and `PortForward` return
`nil, nil` — "not supported here" — and the UI falls back to a toast.
