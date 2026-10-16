# Correctness hardening: integration verification

This report covers the uncommitted hardening changes integrated into `main` on top of `227fed8d5381e11e6baa283db7d4c6d3bb5bfa2c`. It is not a release or live-cluster certification.

Subsequent user-authorized live backend testing on `p10-k8s` is recorded separately in [live-p10-k8s.md](live-p10-k8s.md). The offline-gate limits below describe this original run, not that later scoped verification.

Expanded historical results: [live-p10-k8s-full.md](live-p10-k8s-full.md). The subsequent [repair and verification closure](portforward-repair.md) resolves its SPDY fault probe and full-repository lint debt.

## Integrated changes

- H01/H03: asynchronous operations retain the source that admitted them; source leases defer retirement until operations finish. Session/view/request generations reject stale completions and confirmations, with cleanup for editor seeds, log streams, shells and forwards. Accepting a new backend invalidates requests started while its connection was pending, not just requests predating the connection attempt.
- H02: custom-resource rows carry GVR, scope, namespace, name and UID together. Filtering and selection preserve that reference. Describe, YAML, Delete and editor Apply use the selected reference. Legacy CR Delete without a reference fails closed; deletion supplies an API UID precondition.
- H04: edited manifests retain identity and the original resourceVersion. Conflicts are not retried using a freshly fetched version. Failed/stale edits preserve the local file and expose its recovery path.
- H05: drain preflights unmanaged-pod blockers (emptyDir pods are evicted; that data is ephemeral by definition), skips mirror/DaemonSet pods, uses UID-preconditioned eviction, retries PDB throttling, evicts pods concurrently and waits for original UIDs to disappear. A bounded deadline covers the operation. Post-cordon failures report partial state; no forced DELETE or automatic uncordon.
- H06: a log session fixes its pod/container, combines initial tail and follow, checks the pod UID before history reads, and closes on leaving the view. Retained text and history responses have explicit bounds. History uses ordered overlap rather than length subtraction or set deduplication.
- H07: action resolution works before informer warm-up; CR sweeps follow the selected namespace and reject stale-scope results; denied count queries are scoped.
- H08: watch health is distinct from historical initial sync. Watch errors/disconnection and successful re-establishment update health. Namespace informer factories use bounded retention with per-scope cancellation and shutdown ownership; cluster-wide caches remain a separate fixed set.
- H09: read-only mode blocks every external plugin, regardless of its `dangerous` declaration.
- H10: updater requires a valid checksum manifest, verifies gzip trailer integrity, and release CI requires the freshly built artifact identity before publishing.
- H11: documentation describes disabled AI, current search scope and the compatibility/safety changes above without marking the larger feature backlog complete.

## Integrated regressions

Two bugs appeared at the boundaries between independently implemented lanes and were reproduced as assertion failures before the integration fixes:

- `TestComposedCustomResourceEditKeepsReference`: editing beta then changing the selection to alpha must still call beta's typed Apply. Before the fix, the UI reported success without calling the identity-aware API.
- `TestSourceAcceptanceInvalidatesInterimRequests`: a fetch admitted on the old backend while a switch is pending must not reopen its pane after the new backend is accepted.

The initial pinned-log path was also reconciled with the generation/source-lease protocol; `TestUILogsUsePinnedSessionForStreamAndHistory` now passes on the integrated tree.

## Executed gates

All commands below exited zero against the integrated source. Test configuration used a nonexistent kubeconfig and temporary configuration/plugin paths; Kubernetes requests in tests use fakes or loopback HTTP fixtures.

| Gate                                                                                                          | Result                                   |
|---------------------------------------------------------------------------------------------------------------|------------------------------------------|
| `just check` (format, vet, complete tests)                                                                    | PASS                                     |
| `go test -race -count=1 -cover ./...`                                                                         | PASS                                     |
| `just test-perf` without race instrumentation                                                                 | PASS; original thresholds retained       |
| UI generation/stale/context/action/identity/composition regression selection, `-count=20`                     | PASS                                     |
| `golangci-lint run --new-from-rev=HEAD ./...`                                                                 | PASS, 0 issues                           |
| `go build` to an owned temporary path and `--version`                                                         | PASS                                     |
| Headless demo, `j,j,d`, at 140x44, 100x30 and 80x24                                                           | PASS                                     |
| Fresh host-platform release archive from a disposable copy                                                    | PASS                                     |
| Required artifact installation, exact version/commit check, loopback server and temporary install destination | PASS; synthetic `v99.0.0`, not published |
| `git diff --check`                                                                                            | PASS                                     |

A negative control in the disposable copy deliberately replaced the editor's captured source with `m.src`. The composition regression failed with `edit lost original GVR`. Restoring the source made the same test pass. The canonical source was never sabotaged.

The source manifest was checked unchanged through the main gate run. Raw command output, per-gate exits, source SHA-256 manifest and negative-control evidence are retained at:

`/Users/pierreneter/.hermes/backups/k10s-hardening-1cuy2_kw/verification/`

## Worktree reconciliation

All tracked/untracked lane files were checked for counterparts in the canonical checkout. Overlapping files were merged rather than copied over parent work. Differences remaining between lanes and `main` were integration changes, regression fixes, formatting and targeted lint corrections.

Before removal, each worktree was archived with its dirty/untracked/ignored files, including the updater's ignored `dist` artifacts. Every archived regular file was read back and its SHA-256 checked. Receipts and archives are in:

`/Users/pierreneter/.hermes/backups/k10s-hardening-1cuy2_kw/`

The four temporary worktrees and their baseline-only branches were removed. `git worktree list` now contains only the canonical `main` checkout. Changes remain uncommitted, intentionally; there was no commit, push or release.

## Post-review fixes (2026-10-16)

An automated review of the integrated diff reported nine findings. All were fixed on `fix/correctness-hardening` before merge, each behavioral change with a regression test:

- Pinned log sessions read lines up to 1 MiB, the historical `LogsFollow` budget. The initial 64 KiB limit ended the follow stream and history reads on a single long JSON or stack-trace line.
- The by-name custom-resource lookup behind `Describe`, `YAML` and `Apply` reports an ambiguous name, a failed lookup and a missing object as distinct errors instead of "not found" for all three.
- A watch closed by the reflector's own `Stop`, or by shutdown, no longer arms the reconnect-grace degrade timer; only an upstream close the reflector did not request counts as a disconnect.
- `goOffline` installs its placeholder source through the lease owner, so a later successful connect retires the source actually on screen.
- Pressing port-forward while a dial is in flight cancels that dial instead of starting a second one; the orphaned result is stopped when it lands.
- Removed dead work: the per-namespace pod-metrics purge on scope eviction (the metrics tick replaces the map wholesale), the duplicate CR sweep scheduling on the render path, and a second copy of the row-search predicate. Custom Resources rows under `:ns all` now sort namespace-then-name like every other kind.

Gates rerun after the fixes, all clean: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`, `golangci-lint run --new-from-rev=HEAD ./...` (0 issues), `gofmt -l`, `git diff --check`.

## Explicit limits

- Full-repository golangci-lint is **not clean**. With output caps disabled it reports 70 findings; the baseline checkout reports 71. Diagnostic-set differences include byte-identical, unchanged AI files, so raw totals are not a precise issue-by-issue proof. The changed-lines lint gate passes. This work did not bulk-reformat or suppress the repository's existing lint debt.
- No live Kubernetes cluster, real apiserver, production data or installed user binary was modified. Headless rendering is not live-terminal/cluster validation.
- The artifact executed here is the host `darwin/arm64` build. This does not establish runtime behavior on the other release platforms.
- Kubernetes logs do not provide exactly-once delivery or an atomic UID-conditioned log read. UID checks reduce replacement confusion but do not turn separate API requests into an atomic operation. Retention bounds cover retained payload, not total process RSS.
- Checksum verification is integrity relative to the downloaded manifest, not independent signature verification.
- The integrated diff received an owner review and composition/negative-control tests. The only later pass was the automated review recorded under Post-review fixes; no human reviewer independent of the author has read the integrated diff.
