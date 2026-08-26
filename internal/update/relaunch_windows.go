//go:build windows

package update

import (
	"os"
	"os/exec"
)

// Relaunch starts path as a child sharing this console and waits for it,
// because Windows has no exec: the process cannot replace itself, so the
// caller exits once the child does.
func Relaunch(path string) error {
	cmd := exec.Command(path, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
