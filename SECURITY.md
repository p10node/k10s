# Security policy

k10s runs with your kubeconfig, talks to your clusters and replaces its own
binary, so a security bug in it can matter. Please report one privately.

## Supported versions

Only the newest release gets security fixes. A fix ships as a new release,
not as a patch to an older one: upgrade with `k10s update` (`/update` inside
k10s), or by any of the routes in [docs/install.md](docs/install.md).

## Reporting a vulnerability

Please do not open a public issue or pull request for it. Report it
through GitHub's private vulnerability reporting instead:

**https://github.com/p10node/k10s/security/advisories/new**

That is the form behind the repository's **Report a vulnerability** button.
Until an advisory is published, the report is visible only to you and the
people who handle security for this repository.

### What to put in a report

GitHub's form asks for a summary, the details, a proof of concept and the
impact. Also useful:

- The output of `k10s --version`, which names the release, commit, platform
  and Go version, and how you installed k10s.
- What an attacker needs: a cluster object or a log line they control, a
  kubeconfig, a plugin file, a release in the repository k10s updates from,
  a place on the network.
- Whether anyone else knows about it, and any date you plan to publish.

Test against a cluster you own: `kind` works, and `k10s demo` needs no
cluster at all.

## What happens next

k10s is maintained by two people, so this is best effort, with no fixed
response times. A maintainer acknowledges the report and asks in the
advisory for anything needed to reproduce it. If it is a vulnerability,
the fix ships in a new release and the advisory is published with it,
crediting you unless you would rather not be. If it is not, you get the
reason, and you are free to open a public issue.

Please keep the details private until the advisory is published.

## Scope

How k10s is meant to behave, which is what a report measures it against.

**Read-only mode is a guard against k10s, not against you.** With
`--readonly`, every client k10s builds refuses, before the request leaves
the machine, anything other than GET, HEAD or OPTIONS, the GETs that open
exec, attach or port-forward streams, and anything sent through the proxy of
a pod, a service or a node. External plugins and typed shell commands do not
run. A request that changes the cluster and still gets through is a
vulnerability. Read-only mode does not change what your credentials allow,
and the credential helpers in your kubeconfig (an `exec` plugin, the oidc
token refresh) run as they would for kubectl. See
[docs/commands.md](docs/commands.md#read-only-mode-k10s---readonly).

**The self-updater installs only what a release published.** The daily
check reads the newest release of `p10node/k10s`, or of the repository set
with `update.repo` or `$K10S_UPDATE_REPO`, and downloads nothing.
`k10s update` and `/update` download this platform's archive into the
binary's own directory, refuse it unless its SHA-256 matches the release's
`checksums.txt`, and rename it over the binary. Getting the updater to
install anything else, or to replace any other file, is a vulnerability. The
checksum shows that the archive is the one the release lists, not who
published the release: anyone who can publish a release in that repository
can ship a binary that passes, which is what the attestations below are
for. `update.disabled: true` in `~/.k10s/config.yaml` turns the daily check
off. See [docs/update.md](docs/update.md).

**Plugins and shell commands run as you.** A plugin file is a program you
chose to run, so anyone who can write to `~/.k10s/plugins.yaml` can already
run commands as you; that is not a vulnerability in k10s. k10s runs the
plugin's command directly, never through a shell of its own, and fills
object names, namespaces and column values into its arguments as text. When
the command is `sh`, `bash`, `zsh`, `dash` or `ksh` with `-c`, the script
gets them as environment variables instead of having them pasted into its
text. k10s itself turning a value from the cluster into code, or a plugin
with `confirm: true` that runs without asking, is a vulnerability. What a
plugin's own script or program does with those values, as with `python3 -c`
or `kubectl exec ... -- sh -c`, is up to the plugin. See
[docs/plugins.md](docs/plugins.md).

**The installer and uninstaller**, `install.sh` and `uninstall.sh`, are in
scope. The installer checks the archive against `checksums.txt` and refuses
to install on a mismatch.

**A published vulnerability in Go or in a Go module** that a release ships
with is not a secret, so an issue or a pull request is fine. `govulncheck`
runs on every pull request and once a week. If k10s is exposed in a way the
published advisory does not cover, report that privately.

Known limits, documented and not vulnerabilities in themselves:

- The self-updater checks checksums, not signatures or attestations.
- `install.sh` warns and installs without verifying when it cannot check:
  when the release's `checksums.txt` cannot be fetched or does not list the
  archive, or on a machine with no `sha256sum`, `shasum` or `openssl`. The
  self-updater refuses instead.
- `~/.k10s/config.yaml` (mode 0600) can hold the legacy AI API key in plain
  text. AI is disabled in this build.

## Checking a release

Every archive is listed in its release's `checksums.txt`. From v0.1.8 on,
every archive also carries a build provenance attestation, signed through
Sigstore by the release workflow, which shows that this repository's
workflow built it from the tagged commit. With the GitHub CLI:

```bash
gh attestation verify k10s_<tag>_<os>_<arch>.tar.gz --repo p10node/k10s
```

The Windows archive is a `.zip`; the command is the same.
[docs/install.md](docs/install.md#reading-it-before-running-it) has the
checksum check by hand.
