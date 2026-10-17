package sh

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func shellForTest(t *testing.T, name string) ShellSpec {
	t.Helper()
	if runtime.GOOS == "windows" && (name == "bash" || name == "sh" || name == "zsh") {
		t.Skip("Unix shell integration is supported on Linux/macOS; Windows uses native Cmd/PowerShell")
	}
	var spec ShellSpec
	var required bool
	switch name {
	case "bash":
		spec, required = Bash(), runtime.GOOS == "linux" || runtime.GOOS == "darwin"
	case "sh":
		spec, required = Sh(), runtime.GOOS == "linux" || runtime.GOOS == "darwin"
	case "zsh":
		spec, required = Zsh(), runtime.GOOS == "darwin"
	case "cmd":
		spec, required = CmdExe(), runtime.GOOS == "windows"
	case "pwsh":
		spec, required = Pwsh(), runtime.GOOS == "windows"
	default:
		t.Fatal("unknown shell", name)
	}
	path, err := exec.LookPath(spec.base)
	if err != nil {
		if required {
			t.Fatalf("required supported shell %s missing: %v", name, err)
		}
		t.Skipf("optional shell %s unavailable: %v", name, err)
	}
	spec.base = path // Every named-shell behavior also exercises absolute paths.
	versionArgs := []string{"--version"}
	if name == "sh" {
		versionArgs = []string{"-c", "printf 'POSIX sh behavior probe\\n'"}
	}
	if name == "cmd" {
		versionArgs = []string{"/D", "/C", "ver"}
	}
	version, err := exec.Command(path, versionArgs...).CombinedOutput() //nolint:gosec // Installed named shell under test.
	if err != nil {
		t.Fatalf("shell version: %v %s", err, version)
	}
	t.Logf("%s: %s", name, strings.SplitN(strings.TrimSpace(string(version)), "\n", 2)[0])
	return spec
}

func shellOutput(script string, spec ShellSpec) (string, string, Result) {
	var stdout, stderr bytes.Buffer
	result := spec.Shell(script).Stdout(&stdout).Stderr(&stderr).Exec()
	return strings.ReplaceAll(stdout.String(), "\r\n", "\n"), strings.ReplaceAll(stderr.String(), "\r\n", "\n"), result
}

func TestNamedShellSetupOrderCopiesAndStatus(t *testing.T) {
	for _, name := range []string{"bash", "sh", "zsh", "cmd", "pwsh"} {
		t.Run(name, func(t *testing.T) {
			spec := shellForTest(t, name)
			first, left, right, body := "printf 'first\\n'", "printf 'left\\n'", "printf 'right\\n'", "printf 'body\\n'"
			if name == "cmd" {
				first, left, right, body = "echo first", "echo left", "echo right", "echo body"
			}
			if name == "pwsh" {
				first, left, right, body = "Write-Output 'first'", "Write-Output 'left'", "Write-Output 'right'", "Write-Output 'body'"
			}
			base := spec.Setup(first)
			leftSpec, rightSpec := base.Setup(left), base.Setup(right)
			for _, fixture := range []struct {
				spec ShellSpec
				want string
			}{{leftSpec, "first\nleft\nbody\n"}, {rightSpec, "first\nright\nbody\n"}, {base, "first\nbody\n"}} {
				out, stderr, result := shellOutput(body, fixture.spec)
				if !result.OK() || out != fixture.want || stderr != "" {
					t.Fatalf("setup: output=%q stderr=%q result=%v", out, stderr, result.Error())
				}
			}
			exit := "exit 7"
			if name == "cmd" {
				exit = "exit /B 7"
			}
			_, _, result := shellOutput(exit, spec)
			if result.Code() != 7 || result.Error() == nil {
				t.Fatalf("exit status: %#v", result)
			}
		})
	}
}

func TestNamedShellEnvironmentCwdAndIO(t *testing.T) {
	for _, name := range []string{"bash", "sh", "zsh", "cmd", "pwsh"} {
		t.Run(name, func(t *testing.T) {
			spec := shellForTest(t, name)
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("GORN_SHELL_ENV", "inherited")
			script := `printf '%s\n' "$GORN_SHELL_ENV"; pwd; printf 'diagnostic' >&2`
			if name == "cmd" {
				// set /p prints without a newline but returns failure on EOF;
				// this IO fixture deliberately finishes with explicit success.
				script = `echo %GORN_SHELL_ENV%& cd & <nul set /p "=diagnostic" 1>&2&exit /B 0`
			}
			if name == "pwsh" {
				script = `Write-Output $env:GORN_SHELL_ENV; (Get-Location).Path; [Console]::Error.Write('diagnostic')`
			}
			var stdout, stderr bytes.Buffer
			base := spec.Shell(script).Dir(dir).Stdout(&stdout).Stderr(&stderr)
			result := base.Env("GORN_SHELL_ENV", "overridden").Exec()
			out := strings.ReplaceAll(stdout.String(), "\r\n", "\n")
			lines := strings.Split(strings.TrimSpace(out), "\n")
			if !result.OK() || len(lines) != 2 || strings.TrimSpace(lines[0]) != "overridden" || stderr.String() != "diagnostic" {
				t.Fatalf("env/IO: %q %q %v", out, stderr.String(), result.Error())
			}
			actual, err := os.Stat(strings.TrimSpace(lines[1]))
			want, statErr := os.Stat(dir)
			if err != nil || statErr != nil || !os.SameFile(actual, want) {
				t.Fatal("cwd", lines[1], dir, err, statErr)
			}
			stdout.Reset()
			stderr.Reset()
			if result := base.Exec(); !result.OK() || !strings.HasPrefix(stdout.String(), "inherited") {
				t.Fatal("Env mutated base", stdout.String(), result.Error())
			}
		})
	}
}

func TestNamedShellStdin(t *testing.T) {
	for _, name := range []string{"bash", "sh", "zsh", "cmd", "pwsh"} {
		t.Run(name, func(t *testing.T) {
			spec := shellForTest(t, name)
			script := `IFS= read -r line; printf '%s\n' "$line"`
			if name == "cmd" {
				script = `sort`
			}
			if name == "pwsh" {
				script = `[Console]::Out.WriteLine([Console]::ReadLine())`
			}
			var out, stderr bytes.Buffer
			result := spec.Shell(script).Stdin(strings.NewReader("input\n")).Stdout(&out).Stderr(&stderr).Exec()
			if !result.OK() || strings.ReplaceAll(out.String(), "\r\n", "\n") != "input\n" || stderr.Len() != 0 {
				t.Fatal("stdin", out.String(), stderr.String(), result.Error())
			}
		})
	}
}

func TestNamedShellStrictFailure(t *testing.T) {
	for _, name := range []string{"bash", "sh", "zsh", "pwsh"} {
		t.Run(name, func(t *testing.T) {
			spec := shellForTest(t, name)
			script := "false; printf 'after'"
			if name == "bash" || name == "zsh" {
				script = "false | true; printf 'after'"
			}
			if name == "pwsh" {
				script = "Write-Error 'expected'; Write-Output 'after'"
			}
			out, _, result := shellOutput(script, spec)
			if !result.OK() || !strings.Contains(out, "after") {
				t.Fatal("non-strict sequence", out, result.Error())
			}
			out, _, result = shellOutput(script, spec.Strict())
			if result.OK() || strings.Contains(out, "after") {
				t.Fatal("strict sequence continued", out, result.Error())
			}
		})
	}
}

func TestShellStartupFilesAreExplicit(t *testing.T) {
	for _, name := range []string{"bash", "zsh"} {
		t.Run(name, func(t *testing.T) {
			spec := shellForTest(t, name)
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			t.Setenv("ZDOTDIR", dir)
			t.Setenv("BASH_ENV", "")
			for _, file := range []string{".bash_profile", ".zprofile", ".zshenv"} {
				if err := os.WriteFile(filepath.Join(dir, file), []byte("printf 'PERSONAL_STARTUP\\n'\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			out, stderr, result := shellOutput("printf 'body'", spec)
			if !result.OK() || out != "body" || stderr != "" {
				t.Fatal("personal startup ran", out, stderr, result.Error())
			}
			if name == "bash" {
				t.Setenv("BASH_ENV", filepath.Join(dir, ".bash_profile"))
				out, _, result = shellOutput("printf 'body'", spec)
				if !result.OK() || out != "PERSONAL_STARTUP\nbody" {
					t.Fatal("inherited BASH_ENV ignored", out, result.Error())
				}
			}
		})
	}
}

func TestHostSelectionAndMissingShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Setenv("SHELL", "pwsh.exe")
		if Host().base != baseCmdExe {
			t.Fatal("Windows host used SHELL")
		}
	} else {
		for _, name := range []string{"bash", "zsh", "sh"} {
			path := filepath.Join("/custom", name)
			t.Setenv("SHELL", path)
			if Host().base != path {
				t.Fatal("host lost executable path", Host())
			}
		}
		t.Setenv("SHELL", "/custom/fish")
		if Host().base != basePosixSh {
			t.Fatal("unsupported host did not use sh")
		}
	}
	result := NewShell(filepath.Join(t.TempDir(), "absent-shell"), "-c").Exec("echo should-not-run")
	if result.OK() || result.Code() != 1 || result.Error() == nil {
		t.Fatal("missing shell", result)
	}
}
