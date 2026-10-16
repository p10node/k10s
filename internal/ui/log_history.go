package ui

// prependLogHistory finds an ordered overlap, not a length difference. Live
// lines can arrive during the history request; repeated log messages must not
// be deduplicated as a set. Ambiguous/no overlap leaves the live view intact.
func prependLogHistory(history, current []string) ([]string, bool) {
	if len(current) == 0 {
		return current, false
	}
	prefix := make([]int, len(current))
	for i, j := 1, 0; i < len(current); i++ {
		for j > 0 && current[i] != current[j] {
			j = prefix[j-1]
		}
		if current[i] == current[j] {
			j++
		}
		prefix[i] = j
	}
	best, start, matches, j := 0, 0, 0, 0
	for i, line := range history {
		for j > 0 && line != current[j] {
			j = prefix[j-1]
		}
		if line == current[j] {
			j++
		}
		if j > best {
			best, start, matches = j, i-j+1, 1
		} else if j > 0 && j == best {
			matches++
		}
		if j == len(current) {
			j = prefix[j-1]
		}
	}
	if best == 0 || matches != 1 {
		return current, false
	}
	out := make([]string, 0, start+len(current))
	out = append(out, history[:start]...)
	out = append(out, current...)
	return out, true
}
