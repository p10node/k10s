//go:build race

package ui

// raceEnabled is true when the race detector is instrumenting the build.
// Timing assertions are meaningless then: every memory access goes through
// the detector, so wall-clock budgets measure the tooling, not the code.
const raceEnabled = true
