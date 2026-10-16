package k8s

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestPortForwardStartupHonorsConfiguredTimeout(t *testing.T) {
	entered := make(chan struct{})
	disconnected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			pod := corev1.Pod{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}, ObjectMeta: metav1.ObjectMeta{Name: "probe", Namespace: "default"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "probe", Ports: []corev1.ContainerPort{{ContainerPort: 8080}}}}}}
			if err := json.NewEncoder(w).Encode(pod); err != nil {
				t.Errorf("encode: %v", err)
			}
			return
		}
		close(entered)
		<-r.Context().Done()
		close(disconnected)
	}))
	defer server.Close()
	defer server.CloseClientConnections()
	cfg := &rest.Config{Host: server.URL, Timeout: 250 * time.Millisecond}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{c: &Client{Clientset: client, RestConfig: cfg}}
	done := make(chan error, 1)
	go func() {
		_, stop, e := store.PortForward("pods", "default", "probe")
		if stop != nil {
			stop()
		}
		done <- e
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("fixture handshake never reached")
	}
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
			t.Fatalf("expected handshake deadline error, got %v", err)
		}
	case <-time.After(2 * time.Second):
		server.CloseClientConnections()
		<-done
		t.Fatal("SPDY startup ignored configured timeout and hung until test closed connection")
	}
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("timed-out handshake did not close its owned connection")
	}
}
