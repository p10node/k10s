package domain

import "testing"

// Read-only mode keeps the actions that only read and drops every other one:
// the ones that change the cluster, and the ones that open a session into it.
func TestReadOnlyAllowsOnlyReadingActions(t *testing.T) {
	reads := map[string]bool{ADescribe: true, AYAML: true, ALogs: true, ATop: true}
	all := []string{ADescribe, AYAML, ALogs, AShell, AShellOut, APortFwd, ARestart, AEdit, AScale, ATop, ACordon, ADrain, ADelete}
	for _, id := range all {
		if got := ReadOnlyAllows(id); got != reads[id] {
			t.Errorf("ReadOnlyAllows(%q) = %v, want %v", id, got, reads[id])
		}
	}
}
