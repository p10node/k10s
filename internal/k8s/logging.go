package k8s

import (
	"flag"
	"io"

	"k8s.io/klog/v2"
)

// SilenceLogging stops client-go from writing to stderr.
//
// Its reflectors log every failed list/watch — a routine occurrence on
// RBAC-restricted, flaky, or partially unreachable clusters — and in a
// full-screen TUI those writes land directly on top of the rendered frame,
// corrupting the display and making the app look like it is freezing.
// Failures that matter reach the user as a toast instead.
//
// Call this once, before building any client.
func SilenceLogging() {
	fs := flag.NewFlagSet("klog", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	klog.InitFlags(fs)
	_ = fs.Set("logtostderr", "false")
	_ = fs.Set("alsologtostderr", "false")
	_ = fs.Set("stderrthreshold", "FATAL")
	klog.SetOutput(io.Discard)
	klog.LogToStderr(false)
}
