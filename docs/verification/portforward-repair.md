# Closure of verification findings

This follow-up supersedes the open-failure and legacy-lint status in [the expanded p10-k8s report](live-p10-k8s-full.md). The two UI lifecycle findings were already repaired there; the remaining SPDY startup finding and full-repository lint findings are now resolved within the tested scope.

## Repair

- The original `TestPortForwardStartupHonorsConfiguredTimeout` failed again before this change: an owned silent upgrade peer kept `Store.PortForward` blocked beyond the watchdog. The test is now a required canonical test, not an external failing overlay.
- The dialer uses client-go REST transport configuration and the standard HTTP upgrade path, retaining TLS validation, credentials, impersonation and proxy policy. HTTP/1.1 is selected for SPDY, including when the peer also supports HTTP/2.
- A handshake timer uses the tighter of 10 seconds and a positive configured REST timeout. It covers connection establishment, TLS, proxy CONNECT, upgrade headers and rejection-body reads. Target resolution retains its separate existing timeout; this is not a claim that the whole operation takes at most 10 seconds.
- Merely canceling `RoundTrip` was insufficient: new tests demonstrated that speculative TLS/CONNECT sockets remained open. Each dial now owns its raw socket and cancellation closes it. Per-session transports are kept out of client-go's global TLS cache so it cannot retain the session closure.
- Successful negotiation disarms the startup timer. The established stream owns cancellation until Close; reading through the upgraded response body preserves already-buffered SPDY bytes.
- A separate RED probe demonstrated that reading a malformed HTTP 101 as an error body could hang after HTTP cancellation stopped applying. Such responses are now rejected without reading the upgraded stream.
- The implementation uses `k8s.io/streaming` and `portforward.NewForStreaming`, not deprecated compatibility imports. No dependency versions were changed.

## Canonical regression coverage

`go test -race ./internal/k8s -run 'TestForwardDialer|TestPortForwardStartup' -count=10 -timeout=90s` passed on the final source.

Coverage includes:

- Silent upgrade peer: deadline error and peer-observed disconnection.
- Silent TLS handshake and proxy CONNECT: deadline error plus actual socket closure.
- Stalled HTTP error body: bounded cancellation and cleanup.
- Malformed HTTP 101: immediate rejection and connection closure.
- API Forbidden: Kubernetes error classification preserved.
- Untrusted TLS certificate: rejected.
- Real TLS through an owned CONNECT proxy, with auth and impersonation headers verified by the peer.
- A real SPDY stream exchanges two distinct payloads after outliving the startup budget. The TLS peer also advertises HTTP/2.

The final independent read-only review found no concrete additional issues in the seven scoped implementation/test files and confirmed unchanged hashes. That review was static; execution evidence comes from the parent-run gates and live harness.

## Final gates

All passed on unchanged source:

- `KUBECONFIG=/nonexistent go test -race -count=1 ./...`
- `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0 ./...`: **0 issues for the entire repo**, not only new findings.
- `go vet ./...`
- The repeated port-forward regression command above.
- Fresh application and live-harness builds.

Lint cleanup includes existing unchecked returns, redundant selectors/conditions and unused private code. UI config saving already reports failures as a toast; callers now explicitly acknowledge its return. Best-effort cleanup and in-memory terminal writes are explicitly acknowledged. No blanket lint suppression was introduced.

## p10-k8s verification and cleanup

The expanded fixture matrix was rerun twice during the repair, including once on the final streaming-API build. Both exited 0. The final run again passed ConfigMap/CR identity, conflict/UID checks, logs/history, exec, namespace-limited RBAC, private-proxy watch outage/recovery and actual PTY YAML/logs/quit.

The live port-forward test now performs HTTP readback, waits 12 seconds beyond the maximum startup budget, performs another HTTP readback through the established tunnel, then stops it and confirms the listener closes. It passed on the final build.

Both newly created namespaces and all four CRDs were removed and independently confirmed absent using explicit `--context=p10-k8s` queries. Namespace-contained fixtures were consequently removed. No real node drain or modification of pre-existing workloads occurred. Expected client-go closed-network diagnostics during deliberate shell cancellation remain in the raw logs; this is not described as a warning-free run.

Durable evidence, gate commands and exits, source SHA-256 manifest, live/PTY recordings and cleanup receipts:

`/Users/pierreneter/.hermes/backups/k10s-hardening-1cuy2_kw/spdy-repair/`

The broader coverage limitations in the earlier report still apply: no live drain, exhaustive provider/version/plugin certification, or new release-artifact installation run. Arbitrary non-cooperative custom authentication/dial/transport callbacks are not made forcibly cancellable by a network deadline. No commit or push was performed.
