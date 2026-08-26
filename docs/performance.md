# Performance

k10s was, at one point, unusably slow: several seconds to start, and visible
input lag when moving between kinds. The causes are worth recording, because
each fix is easy to undo by accident.

## What went wrong

**1. The sidebar made live API calls on every frame.** `viewList` asks for a
badge count for all 16 kinds on every repaint. For custom resources that
meant one API `LIST` *per CRD*, sequentially, on the UI thread. On a cluster
running cert-manager, Argo and prometheus-operator that's dozens of
round-trips every few seconds — seconds of frozen UI, repeatedly.

**2. Startup watched the entire cluster.** All 16 informers were registered
up front, so starting k10s issued 16 cluster-wide `LIST`s — including every
Secret and every Event — before showing anything.

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

It runs 16× per frame. It reads only caches that are already running; for a
kind with no informer it returns `domain.CountUnknown` and lets the
background sweep supply a number instead. Starting a watch there would
reintroduce cause #2 by the back door.

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
