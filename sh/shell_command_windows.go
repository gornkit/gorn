package sh

import (
	"os/exec"
	"strings"
	"syscall"
)

// Cmd's script syntax is not CommandLineToArgvW quoting. Keep the complete
// caller-authored script raw inside one outer quote pair, consumed by /S /C.
// Custom cmd specs ending in /C or /K receive the same script transport.
func configureShellCommand(cmd *exec.Cmd, shell ShellSpec, script string) {
	if shellBase(shell.base) != baseCmdExe || len(shell.args) == 0 {
		return
	}
	last := shell.args[len(shell.args)-1]
	if !strings.EqualFold(last, "/C") && !strings.EqualFold(last, "/K") {
		return
	}
	parts := []string{syscall.EscapeArg(cmd.Path)}
	for _, arg := range shell.args {
		parts = append(parts, syscall.EscapeArg(arg))
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CmdLine = strings.Join(parts, " ") + ` "` + script + `"`
}
