# Execution plan

The k10s backlog as **task cards for worker agents**. Each card carries its
own context: an agent that reads the card plus the
[Worker contract](#worker-contract) section has enough to do the work without
asking back.

Dispatcher: pick a card → paste the [prompt template](#prompt-template) with
`<ID>` → review the diff → tick `[x]` on the [Board](#board).

---

## Worker contract

Read this **before** touching any file. These are the repo's invariants;
breaking one means red CI or a UI that fails silently.

### Architecture

- `internal/domain` is the only boundary between the UI and the backends.
  `internal/ui` **must not** import `internal/k8s` or `internal/mock`.
- **Do not add methods to `domain.Source`.** The interface already has 25
  methods and 4 implementations (`k8s.Store`, `mock.Source`, the stubs in
  `internal/ui/*_test.go`, `connect_test.go:pendingSource`). New capabilities
  go through an **optional interface**:

  ```go
  // internal/domain/domain.go
  //
  // Containers lists the containers of a pod-bearing object. Backends that
  // cannot answer simply do not implement it; the UI then hides the picker.
  type Containers interface {
      Containers(kind, ns, name string) ([]string, error)
  }
  ```

  The UI type-asserts and degrades silently when it is missing:

  ```go
  if c, ok := m.src.(domain.Containers); ok { … }
  ```

  Both `internal/k8s` **and** `internal/mock` must implement it (otherwise the
  demo backend and `just shot` never see the feature, and the UI cannot be
  reviewed).

### Performance (guarded by tests; a failure is a real regression)

- The render path **must not do I/O**. Guards: `TestViewDoesNotBuildRows`,
  `TestKeypressLatency` in `internal/ui`.
- `RowCount` **must not format rows or open watches**. Guards:
  `TestRowCount`, `TestNewStoreReturnsFast`.
- Informers are lazy per kind. Startup registers **zero** watches. Do not add
  anything to the startup path that calls the API server.
- Read `docs/performance.md` before touching `internal/k8s/store.go` or
  `counts.go`.

### Render

- **Every rendered line must be exactly the terminal width.** Overlays and
  joins rely on it. Guard: `internal/ui/block_test.go`.
- Do not nest a style inside another style's `Render` (the inner reset
  swallows the outer background) → use `padBG` plus sibling runs.
- Verify the UI without a TTY: `just shot 140 44 "j,j,d"`.

### Config

`internal/config/config.go` uses a **hand-written YAML parser**: flat, one
level of nesting, **no list-of-map support**. Adding a field means changing
**both** `render()` **and** `parse()` **and** adding a test case. Anything more
structured (saved views, for example) → its own file under `~/.k10s/`; do not
cram it into `config.yaml`.

### Adding a resource kind

Follow the 5 steps in `docs/dev.md` § "Adding a resource kind" exactly.
Skipping step 5 (mirroring into `internal/mock/data.go`) leaves the demo
backend out of step with live.

### Definition of done

A card is done only when **all** of these hold:

1. `just check` is green (= `fmt-check` + `vet` + `test`).
2. New tests cover exactly the behavior the card describes. Backend tests use
   the fake clientset (`newTestStore`); remember `syncKinds(t, s, kPods, …)`
   before asserting rows.
3. The feature is visible on the demo backend → paste the `just shot` output
   into the PR.
4. Docs updated: `docs/keybindings.md` for a new key, `docs/commands.md` for a
   new command, `docs/config.md` for a new config key, the README feature
   table if users can see it.
5. Tick this card in `docs/plan.md` § Board.
6. Do not touch files outside the card's "Files". If you need to → note it in
   the PR; do not widen the scope yourself.
7. **Single-card mode** (one agent, one card): do not commit; leave the diff
   in the working tree. **Lane mode** (5 agents in parallel): one commit per
   card on the lane's branch, no push — see § Dispatch protocol.

### Do not

- Do not change the `domain.Source` signature.
- Do not add a dependency when the stdlib or an existing one can do the job.
  `go.mod` currently has only bubbletea/lipgloss/bubblezone/vt10x/client-go.
- No "while I'm here" refactors.
- No commit, no push, no tag. Leave the diff in the working tree.

---

## Board

### P0 — trust & correctness

- [ ] **T01** kind-cluster e2e in CI
- [ ] **T02** container picker for logs / shell / top
- [ ] **T03** port-forward: pick container + port
- [ ] **T04** events of the selected object
- [ ] **T05** CLI flags
- [ ] **T06** read-only mode + prod context guard

### P1 — daily driver

- [ ] **T07** sort by column
- [ ] **T08** multi-select + bulk action
- [ ] **T09** logs: grep / previous / timestamps / save
- [ ] **T10** port-forward manager
- [ ] **T11** secret decode
- [ ] **T12** saved views
- [ ] **T13** label / field selector

### P2 — differentiator

- [ ] **T14** owner tree (xray)
- [ ] **T15** AI v2 — stream + auto-context + redact
- [ ] **T16** pulse dashboard
- [ ] **T17** `can-i` / RBAC introspection
- [ ] **T18** diff before apply
- [ ] **T19** helm releases
- [ ] **T20** ephemeral debug container + node shell

### P3 — security / polish / distribution

- [ ] **T21** cosign verify for self-update
- [ ] **T22** API key in the OS keychain
- [ ] **T23** custom keybindings + custom columns
- [ ] **T24** export CSV/JSON + clipboard OSC 52
- [ ] **T25** packaging: brew / scoop / nix
- [ ] **T26** missing kinds

### Lanes

Five lanes, one agent per lane, each lane owning one region of files. Cards
within a lane run **sequentially** (dependencies); lanes run **in parallel**.

| Lane | Owns | Card order |
|---|---|---|
| **A** cluster backend | `internal/k8s/`, `internal/mock/`, `.github/workflows/e2e.yml` | T01 → T13 → T26 → T03 → T10 → T17 → T19 |
| **B** table & render | the table drawing path in `internal/ui/view.go` | T07 → T08 → T18 → T24 |
| **C** streams | logs / exec / shell / port-forward streams | T02 → T09 → T20 |
| **D** shell, config, release | `main.go`, `internal/config/`, `internal/update/` | T05 → T06 → T22 → T12 → T23 → T21 → T25 |
| **E** new views & AI | new files (`pulse.go`, `treeview.go`), `internal/ai/` | T04 → T11 → T16 → T14 → T15 |

Cross-lane dependencies — there are three, and all three sit at the **end**
of the lane that needs them:

```
C:T02 (container picker)   →  A:T03, A:T10
B:T07 (sort state)         →  D:T12, D:T23
D:T06 (action hiding)      →  A:T17
```

A card blocked on an unmerged dependency → **skip it, do the next card,
report it as deferred**. Never implement another lane's dependency yourself.

### Dispatch protocol

Five agents on one repo will collide in `model.go`, `domain.go`,
`actions.go`. The rules that keep that from becoming chaos:

1. **One branch per lane**: `lane/a-backend`, `lane/b-table`,
   `lane/c-streams`, `lane/d-shell`, `lane/e-views`. Created from `main`.
2. **One commit per card**, message `T07: sort by column`. No push, no tag.
   (This is where lane mode differs from single-card mode in § Definition of
   done.)
3. **Rebase onto `main` before every new card.** A lane's next card must sit
   on top of whatever the other lanes just merged.
4. **Shared files — append only, never reorder:**
   - `internal/domain/domain.go`: new interfaces **appended at the end of the
     file**, one block per card, with a card-ID comment. Do not modify
     `Source`.
   - `internal/ui/model.go`: new fields **appended at the end of the `Model`
     struct**, one block per card. Do not reorder existing fields.
   - `internal/ui/actions.go`: append to the end of the `Actions` slice.
   - `internal/mock/`: append. Do not modify existing rows (other tests
     assert them).
   - No `go mod tidy`, no `go.mod` changes, unless the card says so.
5. **Merge order**: `C → B → D → A → E`. This is the topological order of the
   three cross-lane dependencies above — A merges after D because `A:T17`
   needs `D:T06`.
6. A lane that touches a file outside its region → stop and report to the
   dispatcher.

---

# P0

## T01 — kind-cluster e2e in CI

**Effort** M · **Deps** none · **Lane** A

**Goal** — CI runs the real k10s against a `kind` cluster, not only the fake
clientset.

**Why** — `docs/roadmap.md` § Known limits admits it: "Untested against a real
cluster by its author". The whole live path (`internal/k8s`) is tested only
with `k8s.io/client-go/kubernetes/fake`, which cannot catch RBAC, discovery,
API version drift, SPDY or exec. This is the repo's number-one risk — every
later card rests on the assumption that "the live path works".

**Files**

- `.github/workflows/e2e.yml` (new)
- `internal/k8s/e2e_test.go` (new, build tag `//go:build e2e`)
- `Justfile` — add a `test-e2e` recipe
- `docs/dev.md` § Tests

**Design**

- `helm/kind-action@v1` brings up the cluster and applies a fixture manifest
  (`internal/k8s/testdata/e2e.yaml`: 1 Deployment with 2 replicas, 1 Service,
  1 ConfigMap, 1 Secret, 1 Job, 1 CrashLoopBackOff pod).
- Tests carry the `e2e` build tag → a plain `just test` does **not** run them;
  CI calls `go test -tags e2e ./internal/k8s/...`.
- Cover, one test each: `NewStore` real connect · `Rows` for ≥8 kinds ·
  `RowCount` · `Describe` · `YAML` · `LogsTail` · `LogsFollow` (receive ≥1
  line, then `stop()`) · `Scale` · `Restart` · `Delete` · `PortForward` (dial
  the local address, then stop) · `ShellSession` (run `echo hi`, read it
  back).
- kind has no metrics-server by default → `TopPod`/`TopNode` must fail
  **gracefully**; assert the exact error, not a panic.

**Accept**

- [ ] `just test` (untagged) still needs no cluster and takes the same time.
- [ ] `go test -tags e2e ./internal/k8s/...` is green on kind.
- [ ] The workflow runs on push to `main` + PRs, matrix of ≥2 k8s versions
      (oldest supported + latest).
- [ ] Any path that panics against a real API server → red test, not a dead
      TUI.
- [ ] `docs/roadmap.md` § Known limits drops the "Untested against a real
      cluster" line.

**Not** — no production code changes unless e2e finds a real bug; a found bug
becomes its own card.

---

## T02 — container picker for logs / shell / top

**Effort** M · **Deps** none · **Lane** C

**Goal** — on a multi-container pod the user picks the container instead of
being forced onto `Containers[0]`.

**Why** — `internal/k8s/logs.go:18 podContainer()` always returns
`p.Spec.Containers[0].Name`. With istio-proxy / vault-agent / linkerd
sidecars, `Containers[0]` is usually **not** the app → users read the wrong
container's logs without knowing. A silent bug, worse than a crash.

**Files**

- `internal/domain/domain.go` — optional interface `Containers`
- `internal/k8s/logs.go`, `internal/k8s/shell.go`, `internal/k8s/exec.go`,
  `internal/k8s/metrics_top.go`
- `internal/mock/source.go`, `internal/mock/data.go`
- `internal/ui/pickers.go`, `internal/ui/pickers_view.go`, `internal/ui/model.go`
- `docs/keybindings.md`

**Design**

```go
type Containers interface {
    // Containers lists container names of the pod this object resolves to,
    // init and ephemeral containers included, app containers first.
    Containers(kind, ns, name string) ([]string, error)
}
type LogsInContainer interface {
    LogsTailIn(kind, ns, name, container string, n int) ([]string, bool, error)
    LogsFollowIn(kind, ns, name, container string) (<-chan string, func(), error)
}
```

- 1 container → **no** picker; behavior exactly as before. This is the
  majority case; do not make them press enter one more time.
- ≥2 containers → popup picker reusing the `ctxpicker.go` pattern (already has
  filtering, already clickable). The chosen container is remembered per
  `(ns,pod)` in `Model`, not persisted to config.
- The log panel title shows `logs -f <pod> · <container>`.
- `c` cycles containers inside the log view without closing and reopening.

**Accept**

- [ ] Pod with 3 containers → `l` opens the picker; picking the 2nd shows its
      logs.
- [ ] Pod with 1 container → `l` opens the logs directly, no popup.
- [ ] `s` (shell) and `m` (top) share the chosen container.
- [ ] Backend without `Containers` → no picker, no crash.
- [ ] The mock backend has a multi-container pod so `just shot` can demo it.

**Tests** — `internal/k8s/logs_test.go`: a 3-container pod, assert the order
and that `LogsTailIn` targets the right container. `internal/ui`: assert the
picker opens only when ≥2.

**Not** — no merged multi-container tail (`--all-containers`); separate card.

---

## T03 — port-forward: pick container + port

**Effort** S · **Deps** T02 (reuses the picker) · **Lane** A

**Goal** — forward any port, not only the first port of the first container.

**Why** — `internal/k8s/portforward.go:76` hardcodes
`p.Spec.Containers[0].Ports[0].ContainerPort` and skips any pod whose
`Containers[0]` declares no port (line 73) — a pod with a mesh sidecar almost
always lands there. A multi-port Service also forwards only its first port.

**Files** — `internal/k8s/portforward.go`, `internal/domain/domain.go`,
`internal/mock/source.go`, `internal/ui/pickers.go`

**Design**

- Optional interface `PortSource { Ports(kind, ns, name string) ([]Port, error) }`,
  `Port{Container, Name string; Number int32; Protocol string}`.
- Collect ports from **every** container, not only `[0]`.
- 1 port → forward immediately. ≥2 → picker showing `container · name · number`.
- Allow a specific local port: `p` = auto (:0), `shift+p` = prompt for the
  local port.
- Service: resolve through endpoints → pod, keeping current behavior but
  listing all of the service's ports.

**Accept**

- [ ] Pod whose `Containers[0]` has no port but `Containers[1]` does →
      forwards.
- [ ] Pod with 3 ports → picker; the chosen one is what gets forwarded.
- [ ] Requested local port already taken → clear error, no hang.

---

## T04 — events of the selected object

**Effort** S · **Deps** none · **Lane** E

**Goal** — `shift+e` on any row → only that object's events, newest first.

**Why** — real Kubernetes debugging starts at events ("FailedScheduling",
"ImagePullBackOff", "OOMKilled"). Today `:ev` lists the whole namespace and
the user scans the OBJECT column by hand. The backend already has it:
`internal/k8s/rows.go:605` reads `e.InvolvedObject`; only the filter and the
UI entry point are missing.

**Files** — `internal/k8s/rows.go`, `internal/domain/domain.go`,
`internal/ui/actions.go`, `internal/ui/model.go`, `internal/mock/extra.go`,
`docs/keybindings.md`, README

**Design**

- Optional interface `ObjectEvents { EventsFor(kind, ns, name string) (cols []string, rows [][]string, err error) }`.
- Match on `InvolvedObject.Kind` + `Name` (+ `UID` when present, so an old
  same-named pod does not leak in).
- Deployment → include events of the ReplicaSets and Pods it owns. This is
  the real difference from `kubectl describe`: a Deployment's failure almost
  always lives on a Pod.
- Add action `{domain.AEvents, "E", "Events", "󰀦", false}` for **every** kind.
- Warning events colored `theme.Danger`; newest first.
- Empty → "no events for this object in the last hour" (events have a TTL),
  not an empty table.

**Accept**

- [ ] `shift+e` on a pod → only that pod's events.
- [ ] `shift+e` on a deployment → events of the deployment + RS + pods.
- [ ] Object without events → message explaining the TTL.
- [ ] The action shows in the pane for every kind.

---

## T05 — CLI flags

**Effort** S · **Deps** none · **Lane** D

**Goal** — `k10s -n kube-system --context prod po` opens straight at the right
place.

**Why** — `main.go:40` accepts only `update` / `version` / `help`. Not
scriptable, not aliasable, unusable in a tmux layout. Every Kubernetes TUI has
this, and it is what blocks users before they see any other feature.

**Files** — `main.go`, `internal/ui/model.go` (`Startup` struct),
`docs/install.md`, README § Install

**Design**

Use the stdlib `flag`; **do not** add cobra.

```
k10s [flags] [resource]

  -n, --namespace   namespace to open with ("all" works too)
      --context     kube context, overrides current-context
      --kubeconfig  kubeconfig path
      --theme       theme for this session, not written to config
      --readonly    hide every destructive action (see T06)
  -h, --help  -v, --version
```

- The positional arg is a resource alias and goes through the same
  `kindForAlias` (`internal/ui/commands.go:182`) that `:po` uses — one source
  of truth.
- Precedence: flag > env (`KUBECONFIG`) > config.yaml > kubeconfig
  current-context. Put this table in `docs/config.md`.
- Flags **do not** write to the config file. They override the running
  session.
- Unknown alias → print the list of valid aliases and `exit 2`; do not open
  the TUI and complain afterwards.

**Accept**

- [ ] `k10s -n kube-system po` opens Pods in kube-system.
- [ ] `k10s --context nonexistent` reports clearly, exit 1, no TUI.
- [ ] `k10s --theme dracula` does not modify `~/.k10s/config.yaml`.
- [ ] Bare `k10s` behaves exactly as before.
- [ ] `--help` lists every flag.

---

## T06 — read-only mode + prod context guard

**Effort** M · **Deps** T05 · **Lane** D

**Goal** — no accidental production delete from a single click.

**Why** — `grep readonly internal/` → 0 hits. k10s sells "clickable" as its
strength, but `D` delete and `u` drain are clickable too, one pixel from the
row you meant. A mouse-first TUI without a brake makes an accident only a
matter of time. k9s has had `readOnly` for ages; this is the counterweight to
the repo's headline feature.

**Files** — `internal/config/config.go`, `internal/ui/actions.go`,
`internal/ui/model.go`, `internal/ui/view.go`, `internal/plugin/plugin.go`,
`main.go`, `docs/config.md`, README

**Design**

New config (remember to change **both** `render()` and `parse()`):

```yaml
readonly: false
danger_contexts: "prod,production,*-prod"   # glob, matched against the context name
```

- Read-only → `Risky` actions (`ADelete`, `ADrain`) plus `AEdit`, `AScale`,
  `ARestart`, `ACordon` disappear from the Actions pane; their keys become
  no-ops with a "read-only mode" toast. **Hidden entirely**, not
  shown-then-refused — the pane is the contract "this is what you can do".
- Plugins with `dangerous: true` are blocked too. If a plugin can bypass it,
  read-only is decoration.
- A context matching `danger_contexts` → a persistent red banner in the
  header, and the confirm modal of destructive actions requires **typing the
  object's name**, not just enter.
- `--readonly` enables it for the session; `:ro` toggles within the session
  (only when config does not force `readonly: true`).

**Accept**

- [ ] `--readonly` → `D` does not delete; the Actions pane does not show
      Delete.
- [ ] Context `prod` → red banner; delete requires typing the name.
- [ ] Plugin `dangerous: true` is blocked in read-only.
- [ ] Config `readonly: true` → `:ro` cannot turn it off.
- [ ] `just shot` captures both states.

---

# P1

## T07 — sort by column

**Effort** M · **Deps** none · **Lane** B

**Goal** — click a header or `shift+<n>` to sort; `RESTARTS` descending is
one click.

**Why** — today there is only natural A→Z (`internal/k8s/rows.go:87
sortRows`). The most common questions — "which pod restarts most", "which
node has the highest CPU", "what just changed" — are all column sorts and
none of them work.

**Files** — `internal/ui/view.go`, `internal/ui/model.go`, `internal/domain/domain.go`

**Design**

- Sort in the **UI**, not the backend. The backend keeps its stable order;
  the UI sorts the filtered copy. Sorting in the backend would touch the
  cache and `RowCount`.
- State: `sortCol int` (-1 = default), `sortDesc bool`, remembered **per
  kind** (`map[string]sortState`) — a Pods sort should not apply to Services.
- Click 1 = asc, click 2 = desc, click 3 = back to default.
- The comparator infers the type from the cell value: numbers (`RESTARTS`,
  `CPU`), durations (`AGE`: `5d2h`), quantities (`100Mi`, `2Gi`), otherwise
  `domain.NaturalLess`. Write it as `domain.CompareCell(a, b string) int` so
  it can be tested on its own.
- The sorted header shows `▲`/`▼`, with a bubblezone zone per header.
- Events keep newest-first as the default, as today.

**Accept**

- [ ] Click `RESTARTS` twice → highest first.
- [ ] `AGE` sorts by real time, not string (`10m` < `2h` < `3d`).
- [ ] `2Gi` > `900Mi`.
- [ ] Switch kind and back → that kind's sort is kept.
- [ ] `TestViewDoesNotBuildRows` stays green.

**Tests** — a table test for `CompareCell` covering: numbers, durations,
quantities, strings, empty cells, `<none>`.

---

## T08 — multi-select + bulk action

**Effort** M · **Deps** T07 · **Lane** B

**Goal** — `space` marks several rows; one `D` handles the whole set.

**Why** — cleaning up 12 Evicted pods is currently 12 × `D` + 12 confirms.

**Files** — `internal/ui/model.go`, `internal/ui/view.go`, `internal/ui/actions.go`

**Design**

- `marked map[string]bool` keyed `ns/name`, **cleared on kind or namespace
  change** — invisible marks across views are how you delete the wrong thing.
- `space` toggles; `ctrl+a`… is taken by AI → use `*` for
  mark-all-filtered, `esc` clears.
- A marked row: gutter color change + `▌` on the left. The status bar shows
  `3 marked`.
- Bulk runs **sequentially**, with progress; on an error it continues with the
  rest and ends with a summary "9 deleted, 3 failed" + the error list.
- The confirm modal lists **every** name (scrollable), not "3 objects".
- Read-only (T06) blocks bulk the same way it blocks single.

**Accept**

- [ ] Mark 3 pods → `D` → 1 confirm listing all 3 names, deletes all 3.
- [ ] 1 of 3 fails → the other 2 still delete; accurate report.
- [ ] Change namespace → marks disappear.
- [ ] Nothing marked → `D` handles the selected row as before.

---

## T09 — logs: grep / previous / timestamps / save

**Effort** M · **Deps** T02 · **Lane** C

**Goal** — the log view does the 4 things `kubectl logs` can that it cannot
yet.

**Why** — `internal/ui/logview.go` only has follow + scroll. The biggest gap
is `--previous`: on a CrashLoopBackOff pod the **current** log is empty; what
you need is in the container that died. Exactly when it matters most, the
tool has nothing.

**Files** — `internal/ui/logview.go`, `internal/ui/model.go`,
`internal/k8s/logs.go`, `internal/domain/domain.go`, `internal/mock/source.go`,
`docs/keybindings.md`

**Design**

Keys in the log view:

| key | does |
|---|---|
| `/` | grep: show only matching lines, highlight, `n`/`N` jump, works while following |
| `p` | toggle `--previous` — the dead container |
| `t` | toggle timestamps |
| `w` | write the buffer to `~/k10s-logs/<ns>-<pod>-<ts>.log`, toast the path |
| `c` | switch container (T02) |

- Grep is a **display filter**; it does not trim the buffer — turning it off
  must show everything again. Regex supported; a bad regex reports inline
  rather than silently falling back to substring.
- `--previous` with no previous container → say plainly "no previous
  container (pod has not restarted)", not an empty panel.
- Sanitize `ns`/`pod` in the file path.

**Accept**

- [ ] Grep while following: non-matching new lines stay hidden; turning grep
      off shows them.
- [ ] Restarted pod → `p` shows the previous crash's log.
- [ ] Pod never restarted → `p` reports clearly.
- [ ] `w` writes the right file, even with 5000 lines.
- [ ] Bad grep regex → inline error, no crash.

---

## T10 — port-forward manager

**Effort** S · **Deps** T03 · **Lane** A

**Goal** — `:pf` lists every running forward; stop one or stop all.

**Why** — forwards currently live in the pane; closing the pane or switching
views loses track of them. The goroutine keeps running, the port stays held,
and there is no way to see or stop it short of quitting.

**Files** — `internal/ui/model.go`, `internal/ui/commands.go`, `internal/ui/view.go`

**Design**

- A registry in `Model`: `[]activeForward{kind, ns, name, container, local, remote, since, stop func()}`.
- The `:pf` view is an ordinary table (reusing the existing table render);
  action `D` = stop.
- The header shows `⇄ 2` while forwards run — invisible state is forgotten
  state.
- Quitting the app → stop all in cleanup.
- A forward that dies unexpectedly (pod deleted) → removed from the registry
  + toast.

**Accept**

- [ ] 2 forwards → `:pf` lists both with local addresses.
- [ ] `D` stops that one; the other keeps running.
- [ ] Delete a forwarded pod → gone from the list, with a toast.
- [ ] Quit → no port left held.

---

## T11 — secret decode

**Effort** S · **Deps** none · **Lane** E

**Goal** — `x` on a Secret → view plaintext, instead of copying out to
`base64 -d`.

**Why** — everyone does this daily and today it is needlessly annoying.

**Files** — `internal/k8s/yaml.go`, `internal/domain/domain.go`,
`internal/mock/source.go`, `internal/ui/actions.go`, `docs/keybindings.md`

**Design**

- Optional interface `Decoder { Decoded(kind, ns, name string) (string, error) }`.
- **Masked** by default: `api-key: ••••••••` — a second `x` reveals. Screen
  sharing is routine; a feature that reveals secrets on open is a trap.
- Binary values → show `<binary, 2048 bytes>`, never dump raw bytes to the
  terminal.
- Read-only mode (T06) **does not** block it — this is a read. But log a
  "revealed" toast so the action leaves a trace on screen.
- Copy mode (`ctrl+s`) still works for copying values out.

**Accept**

- [ ] `x` on a Secret → keys + masked values.
- [ ] Second `x` → real values.
- [ ] Binary secret (`dockerconfigjson`) → layout intact.
- [ ] Kinds other than Secret → no `x` action.

---

## T12 — saved views

**Effort** M · **Deps** T07 · **Lane** D

**Goal** — `kind + ns + filter + sort` saved under a name; `1`–`9` jumps to
it.

**Why** — everyone has 3–5 places they keep looking at ("failing pods in
prod", "ingresses in staging"). Today each one is rebuilt by hand every time.

**Files** — `internal/config/` (new file `views.go`), `internal/ui/model.go`,
`internal/ui/commands.go`, `docs/config.md`

**Design**

- A **separate file** `~/.k10s/views.yaml`. The parser in `config.go` is flat
  and cannot read list-of-map — do not try to cram it in.
- Still no YAML dependency: a self-defined format, one block per view, a
  small parser + tests.

  ```yaml
  views:
    - key: "1"
      name: "prod failures"
      kind: pods
      namespace: all
      filter: "Error|CrashLoop"
      sort: "RESTARTS:desc"
      context: prod
  ```

- `:save <name>` stores the current view in the next free slot; `:views`
  opens a picker; `1`–`9` jump directly (only while the table has focus, not
  while typing).
- `context` is optional: when present, switch context as well.
- A kind/context in the file that no longer exists → skip that view with a
  warning; never break startup.

**Accept**

- [ ] `:save prod-fail` → `~/.k10s/views.yaml` has the entry; it survives a
      restart.
- [ ] `1` restores kind + ns + filter + sort in full.
- [ ] Corrupt file → the app still starts, with a warning.
- [ ] `1` while typing in the prompt types "1"; it does not jump views.

---

## T13 — label / field selector

**Effort** S · **Deps** none · **Lane** A

**Goal** — `:po -l app=api` and `:po --field-selector status.phase=Running`.

**Why** — label selectors are how people actually think about workloads. The
current text filter matches only displayed text; it cannot reach labels.

**Files** — `internal/ui/commands.go`, `internal/k8s/rows.go`,
`internal/domain/domain.go`, `internal/mock/source.go`

**Design**

- Optional interface `Selector { RowsSelected(kind, ns string, sel Selection) (cols []string, rows [][]string) }`,
  `Selection{Labels, Fields string}`.
- The label filter runs **on the lister cache** via `labels.Parse` — no extra
  API call, no broken performance guard.
- Field selector: support only the set Kubernetes actually supports
  (`status.phase`, `spec.nodeName`, `metadata.name`). An unsupported field →
  report it; never return wrong results silently.
- An active selector → a chip in the header; `esc` clears it.

**Accept**

- [ ] `:po -l app=api` shows only pods with that label.
- [ ] `-l 'env in (prod,staging)'` works (`labels.Parse` handles it).
- [ ] Malformed selector → clear error, not a mysteriously empty table.
- [ ] No extra request to the API server.

---

# P2

## T14 — owner tree (xray)

**Effort** L · **Deps** none · **Lane** E

**Goal** — `ctrl+r` opens the `Deployment → ReplicaSet → Pod → Container`
tree, every node clickable.

**Why** — the most k10s-flavored feature there is: ownership is a tree by
nature, and the flat table hides it. "Which pods is this deploy running, and
which belong to the old RS" is a daily question that today is answered by
cross-referencing three tables.

**Files** — `internal/k8s/tree.go` (new), `internal/domain/domain.go`,
`internal/mock/extra.go`, `internal/ui/treeview.go` (new),
`internal/ui/model.go`, `internal/ui/view.go`

**Design**

```go
type Tree interface {
    Tree(kind, ns, name string) (*domain.Node, error)
}
type Node struct {
    Kind, Name, Status string
    Healthy            bool
    Children           []*Node
}
```

- Down: follow `OwnerReferences` in reverse (`internal/k8s/actions.go`
  already reads ownerRefs; reuse it).
- Up: from a pod back to its deployment — same function, different starting
  point.
- Service → endpoints → pods is also worth drawing, even though it is not an
  ownerRef.
- Unhealthy nodes colored `theme.Danger`; expanded by default, `space`
  collapses.
- `enter` on a node = open that object in the main panel like any table.
- The tree is computed **on demand**, never auto-refreshed — the render path
  must not be touched.

**Accept**

- [ ] `ctrl+r` on a Deployment → RS → Pods; counts match the table.
- [ ] `ctrl+r` on a Pod → walks up to the Deployment.
- [ ] CrashLoop pod → red node; the parent shows a sign too.
- [ ] A tree of >200 nodes still renders smoothly.
- [ ] `TestKeypressLatency` stays intact.

---

## T15 — AI v2: stream + auto-context + redact

**Effort** L · **Deps** none · **Lane** E

**Goal** — AI goes from "one-shot that knows the pod name" to "has actually
looked at the object".

**Why** — today `internal/ai` only stuffs context/ns/kind/name into the prompt
and waits for the full response. The model never sees describe, events or
logs — exactly the things that answer the question. And the outgoing prompt
is **not filtered for secrets**.

**Files** — `internal/ai/ai.go`, `internal/ai/redact.go` (new),
`internal/ui/model.go`, `internal/ui/view.go`, `docs/commands.md`

**Design**

Four parts, deliverable in this order:

1. **Redact first** (mandatory, not optional). Before sending anything: strip
   Secret values, `Authorization:` headers, JWTs (`eyJ…`), AWS keys
   (`AKIA…`), private key blocks, `password=`/`token=`. One test per pattern.
   Until this step is done, the other three do not merge.
2. **Stream** — SSE for both OpenAI-compatible and Anthropic; tokens appear as
   they arrive. `esc` cancels midway.
3. **Auto-context** — `ctrl+a` with an object selected → gather `describe` +
   events (T04) + `logs --tail=100` + trimmed YAML, under a token budget;
   when over, cut logs first, then YAML. Show the user **exactly** what is
   about to be sent plus the byte count, with a cancel. Never send anything
   quietly.
4. **Follow-up** — keep history within the session; a second `ctrl+a`
   continues rather than restarts. `:ai clear` resets.

**Accept**

- [ ] A Secret in describe → absent from the payload (test with an httptest
      server, assert on the body).
- [ ] The response streams in; `esc` cancels.
- [ ] The context preview shows the correct byte count before sending.
- [ ] Follow-up remembers the previous question.
- [ ] No API key → AI mode off as before, no error.

**Not** — no "apply the AI's suggested patch" yet. Separate card, after T18.

---

## T16 — pulse dashboard

**Effort** M · **Deps** none · **Lane** E

**Goal** — opening k10s shows the **cluster's state** immediately, not a Pods
table.

**Why** — today the first screen is Pods of one namespace, which answers no
question. The real question on opening the app is "is anything on fire".

**Files** — `internal/ui/pulse.go` (new), `internal/ui/model.go`,
`internal/k8s/counts.go`, `internal/domain/domain.go`

**Design**

Tiles, clickable, each jumping to the matching view:

```
nodes 4/4 ready · pods 128 (3 not ready, 1 crashloop)
restarts in the last 1h: 7  · warning events: 12
pvc >80%: 2 · pod pending: 1 · job failed: 0
```

- Data comes from the **existing counts pass** (`internal/k8s/counts.go`); no
  new watches. This constraint is non-negotiable — it is why k10s starts
  fast.
- Refresh on the existing tick, no separate tick.
- Unknown numbers → show `—`, not `0`. (See `domain.CountUnknown`.)
- Toggle with `--home pulse|pods` (T05) + config `home:`.

**Accept**

- [ ] Startup still opens 0 watches; `TestNewStoreReturnsFast` green.
- [ ] Click "3 not ready" → Pods, pre-filtered.
- [ ] Cluster not yet synced → `—`, not `0`.
- [ ] `--home pods` keeps the old behavior.

---

## T17 — `can-i` / RBAC introspection

**Effort** M · **Deps** T06 · **Lane** A

**Goal** — actions you lack permission for are hidden, instead of a 403 after
the click.

**Why** — the Actions pane is the promise "this is what you can do with this
thing". With a restricted token, that promise is currently false.

**Files** — `internal/k8s/access.go` (new), `internal/domain/domain.go`,
`internal/ui/actions.go`, `internal/mock/extra.go`

**Design**

- `SelfSubjectAccessReview` per `(verb, resource, ns)`.
- **Cached per (verb, resource, ns)**, 5-minute TTL. SSAR must never be
  called from the render path — the pane is drawn every frame.
- Warm asynchronously on kind/ns change; while unknown, **show** the action
  (fail-open). Fail-closed would make actions flicker out of existence on
  open, which is far worse.
- `:cani <verb> <resource>` prints the result directly.
- Denied → tooltip "forbidden: needs delete on pods".

**Accept**

- [ ] Read-only token → Delete/Edit/Scale hidden.
- [ ] No SSAR called from the render path (test guard).
- [ ] Cache: switching kinds back and forth creates no new request within the
      TTL.
- [ ] SSAR error → fail-open; the action still shows.

---

## T18 — diff before apply

**Effort** M · **Deps** none · **Lane** B

**Goal** — `e` edit → see the server-side dry-run diff → then confirm.

**Why** — `Apply` currently writes straight through. Editing YAML in
`$EDITOR` and writing it blindly to the cluster is where `resourceVersion`
gets lost and someone else's change gets overwritten.

**Files** — `internal/k8s/actions.go`, `internal/domain/domain.go`,
`internal/ui/model.go`, `internal/ui/diffview.go` (new)

**Design**

- Optional interface `Differ { Diff(kind, ns, name, yaml string) (string, error) }`
  using server-side apply with `dryRun=All`, compared against the current
  object.
- Colored diff, `+`/`-`, only changed fields; skip `managedFields`/
  `resourceVersion`/`generation` — noise hides the real change.
- No change → "no changes", no apply.
- Conflict (object changed since the editor opened) → report; offer reload or
  force.

**Accept**

- [ ] Change replicas → the diff shows only that line.
- [ ] No edit → "no changes", apply not called.
- [ ] Object changed by someone else meanwhile → conflict warning.
- [ ] Read-only → edit is unreachable.

---

## T19 — helm releases

**Effort** L · **Deps** none · **Lane** A

**Goal** — a new kind `:helm`: list / history / values / rollback.

**Why** — most of what is in a cluster was installed by helm. Seeing releases
without leaving k10s is a clear gap versus `kubectl`-only.

**Files** — `internal/k8s/helm.go` (new), `internal/k8s/kinds.go`,
`internal/mock/data.go`, `internal/domain/domain.go`, `docs/commands.md`

**Design**

- **Do not** import the `helm.sh/helm/v3` SDK — it drags in a forest of
  dependencies. Read the release secrets directly
  (`type=helm.sh/release.v1`): base64 + gzip + JSON. The format has been
  stable since Helm 3.
- Columns: `NAME · NAMESPACE · REVISION · STATUS · CHART · APP VERSION · UPDATED`.
- Actions: `y` values · `h` history · `R` rollback (risky, subject to T06) ·
  `d` describe.
- Rollback shells out to the `helm` binary when it is on PATH; otherwise hide
  the action and say why. Reinstalling a release by hand is wrong.

**Accept**

- [ ] A helm-installed release → correct revision + status.
- [ ] `h` lists every revision.
- [ ] No helm binary → rollback hidden, with an explanation.
- [ ] Cluster with no releases → empty table, no error.

---

## T20 — ephemeral debug container + node shell

**Effort** M · **Deps** T02 · **Lane** C

**Goal** — a shell into distroless pods and into nodes.

**Why** — `s` today breaks on distroless/scratch images (no shell) — exactly
the images production tends to run.

**Files** — `internal/k8s/exec.go`, `internal/k8s/shell.go`,
`internal/domain/domain.go`, `internal/ui/actions.go`

**Design**

- Pod: `shift+s` → ephemeral container (the `kubectl debug` equivalent via
  the `ephemeralcontainers` subresource), default image `busybox:1.36`,
  configurable via `debug_image`.
- Node: `s` on a Node → a privileged `nsenter` pod on that node, cleaned up
  when the session closes.
- Both are **risky** → confirm + subject to read-only (T06).
- Cluster without ephemeral containers (feature gate off) → report clearly,
  no hang.
- Debug pods must be cleaned up even if k10s is killed — label them
  `k10s.io/debug=true` and sweep on the next startup.

**Accept**

- [ ] Distroless pod → `shift+s` gets a shell.
- [ ] `s` on a node → shell on the node.
- [ ] Closing the session → the debug pod is gone.
- [ ] k10s killed midway → the next run finishes the cleanup.
- [ ] Read-only → both blocked.

---

# P3

## T21 — cosign verify for self-update

**Effort** M · **Deps** none · **Lane** D

**Goal** — update verifies a signature, not only a checksum.

**Why** — `docs/roadmap.md` says it itself: a checksum proves the file matches
what the release published, **not** who published it. k10s overwrites its own
running binary — the most valuable attack path in the repo. Do it **before**
outside users arrive, not after.

**Files** — `internal/update/verify.go` (new), `internal/update/update.go`,
`.github/workflows/release.yml`, `Justfile`, `docs/update.md`

**Design**

- Releases signed with cosign keyless (GitHub Actions OIDC) → `.sig` + `.pem`
  next to the archive.
- Verify **inside** the binary; do not shell out to `cosign`. Pin the
  identity (`repo == p10node/k10s`, issuer GitHub).
- No signature (old releases) → a clear warning + prompt; neither wave it
  through silently nor hard-block.
- `K10S_UPDATE_REPO` pointing at a fork → the identity follows that repo, and
  say so to the user.

**Accept**

- [ ] Tampered archive → verify fails, not installed.
- [ ] Signature from another repo → fails.
- [ ] Unsigned release → warning + prompt.
- [ ] The `RealDist` test stays green.

---

## T22 — API key in the OS keychain

**Effort** M · **Deps** none · **Lane** D

**Goal** — the AI key is not plaintext in `config.yaml`.

**Why** — the roadmap lists it as a known limit. `~/.k10s/config.yaml` tends
to get dotfile-synced into git.

**Files** — `internal/config/secret_darwin.go`, `_linux.go`, `_windows.go`
(new), `internal/config/config.go`, `internal/ui/settings.go`

**Design**

- Keep the `AI.APIKey` field in the struct — the UI does not change.
- Backends: macOS `security`, Linux `secret-tool` (libsecret), Windows
  credential manager. None available → plaintext fallback as today **with a
  visible warning in `/settings`**.
- Migration: a key present in the file → moved into the keychain, and
  `api_key: "keychain"` written to the file.

**Accept**

- [ ] macOS: the key goes into Keychain; the file holds no plaintext.
- [ ] No keychain → still runs, with a warning.
- [ ] Old config migrates once, automatically.
- [ ] No cgo dependency added.

---

## T23 — custom keybindings + custom columns

**Effort** M · **Deps** T07 · **Lane** D

**Goal** — parity with k9s's `views.yaml`: rebind keys, choose displayed
columns.

**Files** — `internal/config/views.go`, `internal/ui/model.go`,
`internal/k8s/kinds.go`, `docs/config.md`

**Design** — `~/.k10s/views.yaml` (shared with T12, different section):
columns per kind, columns from labels (`labels.app`), columns from simple
JSONPath. Keybinding map `action → key`; duplicates detected at load and
reported, never silently last-wins.

**Accept** — [ ] adding `labels.app` to the Pods columns works · [ ] duplicate
keys report clearly · [ ] a corrupt file does not block startup.

---

## T24 — export CSV/JSON + clipboard OSC 52

**Effort** S · **Deps** T07, T08 · **Lane** B

**Goal** — `ctrl+e` exports the current table; `Y` copies YAML to the
clipboard.

**Design** — export respects filter + sort + marks (T08). The clipboard uses
**OSC 52** so copying works over ssh/tmux; fallback writes a file + toast.

**Accept** — [ ] the CSV opens in a spreadsheet · [ ] copying over ssh lands
in the local clipboard · [ ] a terminal without OSC 52 → file fallback.

---

## T25 — packaging: brew / scoop / nix

**Effort** M · **Deps** T21 · **Lane** D

**Design** — homebrew tap `p10node/tap`, scoop bucket, nix flake; the release
workflow bumps them automatically. Keep `install.sh` as the default path.

**Accept** — [ ] `brew install p10node/tap/k10s` works · [ ] a new tag bumps
the formula automatically · [ ] self-update detects a package-manager install
and tells the user to use the package manager instead of overwriting.

---

## T26 — missing kinds

**Effort** S · **Deps** none · **Lane** A

IngressClass · VPA · PriorityClass · MutatingWebhookConfiguration ·
ValidatingWebhookConfiguration · Lease · VolumeSnapshot · CSIDriver.

Follow the 5 steps in `docs/dev.md` § "Adding a resource kind" for each. One
commit per kind so it can be reviewed.

**Accept** — [ ] every kind has a row formatter + mock mirror + test · [ ]
`RowCount` opens no watch · [ ] sidebar counts are right.

---

## Prompt template

Paste this block verbatim to the worker agent, replacing `<ID>`:

```
You are working on the k10s repo (Kubernetes TUI, Go, bubbletea) in the current directory.

Task: complete exactly task <ID> in docs/plan.md.

Required, in this order:

1. Read docs/plan.md — the WHOLE "Worker contract" section and card <ID>.
   The Worker contract is the repo's invariants; breaking one means red CI or a UI that fails silently.
2. Read the files in the card's "Files" list, plus docs/architecture.md.
   If the card touches internal/k8s or the render path, also read docs/performance.md.
3. Implement the "Design" section exactly. Modify only files in the card's "Files".
   If you need another file → finish, then note it in the report; do not widen the scope yourself.
4. Write a test for every bullet in "Accept". Backend tests use the fake
   clientset via newTestStore; remember syncKinds(t, s, kPods, …) before asserting rows.
5. Run `just check` (fmt-check + vet + test). It must be green.
6. If the feature changes the UI → run `just shot 140 44 "<keys>"` and paste the frame into the report.
7. Update docs per "Definition of done" item 4.
8. Tick card <ID> in docs/plan.md § Board.

Hard limits:
- Do NOT add methods to domain.Source. New capabilities go through an optional
  interface + type-assert; both internal/k8s and internal/mock implement it.
- NO I/O in the render path. NO new watches at startup.
- NO new dependencies unless the card says so.
- NO commit, NO push, NO tag. Leave the diff in the working tree.
- NO refactoring outside the card's scope.

Final report, exactly 5 sections:
- What was done (per Accept bullet)
- Files changed + why
- Tests added + `just check` output
- `just shot` frame if the UI changed
- What deviates from the card, and why
```

## Lane prompt template

For lane mode. Replace `<LANE>`, `<LANE NAME>`, `<OWNED REGION>`,
`<CARD LIST>`, `<CROSS-LANE DEPS>`:

```
You are the worker agent for lane <LANE> (<LANE NAME>) on the k10s repo
(Kubernetes TUI, Go, bubbletea) in the current directory.

Your lane owns: <OWNED REGION>
Four other lanes run in parallel on other file regions. Leaving your region
creates conflicts for them.

SETUP (once):
  git checkout -b lane/<LANE>-<name> main

Read before changing anything:
  - docs/plan.md § "Worker contract" — ALL of it. These are the repo's invariants.
  - docs/plan.md § "Dispatch protocol" — the anti-conflict rules for the 5 lanes.
  - docs/architecture.md

CARDS, done SEQUENTIALLY in exactly this order:
<CARD LIST>

For EACH card, run the full loop:
  1. git rebase main   (another lane may have merged what you need)
  2. Read the card in docs/plan.md: Goal, Why, Files, Design, Accept, Tests, Not.
     Card touches internal/k8s or the render path → also read docs/performance.md.
  3. Implement the Design exactly. Modify only files in the card's "Files".
  4. Write a test for EVERY bullet in "Accept". Backend tests use the fake
     clientset via newTestStore; remember syncKinds(t, s, kPods, …) before
     asserting rows.
  5. `just check` must be green.
  6. UI changed → `just shot 140 44 "<keys>"`, keep the frame for the report.
  7. Update docs per Definition of done item 4; tick the card in § Board.
  8. git commit -m "<ID>: <card title>"   — NO push, NO tag.
  9. Move to the next card.

CROSS-LANE DEPS:
<CROSS-LANE DEPS>
  Dependency not yet on main → SKIP that card, do the next one, write
  "deferred" in the report. NEVER implement another lane's card yourself.

HARD LIMITS:
  - Do NOT add methods to domain.Source. New capability = optional interface
    + type-assert; both internal/k8s and internal/mock implement it.
  - Shared files (domain.go, model.go, actions.go, internal/mock/) are
    APPEND-ONLY, at the end, one block per card, with a card-ID comment. No
    reordering, no changing existing field order, no editing existing mock rows.
  - NO I/O in the render path. NO new watches at startup.
  - NO new dependencies, NO go mod tidy, NO go.mod changes.
  - NO push, NO tag, NO merge into main.
  - NO refactoring outside the card's scope.
  - Touching a file outside the lane's region → STOP, report to the dispatcher,
    do not decide on your own.

FINAL REPORT (after the last card), one section per card:
  - Card ID + done / deferred (reason)
  - Each Accept bullet: met or not
  - Files changed
  - Tests added + `just check` output
  - `just shot` frame if the UI changed
  - What deviates from the card, and why
  At the end: branch name + `git log --oneline main..HEAD`.
```
