package sh

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func requireBash(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("bash test")
	}
	if _, err := exec.LookPath(baseBash); err != nil {
		t.Skip("bash not found")
	}
}

func TestHostKeepsShellPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SHELL is not used on Windows")
	}

	t.Setenv("SHELL", "/usr/local/bin/bash")

	got := Host()
	if got.base != "/usr/local/bin/bash" || len(got.args) != 1 || got.args[0] != "-lc" {
		t.Fatalf("Host() = %#v, want bash path with -lc", got)
	}
}

func TestStrictUsesShellSetup(t *testing.T) {
	tests := []struct {
		name string
		in   ShellSpec
		want string
	}{
		{name: "bash", in: Bash(), want: "set -euo pipefail"},
		{name: "zsh", in: Zsh(), want: "set -euo pipefail"},
		{name: "sh", in: Sh(), want: "set -eu"},
		{name: "pwsh", in: Pwsh(), want: "$ErrorActionPreference = 'Stop'"},
		{name: "path", in: NewShell("/usr/local/bin/bash", "-lc"), want: "set -euo pipefail"},
	}

	for _, tt := range tests {
		got := tt.in.Strict()
		if len(got.setupLines) != 1 || got.setupLines[0] != tt.want {
			t.Fatalf("%s: Strict() setup = %#v, want %q", tt.name, got.setupLines, tt.want)
		}
	}
}

func TestSetupDoesNotAlias(t *testing.T) {
	base := Bash().Setup("one")

	left := base.Setup("left")
	right := base.Setup("right")

	if left.setupLines[1] != "left" || right.setupLines[1] != "right" {
		t.Fatalf("Setup aliases backing slices: left %#v, right %#v", left.setupLines, right.setupLines)
	}
}

func TestStrictRunsPipeline(t *testing.T) {
	requireBash(t)

	var out bytes.Buffer
	got := Bash().Strict().
		Shell("echo \"hello\nworld\" | wc -l").
		Stdout(&out).
		Exec()

	if !got.OK() {
		t.Fatalf("strict pipeline failed: code %d, err %v", got.Code(), got.Error())
	}
	if strings.TrimSpace(out.String()) != "2" {
		t.Fatalf("wc output = %q, want 2", out.String())
	}
}

func TestStrictCatchesPipelineError(t *testing.T) {
	requireBash(t)

	if got := Bash().Exec("false | true"); !got.OK() {
		t.Fatalf("non-strict pipeline failed: code %d, err %v", got.Code(), got.Error())
	}
	if got := Bash().Strict().Exec("false | true"); got.OK() {
		t.Fatal("strict pipeline succeeded, want pipefail error")
	}
}

func TestCommandIOEnvAndDir(t *testing.T) {
	requireBash(t)

	dir := t.TempDir()
	// The shell's pwd reports the symlink-resolved path (e.g. macOS maps
	// /var -> /private/var), while t.TempDir() does not, so compare against
	// the resolved form.
	wantDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	got := Bash().
		Shell(`read line; printf "out:%s:%s:%s" "$line" "$GORN_TEST_VALUE" "$(pwd)"; printf "err:%s" "$line" >&2`).
		Dir(dir).
		Env("GORN_TEST_VALUE", "env").
		Stdin(strings.NewReader("stdin\n")).
		Stdout(&stdout).
		Stderr(&stderr).
		Exec()

	if !got.OK() {
		t.Fatalf("Exec() failed: code %d, err %v", got.Code(), got.Error())
	}
	if stdout.String() != "out:stdin:env:"+wantDir {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.String() != "err:stdin" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestStartKillWait(t *testing.T) {
	requireBash(t)

	proc, err := Bash().Shell("sleep 10").Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}
	if got := proc.Wait(); got.OK() {
		t.Fatal("Wait() after Kill() succeeded, want failure")
	}
}

func TestExecReportsExitCode(t *testing.T) {
	script := "exit 7"
	if runtime.GOOS == "windows" {
		script = "exit /B 7"
	}

	got := Host().Exec(script)
	if got.OK() || got.Code() != 7 || got.Error() == nil {
		t.Fatalf("Exec(exit 7) = OK %v, code %d, err %v", got.OK(), got.Code(), got.Error())
	}
}
