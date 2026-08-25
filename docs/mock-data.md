# Mock data (`internal/mock`)

Everything the UI shows is fake and lives here so swapping in client-go later
touches nothing in `internal/ui`.

## data.go

- `Cluster` — context, server, version, namespace, 3 nodes (one NotReady,
  used by the banner totals and by `TopNode`'s CPU/MEM figures).
- `Actions` — the 12 quick actions: id, hotkey, label, `Risky` flag (Drain,
  Delete).
- `Resources` — 16 kinds with columns, rows, and an `Allowed` action list
  per kind (drives dimming in the Actions pane; `nodeActions` gates
  Top/Cordon/Drain to the Nodes kind only). Rows include deliberate
  trouble: `billing-worker` CrashLoopBackOff ×17, `payment-api` Pending on
  insufficient CPU, a Terminating pod, a NotReady node — so status colors,
  events and the AI answer have something real to point at. The last two
  kinds (`crds`, `customresources`) cover CustomResourceDefinitions and
  their instances — see Namespaces below and commands.md.
- `Describe`, `Logs`, `YAML` — canned kubectl-shaped output.
- `NSRow` / `Resource.Extra` / `Visible(r, ns)` / `VisibleCount(r, ns)` /
  `NamespaceCycle()` — the namespace-filtering layer behind `/ns`. A
  resource's base `Rows` are implicitly namespace `default`; `Extra` holds
  rows tagged with any other namespace. `Visible` picks what a given `ns`
  should show (default rows only / one namespace's `Extra` rows / everything
  with a synthesized NAMESPACE column for `all`) — see commands.md#namespaces
  for the user-facing behavior this produces.

## extra.go

- `Contexts` — 3 fake contexts for `/context` cycling.
- `SlashCommands` — name/args/desc, feeds the suggestion popup (includes
  `/filter`, `/crd`, `/dr` alongside the earlier commands).
- `AIProviders` — OpenAI-compatible + Anthropic presets (default URL, model)
  for the `/config` modal.
- `AIAnswer(q)` — canned AI response that diagnoses the seeded problems
  (billing-worker crashloop, Pending payment-api, NotReady node).
- `NodeCordoned(name)` / `SetCordon(name, bool)` / `ToggleCordon(name)` — the
  Nodes resource table *is* the node's state in this mock (no separate
  object), so cordon status is read from and written to the STATUS cell of
  that row directly (`Ready` ↔ `Ready,SchedulingDisabled`).
- `TopPod(name)` / `TopNode(name)` — canned `kubectl top --containers` /
  `kubectl top node` + capacity/allocatable text, driven by the Cluster.Nodes
  CPU/MEM percentages so the numbers stay consistent with the banner totals.
- `Help()` — the `/help` text view.

## Replacement plan

Phase 1 keeps these types as the boundary: an informer-backed provider will
fill `Resources[i].Rows` and `Cluster` on a refresh tick; `Describe/Logs/
YAML` become real calls; `AIAnswer` becomes an HTTP call per the `/config`
settings with cluster context injected.
