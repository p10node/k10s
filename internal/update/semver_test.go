package update

import "testing"

func TestCompareOrdersReleases(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"v1.0.0", "1.0.0", 0},       // the "v" is decoration, not version
		{"1.4", "1.4.0", 0},          // a missing part is zero
		{"1.0.0+build7", "1.0.0", 0}, // build metadata carries no order
		{"1.0.0", "1.0.1", -1},
		{"1.9.0", "1.10.0", -1}, // string order would get this backwards
		{"2.0.0", "1.99.99", 1},
		{"1.0.0-rc.1", "1.0.0", -1}, // a prerelease is below the release
		{"1.0.0-rc.1", "1.0.0-rc.2", -1},
		{"1.0.0-rc.2", "1.0.0-rc.10", -1}, // numeric identifiers compare numerically
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-rc", "1.0.0-rc.1", -1}, // fewer identifiers sort first
		{"dev", "1.0.0", -1},           // a source build is older than every release
		{"dev", "dev", 0},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := Compare(c.b, c.a); got != -c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d (not antisymmetric)", c.b, c.a, got, -c.want)
		}
	}
}

func TestNewerTreatsSourceBuildsAsOutdated(t *testing.T) {
	// An unstamped build must be offered the release, otherwise `just build`
	// followed by /update would report "up to date" forever.
	if !Newer("dev", "1.0.0") {
		t.Error("Newer(dev, 1.0.0) = false, want true")
	}
	if Newer("1.0.0", "1.0.0") {
		t.Error("Newer of an identical version = true, want false")
	}
	if Newer("1.2.0", "1.1.9") {
		t.Error("Newer of an older candidate = true, want false")
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"  v1.2.3 ":   "1.2.3",
		"1.2.3+abc":   "1.2.3",
		"v0.1.0-rc.1": "0.1.0-rc.1",
		"dev":         "dev",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
