package ui

import "k10s/internal/domain"

// Action is one button of the right-hand Actions pane.
type Action struct {
	ID    string
	Key   string
	Label string
	Icon  string
	Risky bool
}

var Actions = []Action{
	{domain.ADescribe, "d", "Describe", "󰈙", false},
	{domain.AYAML, "y", "YAML", "󰈮", false},
	{domain.ALogs, "l", "Logs", "󰑍", false},
	{domain.AShell, "s", "Shell", "", false},
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
