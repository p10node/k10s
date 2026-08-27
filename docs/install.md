# Install

Four ways in, all landing on the same static binary.

| Method           | Command                                                            | Notes                                          |
|------------------|--------------------------------------------------------------------|------------------------------------------------|
| Installer script | `curl -fsSL https://p10node.com/k10s/install.sh \| sh`             | macOS + Linux, checksum-verified, no Go needed |
| Go               | `go install github.com/p10node/k10s@latest`                        | needs Go 1.26+                                 |
| From a clone     | `just install`                                                     | stamps the version from `git describe`         |
| Release archive  | download from [releases](https://github.com/p10node/k10s/releases) | the only route on Windows                      |

After the first install, every later upgrade is `k10s update` — the binary
replaces itself, checksum-verified, and offers to restart. See
[update.md](update.md).

## The installer script

[`install.sh`](../install.sh) lives at the repo root. It is POSIX `sh` and
needs only `curl` (or `wget`) and `tar`:

1. Detects `darwin`/`linux` × `amd64`/`arm64` from `uname`. Windows and
   anything else exits with the manual instructions instead of guessing.
2. Resolves the newest release — the `/releases/latest` redirect first
   (no API quota), then the API, then `releases.atom`, which is the only
   one of the three that can see a pre-release. A stable release always
   wins over a pre-release.
3. Reads the archive name out of the release's `checksums.txt` rather than
   guessing it, so a release whose version string is spelled differently
   still resolves.
4. Verifies sha256 against `checksums.txt` and refuses to install on a
   mismatch. With no `sha256sum`/`shasum`/`openssl` on the box it warns and
   continues.
5. Installs with `install -m 755`, which replaces the file instead of
   writing through it — a running `k10s` keeps its own inode.

### Flags and environment

Both spellings work; the flag wins.

| Flag               | Environment        | Default                  |
|--------------------|--------------------|--------------------------|
| `--version v0.1.0` | `K10S_VERSION`     | the newest release       |
| `--dir ~/bin`      | `K10S_INSTALL_DIR` | see below                |
| `--no-sudo`        | `K10S_NO_SUDO=1`   | escalate only from a TTY |

Piped into `sh`, flags go after `-s --`:

```bash
curl -fsSL https://p10node.com/k10s/install.sh | sh -s -- --dir ~/bin
```

### Where it installs

In order: `--dir` if given → the first of `/usr/local/bin`,
`/opt/homebrew/bin` that is already writable → `/usr/local/bin` with `sudo`,
but **only when stdin is a TTY** → `${XDG_BIN_HOME:-~/.local/bin}`.

That TTY condition is the whole reason the fallback exists: in
`curl … | sh` the script's stdin is the pipe, so a `sudo` password prompt
would have nothing to read and would hang. A directory the user owns is the
only honest default there. If it is not on `PATH`, the script prints the
`export PATH=…` line to add.

### Reading it before running it

Piping a script into a shell means trusting the host that served it. The
paranoid path is the same two steps on any project:

```bash
curl -fsSL https://p10node.com/k10s/install.sh -o k10s-install.sh
less k10s-install.sh
sh k10s-install.sh
```

Or skip the script and verify by hand — it does nothing you cannot type:

```bash
tag=v0.1.0
base=https://github.com/p10node/k10s/releases/download/$tag
curl -fsSLO $base/k10s_${tag}_darwin_arm64.tar.gz
curl -fsSL  $base/checksums.txt | shasum -a 256 -c --ignore-missing
tar xzf k10s_${tag}_darwin_arm64.tar.gz && sudo install -m 755 k10s /usr/local/bin/
```

## Hosting the short URL

Both scripts are served straight out of the repo; the domain is a redirect in
front of them, so publishing a fix is a normal push to `main` with nothing to
deploy.

`p10node.com` is on Cloudflare, so the cheapest wiring is two **Redirect
Rules** (Rules → Redirect Rules → Create), one per script:

| If `http.request.uri.path` is in | Then static redirect `301` to                                      |
|----------------------------------|--------------------------------------------------------------------|
| `/k10s/install.sh` `/k10s`       | `https://raw.githubusercontent.com/p10node/k10s/main/install.sh`   |
| `/k10s/uninstall.sh`             | `https://raw.githubusercontent.com/p10node/k10s/main/uninstall.sh` |

Preserve query string: off, in both.

`curl -fsSL` follows redirects (`-L`), so a 301 is invisible to the user.
`raw.githubusercontent.com` serves it as `text/plain` and `sh` does not care
about the content type.

A Cloudflare **Worker** on the same route is worth it only if you want to
proxy rather than redirect (so the URL bar and `curl -I` never mention
GitHub), pin the script to a tag instead of `main`, or count installs.

### Names worth considering

Checked 2026-08-27 — recheck before buying, availability moves.

| URL                           | Status                             | Verdict                                                                                                                                   |
|-------------------------------|------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------|
| `p10node.com/k10s/install.sh` | yours already                      | **what the docs use.** Zero cost, one redirect rule, reads as "a p10node tool"                                                            |
| `k10s.sh`                     | unregistered at the `.sh` registry | **the upgrade.** `curl -fsSL https://k10s.sh \| sh` — the TLD *is* the shell, and it is the shortest thing anyone will ever type. ~$35/yr |
| `get.p10node.com/k10s`        | yours (subdomain)                  | Same cost as the path version; nicer if p10node ever ships a second tool                                                                  |
| `k10s.dev`                    | taken (parked, Namecheap DNS)      | —                                                                                                                                         |
| `k10s.app`                    | taken (WordPress DNS)              | —                                                                                                                                         |
| `k10s.io`                     | registered, no nameservers         | Parked; a transfer would have to be negotiated                                                                                            |

If you buy `k10s.sh`, keep the `p10node.com` path working — installer URLs
end up pasted into other people's runbooks and CI, and breaking one is a
support ticket you never see.

## Uninstall

```bash
curl -fsSL https://p10node.com/k10s/uninstall.sh | sh
```

[`uninstall.sh`](../uninstall.sh) is the mirror of the installer, and the
same two rules apply: it prints every path before touching it, and it never
prompts without a TTY.

- It collects **every** copy on `PATH` plus `/usr/local/bin`,
  `/opt/homebrew/bin`, `~/.local/bin` and `$GOBIN` — a `go install` build
  and an `install.sh` build can both be on one machine, and only the first
  on `PATH` is visible to `command -v`. Paths are resolved, so the same file
  found twice is reported once.
- The config survives by default. `--purge` deletes it, honouring
  `$K10S_CONFIG` when set (only that file then, not the directory around it).
- `--dry-run` prints the list and stops. Worth doing first.
- Anything needing root that it cannot ask about is skipped with the exact
  `sudo rm …` line to run, and the exit status is non-zero.

| Flag              | Environment        | Effect                                                |
|-------------------|--------------------|-------------------------------------------------------|
| `--purge`         | `K10S_PURGE=1`     | also delete `~/.k10s`                                 |
| `--keep-config`   | —                  | keep it, and drop the reminder that it is still there |
| `--dir <path>`    | `K10S_INSTALL_DIR` | look only there                                       |
| `--dry-run`, `-n` | —                  | list, remove nothing                                  |
| `-y`, `--yes`     | —                  | skip the confirmation prompt                          |
| `--no-sudo`       | `K10S_NO_SUDO=1`   | skip anything needing root                            |

By hand, it is two lines:

```bash
rm "$(command -v k10s)"
rm -rf ~/.k10s          # config + theme choice, see config.md
```
