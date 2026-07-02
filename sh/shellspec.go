package sh

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	baseBash    string = "bash"
	baseZsh     string = "zsh"
	basePosixSh string = "sh"
	baseCmdExe  string = "cmd.exe"
	basePwsh    string = "pwsh"
)

// Host returns the user's host shell: $SHELL on Unix when it is bash, zsh, or sh;
// otherwise cmd.exe on Windows and sh on Unix.
func Host() ShellSpec { return getHostSpec() }

// Bash returns a shell spec for bash -lc.
func Bash() ShellSpec { return NewShell(baseBash, "-lc") }

// Zsh returns a shell spec for zsh -lc.
func Zsh() ShellSpec { return NewShell(baseZsh, "-lc") }

// Sh returns a shell spec for sh -c.
func Sh() ShellSpec { return NewShell(basePosixSh, "-c") }

// CmdExe returns a shell spec for cmd.exe /C.
func CmdExe() ShellSpec { return NewShell(baseCmdExe, "/C") }

// Pwsh returns a shell spec for pwsh -Command.
func Pwsh() ShellSpec { return NewShell(basePwsh, "-Command") }

// NewShell returns a shell spec that invokes base with args before the script.
func NewShell(base string, args ...string) ShellSpec {
	return ShellSpec{base: base, args: append([]string(nil), args...)}
}

// ShellSpec describes how to invoke a shell and which setup lines to prepend.
type ShellSpec struct {
	base       string
	args       []string
	setupLines []string
}

// Setup returns a copy that prepends lines before each script.
func (s ShellSpec) Setup(lines ...string) ShellSpec {
	setupLines := make([]string, 0, len(s.setupLines)+len(lines))
	setupLines = append(setupLines, s.setupLines...)
	setupLines = append(setupLines, lines...)
	s.setupLines = setupLines
	return s
}

// Strict returns a copy with fail-fast setup for known shells.
func (s ShellSpec) Strict() ShellSpec {
	switch filepath.Base(s.base) {
	case baseBash, baseZsh:
		return s.Setup("set -euo pipefail")
	case basePosixSh:
		return s.Setup("set -eu")
	case basePwsh:
		return s.Setup("$ErrorActionPreference = 'Stop'")
	default:
		return s
	}
}

// Shell returns a configured command for script without starting it.
func (s ShellSpec) Shell(script string) Command {
	return Command{shell: s, script: script}
}

// Exec runs script and waits for it to finish.
func (s ShellSpec) Exec(script string) Result {
	return s.Shell(script).Exec()
}

func getHostSpec() ShellSpec {
	switch runtime.GOOS {
	case "windows":
		return CmdExe()
	default:
		envShell := os.Getenv("SHELL")
		if envShell != "" {
			switch filepath.Base(envShell) {
			case baseBash, baseZsh:
				return NewShell(envShell, "-lc")
			case basePosixSh:
				return NewShell(envShell, "-c")
			}
		}
		return Sh()
	}
}
