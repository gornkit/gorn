package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func builtCLI(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds installed-style CLI")
	}
	path := filepath.Join(t.TempDir(), "gorn")
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", path, ".") //nolint:gosec // G204: test-built CLI in a controlled temporary directory.
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	return path
}

func invokeCLI(t *testing.T, bin, cwd, input string, args ...string) runResult {
	t.Helper()
	cmd := exec.Command(bin, args...) //nolint:gosec // G204: test-built CLI with controlled fixture arguments.
	cmd.Dir = cwd
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var exited *exec.ExitError
	if errors.As(err, &exited) {
		code = exited.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return runResult{stdout: stdout.String(), stderr: stderr.String(), code: code, err: err}
}

func writeScript(t *testing.T, dir, name, source string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInstalledCLIExitAndArgumentContract(t *testing.T) {
	bin := builtCLI(t)
	dir := t.TempDir()
	t.Setenv("GORN_CACHE", filepath.Join(dir, "cache"))
	statusScript := writeScript(t, dir, "status.gorn", `package main
import("fmt";"os";"strconv")
func main(){code,_:=strconv.Atoi(os.Args[1]);fmt.Print("out");fmt.Fprint(os.Stderr,"err");os.Exit(code)}`)
	for _, prefix := range [][]string{{"run"}, {}} {
		for _, code := range []int{0, 7, 2} {
			got := invokeCLI(t, bin, dir, "", append(append([]string(nil), prefix...), statusScript, fmt.Sprint(code))...)
			if got.code != code || got.stdout != "out" || got.stderr != "err" {
				t.Fatalf("prefix=%v code=%d: %+v", prefix, code, got)
			}
		}
	}
	for _, flag := range []string{"--help", "--version"} {
		var explicit string
		for _, prefix := range [][]string{{"run"}, {}} {
			got := invokeCLI(t, bin, dir, "", append(append([]string(nil), prefix...), statusScript, flag)...)
			if got.code != 0 || got.stderr != "" {
				t.Fatalf("runner flag %s prefix=%v: %+v", flag, prefix, got)
			}
			if flag == "--version" {
				if explicit == "" {
					explicit = got.stdout
				} else if got.stdout != explicit {
					t.Fatalf("version parity: %q != %q", got.stdout, explicit)
				}
			} else if !strings.Contains(got.stdout, "Usage:") {
				t.Fatalf("missing help: %+v", got)
			}
		}
	}
	argsScript := writeScript(t, dir, "args 世界.gorn", `package main
import("fmt";"io";"os")
func main(){cwd,_:=os.Getwd();data,_:=io.ReadAll(os.Stdin);fmt.Printf("%q\n%s\n%s",os.Args[1:],cwd,data)}`)
	wantDir, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	cwds := []string{dir}
	alias := filepath.Join(t.TempDir(), "cwd-alias")
	if err := os.Symlink(dir, alias); err == nil {
		cwds = append(cwds, alias)
	} else {
		t.Logf("cwd symlink unavailable: %v", err)
	}
	for _, cwd := range cwds {
		for _, prefix := range [][]string{{"run"}, {}} {
			got := invokeCLI(t, bin, cwd, "runtime input", append(append([]string(nil), prefix...), argsScript, "--", "--flag", "two words", "", "--", "-v")...)
			parts := strings.SplitN(got.stdout, "\n", 3)
			if got.code != 0 || got.stderr != "" || len(parts) != 3 || parts[0] != "[\"--flag\" \"two words\" \"\" \"--\" \"-v\"]" || parts[2] != "runtime input" {
				t.Fatalf("arguments prefix=%v cwd=%s: %+v", prefix, cwd, got)
			}
			// cwd spelling may be logical, canonical, or Windows short-name form.
			// Directory identity proves that the runner preserved invocation cwd.
			actualDir, err := os.Stat(parts[1])
			if err != nil || !os.SameFile(wantDir, actualDir) {
				t.Fatalf("runtime cwd=%q differs from invocation directory: %v", parts[1], err)
			}
		}
	}
	for _, args := range [][]string{{"run"}, {"-v", "run"}, {"--verbose", "run"}, {"-v=false", "run"}, {"run", "--unknown"}, {"--unknown"}} {
		got := invokeCLI(t, bin, dir, "", args...)
		if got.code != 2 || got.stdout != "" || !strings.Contains(got.stderr, "Usage:") || strings.Contains(got.stderr, "stat run") {
			t.Fatalf("usage args=%v: %+v", args, got)
		}
	}
	for _, prefix := range [][]string{{"run"}, {}} {
		got := invokeCLI(t, bin, dir, "", append(append([]string(nil), prefix...), "missing.gorn")...)
		if got.code != 1 || got.stdout != "" || strings.Contains(got.stderr, "Usage:") {
			t.Fatalf("source error prefix=%v: %+v", prefix, got)
		}
	}
	writeScript(t, dir, "stdin.gorn", "package main;func main(){panic(\"real file must not be used\")}")
	got := invokeCLI(t, bin, dir, "package main\nimport(\"fmt\";\"io\";\"os\")\nfunc main(){data,_:=io.ReadAll(os.Stdin);fmt.Printf(\"remaining=%d\",len(data))}", "run", "-")
	if got.code != 0 || got.stdout != "remaining=0" {
		t.Fatalf("stdin source: %+v", got)
	}
	got = invokeCLI(t, bin, dir, "package main\nfunc main(){undefined()}", "run", "-")
	if got.code != 1 || !strings.Contains(got.stderr, "stdin.gorn:2:") || strings.Contains(got.stderr, "/dev/stdin") {
		t.Fatalf("stdin diagnostics: %+v", got)
	}
}

func TestInstalledCLISignalExitStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signal status; Windows uses native process exit codes")
	}
	bin := builtCLI(t)
	dir := t.TempDir()
	t.Setenv("GORN_CACHE", filepath.Join(dir, "cache"))
	path := writeScript(t, dir, "signal.gorn", `package main
import("os";"syscall";"time")
func main(){_ = syscall.Kill(os.Getpid(),syscall.SIGTERM);time.Sleep(time.Second)}`)
	got := invokeCLI(t, bin, dir, "", "run", path)
	if got.code != 143 || got.stdout != "" || got.stderr != "" {
		t.Fatalf("signal status: %+v", got)
	}
}
