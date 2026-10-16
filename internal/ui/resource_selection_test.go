package ui

import (
	"github.com/p10node/k10s/internal/domain"
	"github.com/p10node/k10s/internal/mock"
	"testing"
)

type identityUISource struct {
	domain.Source
	rows  []domain.ResourceRow
	calls []domain.ResourceRef
}

func (s *identityUISource) CustomResourceRows(string) ([]string, []domain.ResourceRow) {
	return []string{"NAME", "KIND", "API"}, s.rows
}
func (s *identityUISource) DescribeResource(r domain.ResourceRef) (string, error) {
	s.calls = append(s.calls, r)
	return r.Group, nil
}
func (s *identityUISource) YAMLResource(r domain.ResourceRef) (string, error) {
	s.calls = append(s.calls, r)
	return r.Group, nil
}
func (s *identityUISource) DeleteResource(r domain.ResourceRef) error {
	s.calls = append(s.calls, r)
	return nil
}
func (s *identityUISource) ApplyResource(r domain.ResourceRef, _ string) error {
	s.calls = append(s.calls, r)
	return nil
}
func identityUIFixture(t *testing.T) (*Model, *identityUISource) {
	t.Helper()
	s := &identityUISource{Source: mock.New("")}
	for _, g := range []string{"alpha.test", "beta.test"} {
		s.rows = append(s.rows, domain.ResourceRow{Cells: []string{"shared", "Widget", g}, Ref: domain.ResourceRef{Group: g, Version: "v1", Resource: "widgets", Kind: "Widget", Namespaced: true, Namespace: "default", Name: "shared", UID: g}})
	}
	m := newTestModel(t, s)
	m.jumpToResource("customresources")
	return m, s
}
func TestCustomResourceTableFiltersReferencesTogether(t *testing.T) {
	m, _ := identityUIFixture(t)
	m.rowSearch = "beta.test"
	_, rows := m.tableData()
	if len(rows) != 1 || rows[0][2] != "beta.test" {
		t.Fatalf("did not use typed snapshot: %v", rows)
	}
}
func TestCustomResourceDeleteUsesFilteredReference(t *testing.T) {
	m, s := identityUIFixture(t)
	m.rowSearch = "beta.test"
	m.fireAction(Action{ID: domain.ADelete, Label: "Delete"})
	if m.confirm == nil {
		t.Fatal("delete confirmation missing")
	}
	cmd := m.confirm.onOK(m)
	if cmd == nil {
		t.Fatal("delete command missing")
	}
	cmd()
	if len(s.calls) != 1 || s.calls[0].Group != "beta.test" {
		t.Fatalf("delete did not retain exact selected identity: %v", s.calls)
	}
}

func TestCustomResourceSelectionSurvivesCacheReorder(t *testing.T) {
	m, s := identityUIFixture(t)
	m.rowIdx = 1
	m.tableData()
	s.rows[0], s.rows[1] = s.rows[1], s.rows[0]
	m.tableData()
	if m.rowIdx != 0 {
		t.Fatalf("selected row changed identity during reorder: index %d", m.rowIdx)
	}
}
