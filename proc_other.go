//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func hideWindow(cmd *exec.Cmd) {}

// restartAs replaces this program with the new version, keeping the same
// process, so a terminal it was started from stays attached to it.
func restartAs(exe string, args, env []string) error {
	return syscall.Exec(exe, append([]string{exe}, args...), env)
}
