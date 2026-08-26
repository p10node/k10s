# k10s docs

| File                               | Contents                                                    |
|------------------------------------|-------------------------------------------------------------|
| [architecture.md](architecture.md) | Packages, the Source boundary, render pipeline, mouse zones |
| [backends.md](backends.md)         | The live cluster backend and the offline demo               |
| [performance.md](performance.md)   | Lazy watches, cheap counts, and the rules that keep it fast |
| [ui.md](ui.md)                     | Layout, panes, focus model, zoom, overlays, loading         |
| [keybindings.md](keybindings.md)   | Full key + mouse reference                                  |
| [commands.md](commands.md)         | Prompt modes, slash commands, search palette, AI settings   |
| [themes.md](themes.md)             | Theme list, picker, palette contract, adding a theme        |
| [config.md](config.md)             | `~/.k10s/config.yaml` — what's saved, when, and how         |
| [update.md](update.md)             | Self-update: version stamps, release matching, installing   |
| [dev.md](dev.md)                   | Build, tests, headless renderer, width invariant            |
| [roadmap.md](roadmap.md)           | What's done and what's left                                 |
| [marketing.md](marketing.md)       | Positioning, channels, launch sequence, repo hygiene        |
| [build-in-public.md](build-in-public.md) | Post shapes, cadence, the demo GIF, what stays private |

Status: **live** (2026-08-26). k10s talks to a real cluster through
client-go and falls back to the offline demo when none is reachable.
