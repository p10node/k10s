package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/streaming/pkg/httpstream"
	streamspdy "k8s.io/streaming/pkg/httpstream/spdy"
)

// Use the standard HTTP transport for the upgrade: the legacy SPDY transport
// reads headers directly from a socket and does not honor HTTP cancellation.
// The startup timer is disarmed on success; the upgraded body owns cancellation
// from then on, so an established tunnel does not inherit the startup deadline.
type boundedForwardDialer struct {
	config  *rest.Config
	url     *url.URL
	timeout time.Duration
}

func newForwardDialer(config *rest.Config, target *url.URL) (*boundedForwardDialer, error) {
	cfg := rest.CopyConfig(config)
	cfg.NextProtos = []string{"http/1.1"}
	if _, err := cfg.TransportConfig(); err != nil {
		return nil, err
	}
	timeout := 10 * time.Second
	if cfg.Timeout > 0 && cfg.Timeout < timeout {
		timeout = cfg.Timeout
	}
	return &boundedForwardDialer{config: cfg, url: target, timeout: timeout}, nil
}

func (d *boundedForwardDialer) Dial(protocols ...string) (connection httpstream.Connection, protocol string, err error) {
	ctx, cancel := context.WithCancelCause(context.Background())
	timer := time.AfterFunc(d.timeout, func() { cancel(context.DeadlineExceeded) })
	var body io.ReadCloser
	defer func() {
		if !timer.Stop() {
			<-ctx.Done()
			if connection != nil {
				_ = connection.Close()
				connection = nil
			}
			err = fmt.Errorf("port-forward startup: %w", context.Cause(ctx))
		}
		if err != nil {
			cancel(err)
			if body != nil {
				_ = body.Close()
			}
		}
	}()
	// net/http may continue a speculative dial/TLS/CONNECT after canceling
	// RoundTrip. Give this one-shot transport an explicitly owned raw socket.
	cfg := rest.CopyConfig(d.config)
	dial := cfg.Dial
	if dial == nil {
		dial = (&net.Dialer{KeepAlive: 30 * time.Second}).DialContext
	}
	owner := ctx
	cfg.Dial = func(_ context.Context, network, address string) (net.Conn, error) {
		conn, dialErr := dial(owner, network, address)
		if conn != nil {
			context.AfterFunc(owner, func() { _ = conn.Close() })
		}
		return conn, dialErr
	}
	if cfg.Proxy == nil {
		// Same default proxy policy; an explicit function also prevents the
		// client-go global TLS cache retaining this session's dial closure.
		cfg.Proxy = http.ProxyFromEnvironment
	}
	transport, transportErr := rest.TransportFor(cfg)
	if transportErr != nil {
		return nil, "", transportErr
	}
	var socket net.Conn
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { socket = info.Conn }})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "SPDY/3.1")
	for _, p := range protocols {
		req.Header.Add(httpstream.HeaderProtocolVersion, p)
	}
	response, err := transport.RoundTrip(req)
	if err != nil {
		return nil, "", err
	}
	body = response.Body
	if response.StatusCode != http.StatusSwitchingProtocols {
		data, readErr := io.ReadAll(io.LimitReader(body, 64*1024))
		if readErr != nil {
			return nil, "", readErr
		}
		var status metav1.Status
		if json.Unmarshal(data, &status) == nil && status.Kind == "Status" {
			return nil, "", apierrors.FromObject(&status)
		}
		return nil, "", fmt.Errorf("port-forward upgrade rejected (%s): %s", response.Status, strings.TrimSpace(string(data)))
	}
	// A 101 body is already a raw, possibly infinite stream; HTTP context
	// cancellation no longer interrupts reads on it. Reject without reading it.
	if !strings.EqualFold(response.Header.Get("Upgrade"), "SPDY/3.1") {
		return nil, "", fmt.Errorf("unexpected port-forward upgrade %q", response.Header.Get("Upgrade"))
	}
	protocol = response.Header.Get(httpstream.HeaderProtocolVersion)
	admitted := false
	for _, p := range protocols {
		if protocol == p {
			admitted = true
			break
		}
	}
	if !admitted {
		return nil, "", fmt.Errorf("unexpected port-forward protocol %q", protocol)
	}
	rw, ok := body.(io.ReadWriteCloser)
	if !ok || socket == nil {
		return nil, "", fmt.Errorf("port-forward upgrade transport did not expose a bidirectional connection")
	}
	conn := &forwardConn{Conn: socket, body: rw, cancel: func() { cancel(context.Canceled) }}
	connection, err = streamspdy.NewClientConnectionWithPings(conn, 5*time.Second)
	return connection, protocol, err
}

// Read through the upgraded body, not the raw socket: net/http may have already
// buffered the first SPDY frame together with the upgrade response headers.
type forwardConn struct {
	net.Conn
	body   io.ReadWriteCloser
	cancel func()
}

func (c *forwardConn) Read(p []byte) (int, error)  { return c.body.Read(p) }
func (c *forwardConn) Write(p []byte) (int, error) { return c.body.Write(p) }
func (c *forwardConn) Close() error                { c.cancel(); return c.body.Close() }
