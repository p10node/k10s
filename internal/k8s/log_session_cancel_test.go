package k8s

import (
	"fmt"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLogSessionCloseCancelsStreamAndHistory(t *testing.T) {
	canceled := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/log") {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = fmt.Fprintln(w, "first")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			close(canceled)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"pod","namespace":"default","uid":"uid"},"spec":{"containers":[{"name":"main"}]}}`)
	}))
	defer srv.Close()
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{c: &Client{Clientset: cs}}
	session, err := s.OpenLogSession("pods", "default", "pod", 1)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-session.Lines():
	case <-time.After(time.Second):
		session.Close()
		t.Fatal("missing initial line")
	}
	session.Close()
	session.Close()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel HTTP request")
	}
	select {
	case _, ok := <-session.Lines():
		if ok {
			t.Fatal("stream remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("stream goroutine did not exit")
	}
	if _, _, err := session.History(10); err == nil {
		t.Fatal("closed session accepted history")
	}
}
