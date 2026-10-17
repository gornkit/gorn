//nolint:gosec // Controlled native shell invocations and an executable copy in a private fixture.
package sh

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

func TestWindowsShellBasenamesAndStrict(t *testing.T) {
	spec := shellForTest(t, "pwsh")
	for _, name := range []string{strings.ToUpper(spec.base), filepath.ToSlash(spec.base)} {
		out, stderr, result := shellOutput("Write-Error 'expected'; Write-Output 'after'", NewShell(name, "-NoProfile", "-NonInteractive", "-Command").Strict())
		if result.OK() || strings.Contains(out, "after") {
			t.Fatal(name, out, stderr, result)
		}
	}
	out, stderr, result := shellOutput(`cmd.exe /D /C exit /B 7&echo after`, CmdExe().Strict())
	if !result.OK() || out != "after\n" || stderr != "" {
		t.Fatal("cmd strict must retain normal continuation", out, stderr, result)
	}
}

func TestWindowsCmdPreservesRawScriptAndQuotedExecutable(t *testing.T) {
	spec := shellForTest(t, "cmd")
	dir := filepath.Join(t.TempDir(), "native tool with spaces 世界")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "echo helper.exe")
	binary, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, binary, 0o700); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	script := `"` + path + `" -test.run=^TestCommandFixture$ -- "a b" "" "a&b"`
	result := spec.Shell(script).Env("GORN_COMMAND_FIXTURE", "echo").Stdout(&out).Stderr(&stderr).Exec()
	var actual []string
	if !result.OK() || json.Unmarshal(out.Bytes(), &actual) != nil || !reflect.DeepEqual(actual, []string{"a b", "", "a&b"}) {
		t.Fatal("raw cmd quoting", out.String(), stderr.String(), result.Error())
	}
	// /V:ON is explicit: command-string percent expansion occurs before Setup.
	custom := NewShell(spec.base, "/D", "/V:ON", "/S", "/C").Setup(`set "GORN_DELAYED=one two"`)
	text, diagnostic, result := shellOutput(`echo !GORN_DELAYED!`, custom)
	if !result.OK() || text != "one two\n" || diagnostic != "" {
		t.Fatal("cmd setup/delayed expansion", text, diagnostic, result.Error())
	}
	cmd := exec.Command(spec.base, "/D", "/S", "/C", "echo ok")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	configureShellCommand(cmd, spec, "echo ok")
	if !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CmdLine == "" {
		t.Fatal("shell transport discarded process attributes")
	}
}

func TestPowerShellNativeStatusIsExplicit(t *testing.T) {
	spec := shellForTest(t, "pwsh").Strict()
	path := strings.ReplaceAll(os.Args[0], "'", "''")
	invoke := `$env:GORN_COMMAND_FIXTURE='exit'; & '` + path + `' '-test.run=^TestCommandFixture$' '--'; `
	out, stderr, result := shellOutput(invoke+`Write-Output 'after'`, spec)
	if !result.OK() || out != "after\n" || stderr != "" {
		t.Fatal("strict invented native fail-fast", out, stderr, result.Error())
	}
	_, _, result = shellOutput(invoke+`exit $LASTEXITCODE`, spec)
	if result.Code() != 7 {
		t.Fatal("explicit native status lost", result)
	}
}
