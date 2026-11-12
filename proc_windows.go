//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// hideWindow stops Audiveris from flashing a console window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// restartAs starts the new version in this console and ends this one.
func restartAs(exe string, args, env []string) error {
	cmd := exec.Command(exe, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
