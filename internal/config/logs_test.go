package config

import (
	"strings"
	"testing"
)

func TestLogRetentionConfigRoundTrip(t *testing.T) {
	var c Config
	parse("logs:\n  max_lines: 1234\n  max_bytes: 234567\n", &c)
	got := render(c)
	if !strings.Contains(got, "max_lines: 1234") || !strings.Contains(got, "max_bytes: 234567") {
		t.Fatal("log retention configuration lost during roundtrip")
	}
}
