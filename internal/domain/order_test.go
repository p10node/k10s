package domain

import "testing"

func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"api", "web", true},
		{"web", "api", false},
		{"API", "web", true},      // case-insensitive
		{"pod-2", "pod-10", true}, // numeric, not lexicographic
		{"pod-10", "pod-2", false},
		{"pod-002", "pod-3", true}, // leading zeros ignored
		{"a", "ab", true},          // prefix sorts first
		{"same", "same", false},
	}
	for _, c := range cases {
		if got := NaturalLess(c.a, c.b); got != c.want {
			t.Errorf("NaturalLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
