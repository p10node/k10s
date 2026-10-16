package ui

import (
	"fmt"
	"strings"

	"github.com/p10node/k10s/internal/domain"
)

type resourceSelection struct {
	rows           []domain.ResourceRow
	selected       domain.ResourceRef
	index          int
	scope, query   string
	valid, missing bool
}

func sameResource(a, b domain.ResourceRef) bool {
	return a.Group == b.Group && a.Version == b.Version && a.Resource == b.Resource && a.Namespace == b.Namespace && a.Name == b.Name && a.UID == b.UID
}

func (m *Model) customResourceTable(src domain.CustomResourceSource) ([]string, [][]string) {
	cols, snapshot := src.CustomResourceRows(m.namespace)
	rows := make([]domain.ResourceRow, 0, len(snapshot))
	q := strings.ToLower(m.rowSearch)
	for _, r := range snapshot {
		if rowMatches(r.Cells, q) {
			rows = append(rows, r)
		}
	}
	state := &m.crSelection
	preserve := state.valid && state.index == m.rowIdx && state.scope == m.namespace && state.query == m.rowSearch
	if preserve {
		state.missing = true
		for i, r := range rows {
			if sameResource(r.Ref, state.selected) {
				m.rowIdx = i
				state.missing = false
				break
			}
		}
	} else {
		state.missing = false
	}
	state.rows, state.scope, state.query = rows, m.namespace, m.rowSearch
	state.index = m.rowIdx
	if !state.missing && m.rowIdx >= 0 && m.rowIdx < len(rows) {
		state.selected = rows[m.rowIdx].Ref
		state.valid = true
	} else if !preserve {
		state.valid = false
	}
	cells := make([][]string, len(rows))
	for i, r := range rows {
		cells[i] = r.Cells
	}
	return cols, cells
}

// selectedActionSource binds CR reads/writes to the exact visible row. Other
// source capabilities remain on the original backend; this adapter only lives
// for one operation and must never replace Model.src.
func (m *Model) selectedActionSource(kind string) (domain.Source, error) {
	if kind != "customresources" {
		return m.src, nil
	}
	typed, ok := m.src.(domain.CustomResourceSource)
	if !ok {
		return nil, fmt.Errorf("backend cannot identify this custom resource safely")
	}
	m.tableData()
	if !m.crSelection.valid || m.crSelection.missing {
		return nil, fmt.Errorf("selected resource changed; select it again")
	}
	return &resourceSource{Source: m.src, typed: typed, ref: m.crSelection.selected}, nil
}

type resourceSource struct {
	domain.Source
	typed domain.CustomResourceSource
	ref   domain.ResourceRef
}

func (s *resourceSource) Describe(_, _, _ string) (string, error) {
	return s.typed.DescribeResource(s.ref)
}
func (s *resourceSource) YAML(_, _, _ string) (string, error) { return s.typed.YAMLResource(s.ref) }
func (s *resourceSource) Delete(_, _, _ string) error         { return s.typed.DeleteResource(s.ref) }
func (s *resourceSource) Apply(_, _, _, manifest string) error {
	return s.typed.ApplyResource(s.ref, manifest)
}
