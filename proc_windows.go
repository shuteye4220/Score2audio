//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// hideWindow stops Audiveris from flashing a console window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
