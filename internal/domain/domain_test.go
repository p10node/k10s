package domain

import "testing"

func TestKindCan(t *testing.T) {
	k := Kind{Allowed: []string{ADescribe, AYAML, ADelete}}

	for _, id := range []string{ADescribe, AYAML, ADelete} {
		if !k.Can(id) {
			t.Errorf("Can(%q) = false, want true", id)
		}
	}
	for _, id := range []string{ARestart, AShell, ""} {
		if k.Can(id) {
			t.Errorf("Can(%q) = true, want false", id)
		}
	}
}
