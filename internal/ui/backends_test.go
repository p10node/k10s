package ui

import (
	"strings"
	"testing"

	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/k8s"
	"github.com/p10node/k10s/internal/mock"
)

// The two backends must offer the same resource kinds in the same order:
// the UI (panes, aliases, actions, row memory) is written against one list,
// and the offline demo is what every screenshot and UI test runs on. A kind
// added to one and forgotten in the other shows up as a command that works
// on a real cluster and dead-ends in the demo — or the reverse.
//
// This is the only place the two backends are compared, and it is a test, so
// neither package depends on the other at runtime.
func TestBackendsServeTheSameKinds(t *testing.T) {
	real, demo := k8s.Kinds(), mock.New("").Kinds()

	if len(real) != len(demo) {
		t.Fatalf("k8s serves %d kinds, mock %d:\n  k8s:  %v\n  mock: %v",
			len(real), len(demo), keysOf(real), keysOf(demo))
	}
	for i := range real {
		r, d := real[i], demo[i]
		if r.Key != d.Key {
			t.Fatalf("kind %d: k8s %q, mock %q — the order must match too", i, r.Key, d.Key)
		}
		if r.Name != d.Name || r.Short != d.Short || r.Group != d.Group || r.Namespaced != d.Namespaced {
			t.Errorf("%s: k8s %s/%s/%s ns=%v != mock %s/%s/%s ns=%v", r.Key,
				r.Name, r.Short, r.Group, r.Namespaced, d.Name, d.Short, d.Group, d.Namespaced)
		}
		if strings.Join(r.Cols, "|") != strings.Join(d.Cols, "|") {
			t.Errorf("%s columns: k8s %v, mock %v", r.Key, r.Cols, d.Cols)
		}
		if strings.Join(r.Allowed, "|") != strings.Join(d.Allowed, "|") {
			t.Errorf("%s actions: k8s %v, mock %v", r.Key, r.Allowed, d.Allowed)
		}
	}
}

// Every kind either backend serves must be typeable, and no two may answer
// to the same word — an ambiguous alias silently opens the wrong table.
func TestEveryServedKindHasUniqueAliases(t *testing.T) {
	for _, kinds := range [][]domain.Kind{k8s.Kinds(), mock.New("").Kinds()} {
		seen := map[string]string{}
		for _, k := range kinds {
			al := aliasesFor(k)
			if len(al) == 0 {
				t.Errorf("%s has no ':' alias, so it can only be reached by mouse", k.Key)
			}
			for _, a := range al {
				if prev, dup := seen[a]; dup {
					t.Errorf("alias %q is claimed by both %s and %s", a, prev, k.Key)
				}
				seen[a] = k.Key
			}
		}
	}
}

func keysOf(ks []domain.Kind) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = k.Key
	}
	return out
}

// A cluster-scoped kind must not advertise a namespace it ignores.
func TestClusterScopedTitlesOmitTheNamespace(t *testing.T) {
	m := newTestModel(t, mock.New(""))
	dismissOnboarding(m)
	m.applyNamespace("kube-system")

	for _, k := range m.kinds() {
		m.jumpToResource(k.Key)
		view := m.View()
		titled := strings.Contains(view, k.Name+" · kube-system")
		if k.Namespaced && !titled {
			t.Errorf("%s: title does not name the namespace it is filtered by", k.Key)
		}
		if !k.Namespaced && titled {
			t.Errorf("%s is cluster-scoped, but its title claims a namespace", k.Key)
		}
	}
}
