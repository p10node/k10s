package ui

import (
	"fmt"
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
)

type typedEditSource struct{ *identityUISource }

func (s typedEditSource) YAMLResource(r domain.ResourceRef) (string, error) {
	return fmt.Sprintf("apiVersion: %s/%s\nkind: %s\nmetadata:\n  name: %s\n  namespace: %s\n  uid: %s\n  resourceVersion: '1'\nspec: {}\n", r.Group, r.Version, r.Kind, r.Name, r.Namespace, r.UID), nil
}

func TestComposedCustomResourceEditKeepsReference(t *testing.T) {
	t.Setenv("EDITOR", "'")
	_, base := identityUIFixture(t)
	s := typedEditSource{base}
	m := newTestModel(t, s)
	m.jumpToResource("customresources")
	m.rowSearch = "beta.test"
	fetched := m.startEdit("customresources", "default", "shared")().(editFetchedMsg)
	if fetched.err != nil {
		t.Fatal(fetched.err)
	}
	defer func() { _ = os.Remove(fetched.path) }()
	m.rowSearch = "alpha.test"
	_, cmd := m.Update(fetched)
	exit := cmd().(editExitMsg)
	exit.err = nil
	_, cmd = m.Update(exit)
	if cmd != nil {
		if batch, ok := cmd().(tea.BatchMsg); ok {
			for _, c := range batch {
				m.Update(c())
			}
		}
	}
	if len(s.calls) != 1 || s.calls[0].Group != "beta.test" {
		t.Fatalf("edit lost original GVR: calls=%v toast=%s", s.calls, m.toast)
	}
}

func TestSourceAcceptanceInvalidatesInterimRequests(t *testing.T) {
	a := &targetSource{Source: mock.New("")}
	a.next = mock.New("")
	m := newTestModel(t, a)
	switching := m.switchContextCmd("B")
	interim := m.runFetch("old backend", func() (string, error) { return "must not appear", nil })
	m.Update(switching())
	m.Update(interim())
	if m.mode == modeText {
		t.Fatal("request started while switch pending reopened old backend view")
	}
}
