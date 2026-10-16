package k8s

import (
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	"k8s.io/streaming/pkg/httpstream"
	streamspdy "k8s.io/streaming/pkg/httpstream/spdy"
)

func TestForwardDialerTLSProxyAndLongLivedStream(t *testing.T) {
	var authorized atomic.Bool
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" || r.Header.Get("Impersonate-User") != "fixture-user" {
			http.Error(w, "missing fixture auth", http.StatusForbidden)
			return
		}
		authorized.Store(true)
		w.Header().Set(httpstream.HeaderProtocolVersion, "portforward.k8s.io")
		conn := streamspdy.NewResponseUpgrader().UpgradeResponse(w, r, func(stream httpstream.Stream, reply <-chan struct{}) error {
			go func() { <-reply; _, _ = io.Copy(stream, stream); _ = stream.Close() }()
			return nil
		})
		if conn == nil {
			return
		}
		defer func() { _ = conn.Close() }()
		<-conn.CloseChan()
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	var proxyCalls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != server.Listener.Addr().String() {
			http.Error(w, "wrong proxy destination", 400)
			return
		}
		proxyCalls.Add(1)
		upstream, err := net.DialTimeout("tcp", r.Host, time.Second)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer func() { _ = upstream.Close() }()
		downstream, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = downstream.Close() }()
		if _, err = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		if err = buffer.Flush(); err != nil {
			return
		}
		done := make(chan struct{})
		go func() { _, _ = io.Copy(upstream, buffer); _ = upstream.Close(); close(done) }()
		_, _ = io.Copy(downstream, upstream)
		_ = downstream.Close()
		<-done
	}))
	defer proxy.Close()
	target, _ := url.Parse(server.URL)
	proxyURL, _ := url.Parse(proxy.URL)
	cfg := &rest.Config{Host: server.URL, Timeout: 300 * time.Millisecond, BearerToken: "fixture-token", Impersonate: rest.ImpersonationConfig{UserName: "fixture-user"}, TLSClientConfig: rest.TLSClientConfig{CAData: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})}, Proxy: http.ProxyURL(proxyURL)}
	dialer, err := newForwardDialer(cfg, target)
	if err != nil {
		t.Fatal(err)
	}
	conn, protocol, err := dialer.Dial("portforward.k8s.io")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if protocol != "portforward.k8s.io" || !authorized.Load() || proxyCalls.Load() != 1 {
		t.Fatal("TLS/auth/impersonation/proxy was not exercised")
	}
	// Deliberately outlive the startup budget before sending application bytes.
	select {
	case <-conn.CloseChan():
		t.Fatal("established connection closed at startup deadline")
	case <-time.After(2 * cfg.Timeout):
	}
	done := make(chan error, 1)
	go func() {
		stream, e := conn.CreateStream(http.Header{})
		if e != nil {
			done <- e
			return
		}
		defer func() { _ = stream.Close() }()
		for _, payload := range [][]byte{[]byte("first-fixture"), []byte("distinct-second")} {
			if _, e = stream.Write(payload); e != nil {
				done <- e
				return
			}
			got := make([]byte, len(payload))
			if _, e = io.ReadFull(stream, got); e != nil {
				done <- e
				return
			}
			if !bytes.Equal(got, payload) {
				done <- errors.New("SPDY data corrupted")
				return
			}
		}
		done <- nil
	}()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("post-deadline SPDY stream blocked")
	}
}

func TestForwardDialerRejectsUntrustedTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("untrusted client reached HTTP handler") }))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	d, err := newForwardDialer(&rest.Config{Host: server.URL, Timeout: time.Second}, target)
	if err != nil {
		t.Fatal(err)
	}
	if conn, _, err := d.Dial("portforward.k8s.io"); err == nil {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("untrusted certificate accepted")
	}
}

func TestForwardDialerForbiddenAndStalledErrorBody(t *testing.T) {
	for _, stall := range []bool{false, true} {
		t.Run(map[bool]string{false: "forbidden", true: "stalled-body"}[stall], func(t *testing.T) {
			disconnected := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(disconnected)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(403)
				if stall {
					w.(http.Flusher).Flush()
					<-r.Context().Done()
					return
				}
				_, _ = io.WriteString(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","code":403,"message":"fixture denied"}`)
			}))
			defer server.Close()
			target, _ := url.Parse(server.URL)
			d, err := newForwardDialer(&rest.Config{Host: server.URL, Timeout: 100 * time.Millisecond}, target)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = d.Dial("portforward.k8s.io")
			if stall && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected deadline: %v", err)
			}
			if !stall && !apierrors.IsForbidden(err) {
				t.Fatalf("API forbidden lost: %v", err)
			}
			select {
			case <-disconnected:
			case <-time.After(time.Second):
				t.Fatal("owned request was not canceled")
			}
		})
	}
}
