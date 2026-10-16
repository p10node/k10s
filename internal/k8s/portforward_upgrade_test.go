package k8s

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"k8s.io/client-go/rest"
)

func TestForwardDialerRejectsMalformedUpgradeWithoutReadingStream(t *testing.T) {
	closed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, b, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close(); close(closed) }()
		t.Cleanup(func() { _ = conn.Close(); <-closed })
		_, _ = b.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: wrong-protocol\r\n\r\n")
		_ = b.Flush()
		_, _ = io.Copy(io.Discard, conn)
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	d, err := newForwardDialer(&rest.Config{Host: server.URL, Timeout: 100 * time.Millisecond}, target)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		c, _, e := d.Dial("portforward.k8s.io")
		if c != nil {
			_ = c.Close()
		}
		done <- e
	}()
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("malformed upgrade accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("read error body from an already upgraded infinite stream")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("rejected upgraded connection leaked")
	}
}
