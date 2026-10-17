//go:build !windows

package sh

import "os/exec"

func configureShellCommand(_ *exec.Cmd, _ ShellSpec, _ string) {}
