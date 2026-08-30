# Performance

k10s was, at one point, unusably slow: several seconds to start, and visible
input lag when moving between kinds. The causes are worth recording, because
each fix is easy to undo by accident.

## What went wrong

**1. The sidebar made live API calls on every frame.** `viewList` asks for a
badge count for every kind on every repaint. For custom resources that
meant one API `LIST` *per CRD*, sequentially, on the UI thread. On a cluster
running cert-manager, Argo and prometheus-operator that's dozens of
round-trips every few seconds — seconds of frozen UI, repeatedly.

**2. Startup watched the entire cluster.** Every informer was registered
up front, so starting k10s issued one cluster-wide `LIST` per kind —
including every Secret and every Event — before showing anything.

**3. Startup then blocked waiting for them.** A `WaitForCacheSync` with a
multi-second timeout ran before the first frame.

## The rules now

### The render path does no I/O

`View` runs on every keystroke. Anything that could block belongs on a
background goroutine, with the render path reading a cache. This is why
custom resources are swept on a timer, and why `RowCount` is forbidden from
starting anything.

### Informers are lazy, per kind

`Store.ensure(kind)` starts a kind's informer the first time that kind is
actually displayed, and returns immediately without waiting for sync. Opening
Pods watches pods — not secrets, not events. Lister access goes through the
accessors in `listers.go`, which call `ensure` for you.

Startup registers **nothing** and awaits nothing:

```
TestNewStoreReturnsFastWhenCachesNeverSync   startup: ~6µs
```

### `RowCount` must stay cheap and side-effect free

It runs once per *visible* sidebar row per frame. It reads only caches that
are already running; for a kind with no informer it returns
`domain.CountUnknown` and lets the background sweep supply a number instead.
Starting a watch there would reintroduce cause #2 by the back door.

```
BenchmarkRowsPods           488392 ns/op   794741 B/op   8018 allocs/op
BenchmarkRowCountAllKinds    33772 ns/op    81040 B/op     16 allocs/op
```

One full sidebar repaint costs ~28 allocations; formatting one screen of pod
rows costs ~8000. That gap is the invariant.

### Counts without watches

`counts.go` asks the API for a one-item page (`Limit: 1`) and reads
`ListMeta.RemainingItemCount`, so each kind costs a single tiny request no
matter how many objects exist. It runs every 30s on a background goroutine,
re-runs promptly when the namespace changes, and recovers from panics —
a badge is not worth crashing a session over.

While a newly-opened kind is syncing, `RowCount` deliberately keeps returning
the *swept* number rather than the lister's truthful `0`, so the badge
doesn't flicker to zero and back.

#### Counting fewer things

Kubernetes has no "count these twenty resources" call — no batch endpoint,
no aggregate. (The apiserver's own `/metrics` does carry
`apiserver_storage_objects` for every resource at once, but it needs a
`nonResourceURLs` grant most users don't have, is cluster-wide only, and the
page is megabytes; thirty `limit=1` LISTs are cheaper than one of those.) So
the work is in *asking for fewer of them*:

| | |
|---|---|
| **Only what's on screen** | The render path stamps every kind it draws a badge for (`noteInterest`); the sweeper counts those and forgets anything nobody has drawn for 90s. Folding a sidebar group therefore stops its requests — 30 kinds became ~12 asked about, and that is the single biggest cut. |
| **Never what's watched** | A kind with a running informer is skipped: its cache already knows the exact number, for free. |
| **At most 6 in flight** | `forEachBounded` caps concurrency, so a sweep is a trickle instead of thirty simultaneous LISTs landing at once. |
| **Refusals are remembered** | Forbidden / no-such-group / 404 puts the kind aside for 10 minutes instead of re-asking every 30s. On a namespace-scoped account, half these kinds answer 403 forever; asking again is noise in someone's audit log and nothing else. A timeout or a 500 is *not* a refusal — that retries next sweep. |
| **Unfolding is one sweep** | A newly wanted kind kicks the sweeper, debounced by 250ms, so opening a group of five costs one sweep and not five. |
| **The CR sweep waits its turn** | Custom resources cost a LIST *per CRD* — dozens on a cluster with cert-manager, Argo and prometheus-operator. That sweep now starts when the Custom Resources badge is drawn, not because the sidebar was drawn at all. |

Net effect on a 30-kind sidebar with the default folds: roughly a dozen
one-item requests every 30s, six at a time, dropping to near zero on a
locked-down cluster once the refusals are known.

Guarded by `counts_test.go`: `TestSweepOnlyCountsWhatTheSidebarDrew`,
`TestSweepForgetsKindsNoLongerOnScreen`, `TestSweepSkipsInformerBackedKinds`,
`TestForbiddenKindsAreNotReasked`, `TestTransientFailuresAreRetried`,
`TestSweepBoundsRequestsInFlight`, and `TestCustomResourceSweepWaitsUntilItIsShown`
in `perf_test.go`.

### Other trims

- Informer resync period is `0` — no event handlers are registered, so
  periodic resyncs would be pure overhead; freshness comes from the watch.
- Pod metrics are only polled once Pods has been opened.
- Custom-resource sweeps are cached and shared between `Rows` and `RowCount`.

## Regression guards

These exist specifically so the above can't silently regress:

| Test                                         | Guards                                   |
|----------------------------------------------|------------------------------------------|
| `TestNewStoreReturnsFastWhenCachesNeverSync` | startup never blocks on cluster I/O      |
| `TestRowCountStartsNoInformers`              | drawing the sidebar opens no watches     |
| `TestOpeningOneKindWatchesOnlyThatKind`      | viewing pods watches only pods           |
| `TestCustomResourceCountMakesNoLiveCalls`    | CR listing stays off the render path     |
| `TestRowCountDoesNotFormatRows`              | counting ≠ formatting (allocation ratio) |
| `TestRowCountStaysCheapPerKind`              | one repaint's 16 counts stay cheap       |
| `TestViewDoesNotBuildRowsForEveryKind`       | the sidebar uses RowCount, not Rows      |
| `TestKeypressLatency`                        | input stays responsive                   |
| `TestSilenceLoggingKeepsStderrClean`         | client-go never paints over the TUI      |

Run them alone with `just test-perf`, or the benchmarks with `just bench`.

## The UI side

The palette (`ctrl+p`) follows the same rule: it searches object names only
for kinds already loaded, and matches unloaded kinds by name. Searching
everything properly would mean watching everything — the exact cost this
page exists to avoid — so the palette footer tells the user which kinds are
in the reduced state rather than pretending otherwise.

The repaint tick is adaptive: 150ms while the visible kind is still syncing,
2s once it has.
