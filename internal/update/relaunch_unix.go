//go:build !windows

package update

import (
	"os"
	"syscall"
)

// Relaunch replaces this process with a fresh run of path, keeping the same
// arguments, environment and terminal. Exec is what makes the restart
// seamless: no second process, no lost TTY, and nothing to clean up — on
// success this function does not return.
func Relaunch(path string) error {
	args := append([]string{path}, os.Args[1:]...)
	return syscall.Exec(path, args, os.Environ())
}
