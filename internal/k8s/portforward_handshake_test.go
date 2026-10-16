package k8s

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"k8s.io/client-go/rest"
)

func TestForwardDialerBoundsTLSHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		conn, e := listener.Accept()
		if e != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(io.Discard, conn)
	}()
	target, _ := url.Parse("https://" + listener.Addr().String())
	d, err := newForwardDialer(&rest.Config{Host: target.String(), Timeout: 100 * time.Millisecond}, target)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = d.Dial("portforward.k8s.io")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("TLS silence not bounded: %v", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("TLS socket not closed")
	}
}

func TestForwardDialerBoundsProxyConnect(t *testing.T) {
	closed := make(chan struct{})
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			t.Error("CONNECT expected")
			return
		}
		<-r.Context().Done()
		close(closed)
	}))
	defer proxy.Close()
	defer proxy.CloseClientConnections()
	proxyURL, _ := url.Parse(proxy.URL)
	target, _ := url.Parse("https://fixture.test:443")
	d, err := newForwardDialer(&rest.Config{Host: target.String(), Timeout: 100 * time.Millisecond, Proxy: http.ProxyURL(proxyURL)}, target)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = d.Dial("portforward.k8s.io")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("CONNECT silence not bounded: %v", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("CONNECT socket not closed")
	}
}
