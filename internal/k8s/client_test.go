package k8s

import "testing"

func TestLoadedKubeconfigPathPreservesMergedKubeconfigList(t *testing.T) {
	t.Setenv("KUBECONFIG", "/tmp/one:/tmp/two")
	if got := loadedKubeconfigPath(""); got != "/tmp/one:/tmp/two" {
		t.Errorf("loadedKubeconfigPath = %q, want full KUBECONFIG list", got)
	}
	if got := loadedKubeconfigPath("/tmp/explicit"); got != "/tmp/explicit" {
		t.Errorf("explicit path = %q", got)
	}
}
