package update

import (
	"strconv"
	"strings"
)

// Release versions are compared with the subset of semver that release tags
// actually use: an optional "v", dot-separated numbers, an optional "-"
// prerelease suffix, and build metadata after "+" that carries no ordering.
// Anything that doesn't parse — "dev" above all — sorts below every real
// version, which is what makes `just build` offer to install a release.

// Normalize strips the decorations two spellings of the same version
// disagree on: surrounding space, a leading "v", and "+build" metadata.
func Normalize(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	return v
}

// Compare orders two versions: -1 if a sorts before b, +1 if after, 0 if
// they are the same release.
func Compare(a, b string) int {
	an, bn := Normalize(a), Normalize(b)
	if an == bn {
		return 0
	}

	aNum, aPre, aOK := split(an)
	bNum, bPre, bOK := split(bn)

	// An unparseable version ("dev", "main", a branch name) is older than
	// any real one; two of them fall back to a stable string order so the
	// comparison is still total.
	switch {
	case !aOK && !bOK:
		return strings.Compare(an, bn)
	case !aOK:
		return -1
	case !bOK:
		return 1
	}

	if c := compareNums(aNum, bNum); c != 0 {
		return c
	}
	return comparePre(aPre, bPre)
}

// Newer reports whether candidate is a release worth installing over
// current.
func Newer(current, candidate string) bool {
	return Compare(current, candidate) < 0
}

// split separates "1.4.0-rc.1" into its numeric parts and its prerelease
// tail. ok is false when the numeric head isn't numeric at all.
func split(v string) (nums []int, pre string, ok bool) {
	head := v
	if i := strings.IndexByte(v, '-'); i >= 0 {
		head, pre = v[:i], v[i+1:]
	}
	if head == "" {
		return nil, "", false
	}
	for _, p := range strings.Split(head, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, "", false
		}
		nums = append(nums, n)
	}
	return nums, pre, true
}

// compareNums treats a missing part as 0, so "1.4" and "1.4.0" are equal.
func compareNums(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		x, y := 0, 0
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return sign(x - y)
		}
	}
	return 0
}

// comparePre implements the one prerelease rule that matters in practice:
// having a prerelease suffix sorts *below* not having one, so 1.4.0 beats
// 1.4.0-rc.1. Two suffixes compare identifier by identifier, numerically
// where both are numbers.
func comparePre(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}
	ap, bp := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(ap) || i < len(bp); i++ {
		if i >= len(ap) {
			return -1 // fewer identifiers sorts first
		}
		if i >= len(bp) {
			return 1
		}
		x, xerr := strconv.Atoi(ap[i])
		y, yerr := strconv.Atoi(bp[i])
		switch {
		case xerr == nil && yerr == nil:
			if x != y {
				return sign(x - y)
			}
		case xerr == nil:
			return -1 // numeric identifiers sort below alphanumeric ones
		case yerr == nil:
			return 1
		default:
			if c := strings.Compare(ap[i], bp[i]); c != 0 {
				return c
			}
		}
	}
	return 0
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	if n > 0 {
		return 1
	}
	return 0
}
