# Maintaining

The maintainer side of the repo: how pull requests get in, what CI and the
rulesets enforce, and how a release goes out. `dev.md` covers building and
testing; `install.md` and `update.md` cover what a release has to look like.

Maintainers: `pierreneter` (admin) and `junior`. `.github/CODEOWNERS` lists
both, so every pull request asks the other one for a review.

## Pull requests

- **Sign off every commit** (`git commit -s`). The `DCO` check fails
  otherwise. Dependabot's commits are exempt.
- **Merge with a merge commit** ("Create a merge commit"). The history is
  `Merge pull request #N from ...` all the way down; squash and rebase are
  allowed by the ruleset but not used.
- **Keep the branch current.** The ruleset requires a branch to be up to
  date with `main` before it merges, so after every merge the other open
  pull requests need an *Update branch*. For a pull request from a fork that
  button only works when the author ticked **Allow edits by maintainers**;
  otherwise the author merges `main` in:

  ```bash
  git fetch upstream main      # upstream = p10node/k10s
  git merge upstream/main
  git push origin <branch>     # origin = the fork
  ```

- **One approval from the other maintainer**, and a push to the branch
  dismisses it. With two maintainers this is review by the other one.
- **Dependabot** opens pull requests weekly: direct Go modules (the `k8s.io`
  and `charmbracelet` modules each move as one group) and GitHub Actions.
  Green CI is the review; merge them like any other pull request. It never
  touches indirect modules. When the weekly `govulncheck` run flags one,
  bump it by hand, as #13 did:

  ```bash
  go get golang.org/x/net@latest && go mod tidy
  go run golang.org/x/vuln/cmd/govulncheck@latest ./...
  ```

## CI

`.github/workflows/test.yml` runs `gofmt`, `go vet`, `go test` and
`govulncheck` on every pull request and push to `main`, on Go 1.26.x and
1.27.x, and once a week on Monday with nothing changed. A scheduled run that
fails when nobody touched anything means a new advisory against a dependency:
bump it.

Three checks are required on `main`: `DCO`, `test (1.26.x)`, `test (1.27.x)`.
The names come from the job and its matrix, so **changing the Go versions in
`test.yml` changes the check names**. Edit the ruleset in the same pull
request (Settings → Rulesets → `protect-main` → Require status checks to
pass), or every later pull request waits forever on a check that no longer
exists.

A job that fails on one Go version only, in a pull request that touches no Go
code, is a flaky test or a runner hiccup. Re-run the failed jobs; if it fails
the same way twice, open an issue with the log.

## Rulesets

Settings → Rulesets. No bypass on `main`, admins included. Changing a rule is
an admin action, and worth a line in the pull request that needs it.

| Ruleset        | Applies to | What it enforces                                                                                             |
|----------------|------------|--------------------------------------------------------------------------------------------------------------|
| `protect-main` | `main`     | Pull requests only, one approval, the three checks above, up to date with `main`, no force push, no deletion |
| `release-tags` | tags `v*`  | Only the bypass list creates a tag; nobody moves or deletes one once it is pushed                            |

The bypass list of `release-tags` is who can ship a release. It has to hold
at least the *Repository admin* role, or `just tag` is refused for everyone.
To take back a tag pushed by mistake: disable `release-tags`
(Enforcement → Disabled), delete the tag and its release, enable it again.

## Releases

```bash
just tag v1.4.0        # tags, pushes, release.yml does the rest
```

`.github/workflows/release.yml` tests, cross-compiles the five platforms,
writes `checksums.txt`, attests every archive's build provenance, and
publishes the release with generated notes. A hand-written
`docs/releases/<tag>.md` goes on top when it exists. The archive names and the
manifest format are what the self-updater matches on; `dev.md` has the
details and the test that catches a mismatch.

Release builds use the newest patch of the current Go release, not the `go`
line in `go.mod`: move `go-version` in `release.yml` when a new Go comes out.

Check a published archive with:

```bash
gh attestation verify k10s_<tag>_<os>_<arch>.tar.gz --repo p10node/k10s
```

## Repository settings

Admin-only, outside the repo, so listed here. #16 is where they were asked
for.

| Setting                                | Where                        | Why                                                                                        |
|----------------------------------------|------------------------------|--------------------------------------------------------------------------------------------|
| Dependabot alerts and security updates | Settings → Advanced Security | Bumps a vulnerable indirect module on its own; Dependabot's config only covers direct ones |
| Workflow token read-only               | Settings → Actions → General | `release.yml` asks for what it needs itself; `test.yml` only reads                         |
| Private vulnerability reporting        | Settings → Advanced Security | A security bug can be reported without a public issue                                      |
| The two rulesets                       | Settings → Rulesets          | See above                                                                                  |
