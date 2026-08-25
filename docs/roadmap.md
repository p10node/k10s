# Roadmap

## Feedback round 1 — applied in mock v2

1. English UI everywhere ✓
2. Search box at the bottom of the Resources pane, type-to-filter ✓
3. Delete moved to `Shift+D` ✓
4. Node list moved into Resources (out of the banner) ✓
5. Banner keeps cluster-total CPU/MEM gauges ✓
6. AI prompt mode + `/config` (OpenAI-compat/Anthropic, URL, model, key)
   + slash commands with suggestion popup (`/context`, `/ns`, `/theme`,
   `/ai`, `/search`, `/help`) ✓
7. Config persisted to `~/.k10s/config.yaml` — theme, context, namespace,
   AI provider/URL/model/key. Saved on every change (theme cycle, `/context`,
   `/ns`, provider toggle, editing a `/config` field), loaded on startup ✓
8. Action set expanded to 12: added Top/metrics (`m`, pods + nodes) and
   Cordon (`o`, toggles label + node STATUS) / Drain (`u`, confirm modal,
   force-cordons) for nodes ✓

## Feedback round 3 — applied in mock v4

9. Main-panel row search — `/` while the main pane is focused (table mode)
   opens the panel's own search box (bottom two rows, mirrors the Resources
   pane's), filtering all columns of every row live; also reachable via
   `/filter <term>` ✓
10. `/ns all` — namespace filtering is now real, not cosmetic: a specific
    namespace shows only that namespace's rows, `all` shows everything with
    a NAMESPACE column, and every Resources-pane badge count updates too.
    `/ns` with no argument cycles every known namespace, ending on `all` ✓
11. CRDs and Custom Resource instances — two new resource kinds
    (`crds`, `customresources`) under a new "Custom Resources" group,
    reachable via `/crd` and `/dr`. CR instances live in `argocd` /
    `cert-manager` / `monitoring`, demonstrating the namespace filtering
    above with data that actually varies by namespace ✓

## Open decisions (awaiting review)

| # | Question                         | Current        |
|---|-----------------------------------|----------------|
| 1 | Pane ratio — widen either side?   | 22 / auto / 24 |
| 2 | Default theme                     | tokyo-night    |

Decisions 3 and 4 are resolved:

- **Action set** — Top/metrics + Cordon/Drain added, see above and
  [ui.md](ui.md#actions-pane-right) / [mock-data.md](mock-data.md).
- **Key storage** — plain file at `~/.k10s/config.yaml`, mode 0600, see
  [config.md](config.md) for the tradeoff and how to swap in an OS keychain
  later if wanted.

## After approval

1. **client-go + informer cache** — replaces `internal/mock` behind the same
   types; auto refresh; UI untouched.
2. **Streams** — logs follow, exec PTY, port-forward.
3. **Real AI** — HTTP per `/config` (OpenAI-compatible or Anthropic wire
   format), cluster context injected into prompts.
4. **Config file** — done, see [config.md](config.md).
