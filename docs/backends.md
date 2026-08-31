# Backends

Two implementations of `domain.Source` (see
[architecture.md](architecture.md#the-source-boundary-internaldomain)). The
UI cannot tell them apart.

`main.go` picks between them **by context name**, and when there is no
cluster it returns no backend at all plus the reason:

```go
// The demo is a context, not a mode: `k10s demo`, `/demo` and `:ctx` all
// just ask to connect to this name, and leaving it is picking another.
if domain.IsDemoContext(ctx) {
    return mock.New(ctx), "demo mode — sample data, not a real cluster · :ctx leaves"
}

store, err := k8s.NewStore("", ctx)   // ctx == "" → current kubeconfig context
if err != nil {
    return nil, noClusterReason(err)
}
if err := store.Ping(); err != nil {   // kubeconfig parsed, nobody answered
    store.Close()
    return nil, noClusterReason(err)
}
return store, ""
```

The UI then shows its **No cluster** panel (`internal/ui/nocluster.go`) —
the reason, three links, and `r` to retry. k10s always starts; what it never
does any more is stand in for the cluster it could not reach.

That fallback used to be `mock.New("")` — silently, for any failure — which
meant a machine with no kubeconfig opened onto forty pods, three nodes and a
CrashLoopBackOff that existed nowhere, labelled only by a line in the status
bar. The demo is now something you ask for by name.

`Ping` is the second half of "no cluster", and the less obvious one. Every
client-go handle is lazy: a context pointing at a deleted cluster, or one
behind a VPN that is down, builds a perfectly healthy `Store` that simply
never returns rows. `Ping` reports the one request `k8s.New` already makes —
the server version handshake — so that case lands on the panel instead of on
an empty table. A `401`/`403` is deliberately *not* unreachable: that is a
live cluster refusing this user, and k10s connects so they can see whatever
they are allowed to. See [cluster-setup.md](cluster-setup.md).

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

The same type also serves the settled no-cluster state, with `err` set to
`errNoCluster`: the same empty rows, a different verdict ("there is nothing
there" rather than "not yet"), so the two states cannot drift into different
answers for the same method. A connection that fails while a working cluster
is already on screen — a `:ctx` switch to one that is down — keeps what is
there and says the switch did not happen; only a startup with nothing behind
it becomes the panel.

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

## The demo as a context

`domain.DemoContext` (`"k10s-demo"`) is the one string the UI and `main.go`
must agree on, which is why it lives in `domain` — neither may import the
other's backend, and the UI never constructs a demo backend. It only ever
names the context and lets `Connect` decide.

Everything follows from that single decision:

| Reaching it | How |
|-------------|-----|
| `k10s demo` | `main.go` sets the startup context to `domain.DemoContext` |
| `/demo` | `switchContextCmd(domain.DemoContext)` |
| `:ctx` | the picker always lists it, labelled |
| leaving | pick any other context — there is no "exit demo" of its own |

`domain.IsDemoContext` matches the name and its `-` prefixed variants, so the
demo can serve several contexts (`k10s-demo-staging`, `k10s-demo-prod`) and
switching between *those* stays inside the demo. They are all named for what
they are: they used to be `prod-eu-west-1` and `gke-prod-asia`, which read
exactly like somebody's real clusters in a screenshot or a bug report.

Two rules keep it honest:

- **Crossing the boundary goes through `Connect`, never `SwitchContext`.**
  `Model.switchContextCmd` checks `IsDemoContext(target) != m.demoMode()`.
  Asking the demo backend to switch to `admin@tp3` would have it hand back a
  demo cluster wearing a real context's name.
- **The demo's own context list is not the way out.** `ctxChoices` merges
  the current backend's contexts with `Model.kubeCtxs` — kubeconfig's list,
  read once at startup — so the real contexts stay reachable from inside the
  demo, which is what "pick another context to leave" depends on.

`Model.demoMode()` is derived, not stored: it reads
`IsDemoContext(m.src.ClusterInfo().Context)`. There is no flag to go stale,
and the header's `DEMO` marker is on every frame the demo produces rather
than in one toast that scrolls away.

## Offline demo — `internal/mock`

Static, in-memory fake cluster: the same 30 kinds, three nodes (one
NotReady), and
rows seeded with deliberate trouble — `billing-worker` in CrashLoopBackOff
×17, a `payment-api` pod Pending on insufficient CPU, a Terminating pod — so
status colors and events have something real to point at.

It backs:
- `cmd/shot`, the headless renderer, which never needs a cluster;
- every UI test;
- the `k10s-demo` context, and nothing else at runtime. It is never reached
  by accident: no cluster means the No cluster panel, not sample data.

A resource's base `Rows` are implicitly namespace `default`; `Extra` holds
rows tagged with another namespace, which is what makes `:ns` switching show
genuinely different data. `argocd` / `cert-manager` only carry Custom
Resource instances.

Mutating methods change the in-memory data (delete removes the row, scale
rewrites READY, cordon flips the node's STATUS), so the UI's optimistic
paths can be exercised without a cluster. `Shell` and `PortForward` return
`nil, nil` — "not supported here" — and the UI falls back to a toast.
