package ui

import "github.com/p10node/k10s/internal/domain"

// Action is one button of the right-hand Actions pane.
type Action struct {
	ID    string
	Key   string
	Label string
	Icon  string
	Risky bool
}

// offers reports whether the Actions pane lists action id for kind r: the
// kind has to support it, and in read-only mode it has to be one that only
// reads.
func (m *Model) offers(r domain.Kind, id string) bool {
	return r.Can(id) && (!m.readOnly || domain.ReadOnlyAllows(id))
}

var Actions = []Action{
	{domain.ADescribe, "d", "Describe", "󰈙", false},
	{domain.AYAML, "y", "YAML", "󰈮", false},
	{domain.ALogs, "l", "Logs", "󰑍", false},
	{domain.AShell, "s", "Shell", "", false},
	// "S" quits k10s into the same shell in the real terminal.
	{domain.AShellOut, "S", "Shell (terminal)", "", false},
	// "p" for port-forward: "f" is now the find/search key.
	{domain.APortFwd, "p", "Port Forward", "󰛳", false},
	{domain.ARestart, "r", "Rollout Restart", "󰑐", false},
	{domain.AScale, "c", "Scale", "󰡎", false},
	{domain.AEdit, "e", "Edit", "", false},
	{domain.ATop, "m", "Top (metrics)", "󰓅", false},
	{domain.ACordon, "o", "Cordon", "󰇙", false},
	{domain.ADrain, "u", "Drain", "󰗇", true},
	{domain.ADelete, "D", "Delete", "󰆴", true},
}
