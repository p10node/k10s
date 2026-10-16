package k8s

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/p10node/k10s/internal/domain"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestLogSessionUsesSingleTailFollowRequestAndPinnedHistory(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	uid := "original"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, r.URL.String())
		if strings.HasSuffix(r.URL.Path, "/log") {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = fmt.Fprint(w, "one\ntwo\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"pod","namespace":"default","uid":%q},"spec":{"containers":[{"name":"main","image":"fixture"}]}}`, uid)
	}))
	defer srv.Close()
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{c: &Client{Clientset: cs}}
	opener, ok := any(s).(domain.LogOpener)
	if !ok {
		t.Fatal("missing pinned log session capability")
	}
	session, err := opener.OpenLogSession("pods", "default", "pod", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for _, want := range []string{"one", "two"} {
		select {
		case line := <-session.Lines():
			if line != want {
				t.Fatalf("got %q want %q", line, want)
			}
		case <-time.After(time.Second):
			t.Fatal("stream stalled")
		}
	}
	mu.Lock()
	logs := 0
	for _, c := range calls {
		if strings.Contains(c, "/log?") {
			logs++
			if !strings.Contains(c, "follow=true") || !strings.Contains(c, "tailLines=2") || !strings.Contains(c, "container=main") {
				t.Errorf("not one tail+follow request: %s", c)
			}
		}
	}
	mu.Unlock()
	if logs != 1 {
		t.Fatalf("initial logs made %d requests", logs)
	}
	lines, _, err := session.History(4)
	if err != nil || strings.Join(lines, ",") != "one,two" {
		t.Fatalf("history: %v %v", lines, err)
	}
	mu.Lock()
	uid = "replacement"
	mu.Unlock()
	if _, _, err := session.History(4); err == nil {
		t.Fatal("history admitted a replacement pod with the same name")
	}
}

const logSessionPodJSON = `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"pod","namespace":"default","uid":"u"},"spec":{"containers":[{"name":"main","image":"fixture"}]}}`

func TestLogSessionHistoryTruncatesToNewestWithinBudget(t *testing.T) {
	line := strings.Repeat("x", 1023)
	const total = 5000 // ~5 MiB, past the 4 MiB budget
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/log") {
			w.Header().Set("Content-Type", "text/plain")
			if strings.Contains(r.URL.RawQuery, "follow=true") {
				_, _ = fmt.Fprint(w, "tail\n")
				return
			}
			for i := 0; i < total; i++ {
				_, _ = fmt.Fprintf(w, "%05d %s\n", i, line)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, logSessionPodJSON)
	}))
	defer srv.Close()
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	session, err := (&Store{c: &Client{Clientset: cs}}).OpenLogSession("pods", "default", "pod", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	lines, more, err := session.History(total)
	if err != nil {
		t.Fatal(err)
	}
	if more {
		t.Fatal("truncated history claimed more is reachable")
	}
	if len(lines) == 0 || len(lines) >= total || !strings.HasPrefix(lines[len(lines)-1], "04999 ") {
		t.Fatalf("kept %d lines; newest line must survive", len(lines))
	}
	bytes := 0
	for i, l := range lines {
		bytes += len(l) + 1
		if want := fmt.Sprintf("%05d ", total-len(lines)+i); !strings.HasPrefix(l, want) {
			t.Fatalf("line %d is %q, want prefix %q", i, l[:6], want)
		}
	}
	if bytes > logReadMaxBytes {
		t.Fatalf("retained %d bytes over budget", bytes)
	}
}

func TestLogSessionStartupWatchdogIsReported(t *testing.T) {
	prev := logStreamStartupTimeout
	logStreamStartupTimeout = 50 * time.Millisecond
	t.Cleanup(func() { logStreamStartupTimeout = prev })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/log") {
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, logSessionPodJSON)
	}))
	defer srv.Close()
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	session, err := (&Store{c: &Client{Clientset: cs}}).OpenLogSession("pods", "default", "pod", 1)
	if err == nil {
		session.Close()
		t.Fatal("stalled stream was returned as healthy")
	}
	if !strings.Contains(err.Error(), "did not start within") {
		t.Fatalf("err=%v", err)
	}
}
