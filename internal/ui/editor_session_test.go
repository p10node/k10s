package ui

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
	"os"
	"strings"
	"testing"
)

type manifestSource struct{ *targetSource }

const originalManifest = "apiVersion: v1\nkind: Pod\nmetadata:\n  name: same\n  namespace: A\n  uid: original\n  resourceVersion: '7'\nspec: {}\n"

func (s manifestSource) YAML(kind, ns, name string) (string, error) { return originalManifest, nil }
func TestEditorPreservesOriginalIdentityAndVersion(t *testing.T) {
	for _, field := range []string{"apiVersion", "kind", "name", "namespace", "uid", "resourceVersion", "spec"} {
		t.Run(field, func(t *testing.T) {
			t.Setenv("EDITOR", "'")
			s := manifestSource{&targetSource{Source: mock.New("")}}
			m := newTestModel(t, s)
			fetched := m.startEdit("pods", "A", "same")().(editFetchedMsg)
			defer func() { _ = os.Remove(fetched.path) }()
			_, cmd := m.Update(fetched)
			exit := cmd().(editExitMsg)
			exit.err = nil
			edits := map[string][2]string{"apiVersion": {"v1", "v2"}, "kind": {"Pod", "Service"}, "name": {"name: same", "name: other"}, "namespace": {"namespace: A", "namespace: B"}, "uid": {"original", "replacement"}, "resourceVersion": {"'7'", "'8'"}, "spec": {"spec: {}", "spec: {nodeName: test}"}}
			pair := edits[field]
			if err := os.WriteFile(fetched.path, []byte(strings.Replace(originalManifest, pair[0], pair[1], 1)), 0600); err != nil {
				t.Fatal(err)
			}
			_, apply := m.Update(exit)
			if apply != nil {
				if batch, ok := apply().(tea.BatchMsg); ok {
					for _, c := range batch {
						m.Update(c())
					}
				}
			}
			want := int32(0)
			if field == "spec" {
				want = 1
			}
			if s.applies.Load() != want {
				t.Fatalf("%s: applies=%d want=%d", field, s.applies.Load(), want)
			}
			if want == 0 {
				if _, err := os.Stat(fetched.path); err != nil {
					t.Fatal("rejected edit lost file")
				}
			}
		})
	}
}

type failingApplySource struct{ domain.Source }

func (s failingApplySource) Apply(kind, ns, name, body string) error { return errors.New("conflict") }
func TestEditorFailurePreservesFile(t *testing.T) {
	m := newTestModel(t, failingApplySource{mock.New("")})
	path := t.TempDir() + "/edit.yaml"
	if err := os.WriteFile(path, []byte("edited"), 0600); err != nil {
		t.Fatal(err)
	}
	_, cmd := m.Update(editExitMsg{kind: "pods", ns: "A", name: "same", path: path})
	if cmd != nil {
		if batch, ok := cmd().(tea.BatchMsg); ok {
			for _, c := range batch {
				m.Update(c())
			}
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("failed apply deleted recovery file: %v", err)
	}
	if !strings.Contains(m.toast, path) {
		t.Fatalf("no recovery path: %s", m.toast)
	}
}
func TestStaleEditorFetchCleanup(t *testing.T) {
	a := &targetSource{Source: mock.New("")}
	a.next = mock.New("")
	m := newTestModel(t, a)
	pending := m.startEdit("pods", "A", "same")
	m.Update(m.switchContextCmd("B")())
	if a.closes.Load() != 0 {
		t.Error("source closed before YAML fetch")
	}
	fetched := pending().(editFetchedMsg)
	defer func() { _ = os.Remove(fetched.path) }()
	_, cmd := m.Update(fetched)
	if cmd != nil {
		t.Error("stale fetch launched editor")
	}
	if _, err := os.Stat(fetched.path); !os.IsNotExist(err) {
		t.Errorf("stale seed leaked: %v", err)
	}
}
func TestEditorApplyCommandCannotRetarget(t *testing.T) {
	a := &targetSource{Source: mock.New("")}
	b := &targetSource{Source: mock.New("")}
	a.next = b
	m := newTestModel(t, a)
	path := t.TempDir() + "/edit.yaml"
	if err := os.WriteFile(path, []byte("edited"), 0600); err != nil {
		t.Fatal(err)
	}
	_, cmd := m.Update(editExitMsg{kind: "pods", ns: "A", name: "same", path: path})
	m.Update(m.switchContextCmd("B")())
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			c()
		}
	}
	if a.applies.Load() != 1 || b.applies.Load() != 0 {
		t.Fatalf("apply A=%d B=%d", a.applies.Load(), b.applies.Load())
	}
}
func TestEditorCannotApplyToDifferentSession(t *testing.T) {
	a := &targetSource{Source: mock.New("")}
	b := &targetSource{Source: mock.New("")}
	a.next = b
	m := newTestModel(t, a)
	fetched := m.startEdit("pods", "default", "same")().(editFetchedMsg)
	defer func() { _ = os.Remove(fetched.path) }()
	if err := os.WriteFile(fetched.path, []byte("user edited bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	m.Update(m.switchContextCmd("B")())
	_, cmd := m.Update(editExitMsg{kind: fetched.kind, ns: fetched.ns, name: fetched.name, path: fetched.path})
	if cmd != nil {
		if batch, ok := cmd().(tea.BatchMsg); ok {
			for _, c := range batch {
				c()
			}
		}
	}
	if b.applies.Load() != 0 || a.applies.Load() != 0 {
		t.Errorf("stale edit applied A=%d B=%d", a.applies.Load(), b.applies.Load())
	}
	data, err := os.ReadFile(fetched.path)
	if err != nil || string(data) != "user edited bytes" {
		t.Errorf("edited file lost: %s %v", data, err)
	}
	if !strings.Contains(m.toast, fetched.path) {
		t.Errorf("missing recovery path: %q", m.toast)
	}
}
