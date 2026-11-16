# Self-update

k10s replaces its own binary from GitHub Releases. Two entry points, one
implementation:

```
/update            inside the TUI — confirms, installs, offers to restart
k10s update        headless — same install with a progress line
/version           what is running, where updates come from, last check
k10s --version     same, one line, no TTY needed
```

Plus a check that runs at most once a day at startup and only speaks up when
there is something newer.

A copy that Homebrew installed is Homebrew's to upgrade: `k10s update` and
`/update` say `brew upgrade k10s` instead. See
[Package managers](#package-managers).

## The two halves

`internal/update` is split so the cheap half can run unattended:

| Half | Cost | What it does |
|------|------|--------------|
| `Client.Check` | one GET | `GET /repos/<owner>/<name>/releases/latest`, compare versions, pick this platform's asset |
| `Apply` | a download | fetch the asset, verify its checksum, extract, rename it over the running binary |

Nothing is downloaded until someone asks for it. `Check` is what the startup
check and the status-bar badge run on.

## What the startup check does — and doesn't

- Runs at most once every 24 hours (`update.CheckInterval`), tracked as
  `update.last_check` in the config file.
- Reports a newer release as a status-bar toast plus a clickable `⇧ X.Y.Z`
  badge. It never opens a dialog: nobody launches k10s to be interrupted.
- Says nothing when there is nothing newer, and **says nothing when it
  fails** — an offline laptop is not an error the user asked about. A manual
  `/update` does report its errors, because it was asked for.
- `/update skip` silences the release the last check found (`update.skip` in
  the config file) without turning the check off — the middle ground between
  installing now and never hearing about updates again. Installing that
  version clears the skip, since there is nothing left to silence.
- Turn the check off entirely in `/settings` → `UPDATES`, or
  `update.disabled: true` in the config file. `/update` still works on
  demand, and still installs a skipped version if asked.

## Installing

`/update` confirms first, through the same modal as delete and drain —
replacing the binary someone is running is not something to do off a
mistyped key. The dialog names the version, the repo, the asset, its size,
whether a checksum is available, and the first few lines of the release
notes.

The install itself:

1. **Resolve the target.** `os.Executable()` with symlinks resolved, so
   updating a `ln -s` in `~/bin` rewrites the real binary instead of
   clobbering the link.
2. **Check the directory is writable** before downloading anything, and say
   what to do when it isn't ("reinstall through your package manager, or
   re-run with sudo") rather than failing with `permission denied` after a
   30 MB download.
3. **Download into that same directory.** The final step is a rename, and a
   rename across filesystems is a copy — not atomic.
4. **Verify** the SHA-256 against the release's `checksums.txt`. A manifest
   that disagrees, *or that doesn't list the asset at all*, stops the
   install. A release with no manifest cannot be installed by self-update.
   There is no unverified-install override. SHA-256 detects artifact mismatch;
   it does not provide independent publisher authentication or signatures.
5. **Extract** `k10s` (or `k10s.exe`) from the `.tar.gz`/`.zip`, or take the
   asset as-is when it is a bare binary. An archive that renamed the binary
   falls back to its largest entry.
6. **Sniff the result.** ELF / Mach-O / PE magic and a minimum size. The
   realistic failure is a proxy serving an HTML error page with a 200, and
   renaming that over a working k10s would break the install.
7. **Rename it into place.** POSIX allows renaming over a running binary —
   the kernel holds the old inode until the process exits, so this session
   keeps working. Windows refuses, so the old file is moved to `k10s.old`
   and deleted on a later run.

A failure at any step leaves the existing binary untouched; there is a test
for that.

## Restarting

The new binary is on disk, but the process is still the old image, so the
update is not real until it restarts. k10s offers it: accepting quits the
program and `main.go` execs the new binary with the same arguments and
environment (`syscall.Exec` on POSIX — no second process, no lost TTY; a
child process on Windows, which has no exec). Declining keeps the session
running on the old build.

## Package managers

A binary that a package manager installed belongs to that package manager,
which keeps its own record of the version it put where. A self-update
written over it would leave that record naming a build that is gone, so
k10s steps aside and names the command that does the job:

| Installed by | Recognised by                                                          | Upgrade with        |
|--------------|------------------------------------------------------------------------|---------------------|
| Homebrew     | the binary, symlinks resolved, is in `<prefix>/Cellar/k10s/<version>/` | `brew upgrade k10s` |

- `k10s update` prints `k10s: installed by Homebrew, which updates it: run
  brew upgrade k10s` and exits 1. It checks nothing and writes nothing.
- `/update`, and a click on the status-bar badge, show the same line instead
  of the confirmation dialog. `/update skip` still silences a release.
- The startup check still runs, and `update.disabled` still turns it off.
  When it finds a newer release, its notice says `brew upgrade k10s`
  instead of `/update`.
- `/version` says who installed the binary.

Recognition compares whole path segments once symlinks are resolved. macOS
reports the `bin/k10s` link that Homebrew puts on `PATH`, and the link points
into the keg; Linux reports the keg directly. A directory that merely has
"Cellar" in its name is not a keg, and the copy `install.sh` puts in
`/usr/local/bin` or `/opt/homebrew/bin` is a plain file, so `k10s update`
replaces that one as before. `Apply` makes the same check before it writes
anything, so no way into the install can write into a keg.

## Which asset gets picked

Assets are matched by name, not by a fixed convention, because the same
release may be built by `just release`, by goreleaser, or by hand. Matching
needs one OS token *and* one architecture token, each delimited:

```
k10s_X.Y.Z_darwin_arm64.tar.gz     ✓
k10s-vX.Y.Z-macos-x86_64.tar.gz    ✓  darwin/amd64
k10s.Darwin.aarch64.tar.gz         ✓  darwin/arm64
k10s-linux-x64.tgz                 ✓  linux/amd64
k10s-linux-amd64                   ✓  bare binary, no archive
```

Delimited is the point: a plain substring match would hand a 32-bit
Raspberry Pi (`GOARCH=arm`) an `arm64` build. Sidecars — `.sha256`, `.sig`,
`.sbom.json` — are skipped, since they repeat the platform tokens in their
own names and would otherwise win by being listed first.

A release with nothing for the running platform is reported as such, with
the list of what *was* published. It is never approximated.

## Versions

`internal/version` holds the stamp:

```
go build -ldflags "-X github.com/p10node/k10s/internal/version.Version=vX.Y.Z \
                   -X github.com/p10node/k10s/internal/version.Commit=abc1234 \
                   -X github.com/p10node/k10s/internal/version.Date=2026-08-26"
```

`just build`, `just install` and `just release` all pass it, taking the
version from `git describe --tags --dirty`.

An unstamped build reports `dev`, and `dev` compares **older than every
release** — so `just build` followed by `/update` offers to install the
latest, which is what you want from a source build. Go's own VCS
pseudo-version (`v0.0.0-20260826114715-bb85f95437c1`) names a commit rather
than a release, so it is treated as `dev` too.

Comparison is the subset of semver that release tags actually use: an
optional `v`, dot-separated numbers, `+build` metadata ignored, and a `-rc.1`
prerelease sorting *below* the same version without one. `1.9.0 < 1.10.0`,
which is the case a string comparison gets backwards.

## Publishing a release

```bash
just tag vX.Y.Z        # tags and pushes; the workflow does the rest
```

`.github/workflows/release.yml` builds `darwin/{amd64,arm64}`,
`linux/{amd64,arm64}` and `windows/amd64`, writes `checksums.txt`, and
publishes them with generated notes. `just release` does the same thing
locally into `dist/` — same names, same manifest format.

After a full release, `.github/workflows/homebrew.yml` points the Homebrew
formula at it; [maintaining.md](maintaining.md#the-homebrew-tap) has the
details.

The naming and the manifest format are not cosmetic: they are what the
matcher and the verifier read. `internal/update`'s end-to-end test installs
a real `just release` build and runs `--version` on the result, which is the
check that the two halves agree:

```bash
just release && go test ./internal/update/ -run RealDist -v
```

It skips when `dist/` is empty.

## Pointing it somewhere else

Resolution order, most specific first:

1. `update.repo` in `~/.config/k10s/config.yaml`
2. `$K10S_UPDATE_REPO`
3. `update.DefaultRepo` — compiled in

A fork therefore updates from itself without being recompiled. The value is
`owner/name`; anything else is rejected with a message saying so rather than
producing a confusing 404.

## Limits

- **GitHub only.** The one API shape supported is
  `/repos/{owner}/{repo}/releases/latest`. A self-hosted manifest would be a
  second `Client` implementation behind the same `Check`/`Apply` pair.
- **No signature checking.** Checksums prove the download matches what the
  release published; they prove nothing about who published it. Verifying a
  signature (cosign, minisign) would mean shipping a public key in the
  binary — worth doing before this is used anywhere that matters.
- **The anonymous rate limit applies.** 60 requests per hour per IP, shared
  with everything else on that IP. The once-a-day throttle exists partly for
  this; hitting it is reported as a rate limit rather than a generic failure.
- **Prereleases are never offered.** `/releases/latest` excludes them, by
  design: someone running `/update` wants a stable build.
