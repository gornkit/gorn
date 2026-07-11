package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

type runResult struct {
	stdout string
	stderr string
	code   int
	err    error
}

type exitCode int

func runCLIForTest(t *testing.T, args ...string) (result runResult) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	result.code = -1
	defer func() {
		result.stdout = stdout.String()
		result.stderr = stderr.String()
		if v := recover(); v != nil {
			code, ok := v.(exitCode)
			if !ok {
				panic(v)
			}
			result.code = int(code)
		}
	}()

	result.err = RunCLI(args, &stdout, &stderr, func(code int) {
		panic(exitCode(code))
	})
	return result
}

func withStdin(t *testing.T, content string) func() {
	t.Helper()

	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r

	go func() {
		_, _ = io.WriteString(w, content)
		_ = w.Close()
	}()

	return func() {
		os.Stdin = old
		_ = r.Close()
	}
}

func TestRunCLIVersion(t *testing.T) {
	got := runCLIForTest(t, "--version")
	if got.code != 0 {
		t.Fatalf("exit code = %d, want 0", got.code)
	}
	if strings.TrimSpace(got.stdout) != (CLI{}).Version() {
		t.Fatalf("version output = %q", got.stdout)
	}
}

func TestRunCLIHelp(t *testing.T) {
	top := runCLIForTest(t)
	if top.err != nil {
		t.Fatal(top.err)
	}
	if !strings.Contains(top.stdout, "Commands:") {
		t.Fatalf("top help missing commands:\n%s", top.stdout)
	}

	run := runCLIForTest(t, "run", "--help")
	if run.code != 0 {
		t.Fatalf("exit code = %d, want 0", run.code)
	}
	usage := usageLine(run.stdout)
	for _, token := range []string{"--print-mod", "script", "args"} {
		if !strings.Contains(usage, token) {
			t.Fatalf("usage %q missing %q", usage, token)
		}
	}
	if strings.Index(usage, "--print-mod") > strings.Index(usage, "script") {
		t.Fatalf("flag appears after args in usage: %q", usage)
	}
}

// TestRunCLIExplicitAndShorthandRunMatch checks that `gorn run x` and the
// shorthand `gorn x` dispatch identically. Uses --print-main so the check
// covers dispatch/flag routing without depending on a real toolchain build.
func TestRunCLIExplicitAndShorthandRunMatch(t *testing.T) {
	explicit := runCLIForTest(t, "run", "--print-main", "testdata/run_hello.gorn", "--", "--flag")
	shorthand := runCLIForTest(t, "--print-main", "testdata/run_hello.gorn", "--", "--flag")
	if explicit.err != nil {
		t.Fatal(explicit.err)
	}
	if shorthand.err != nil {
		t.Fatal(shorthand.err)
	}
	if explicit.stdout != shorthand.stdout || explicit.stderr != shorthand.stderr {
		t.Fatalf("explicit and shorthand output differ:\nexplicit out=%q err=%q\nshorthand out=%q err=%q",
			explicit.stdout, explicit.stderr, shorthand.stdout, shorthand.stderr)
	}
}

func TestRunCLIVerboseAppliesToRunAndShorthand(t *testing.T) {
	explicit := runCLIForTest(t, "--verbose", "run", "--print-main", "testdata/clean.gorn")
	shorthand := runCLIForTest(t, "--verbose", "--print-main", "testdata/clean.gorn")
	plain := runCLIForTest(t, "run", "--print-main", "testdata/clean.gorn")
	for _, r := range []runResult{explicit, shorthand, plain} {
		if r.err != nil {
			t.Fatal(r.err)
		}
	}
	if explicit.stderr != shorthand.stderr {
		t.Fatalf("verbose explicit and shorthand stderr differ:\n%q\n%q", explicit.stderr, shorthand.stderr)
	}
	if explicit.stderr == plain.stderr {
		t.Fatalf("verbose stderr matched plain stderr:\n%s", explicit.stderr)
	}
	if !strings.Contains(explicit.stderr, "--- invocation ---") {
		t.Fatalf("verbose stderr missing dump:\n%s", explicit.stderr)
	}
}

// TestRunCLIVerboseUsesShortAlias confirms -v is accepted for --verbose.
func TestRunCLIVerboseUsesShortAlias(t *testing.T) {
	got := runCLIForTest(t, "-v", "--print-main", "testdata/clean.gorn")
	if got.err != nil {
		t.Fatal(got.err)
	}
	if !strings.Contains(got.stderr, "--- invocation ---") {
		t.Fatalf("-v did not enable verbose dump:\n%s", got.stderr)
	}
}

func TestRunCLINormalRunSurfacesParseError(t *testing.T) {
	got := runCLIForTest(t, "run", "testdata/missing_main.gorn")
	if got.err == nil {
		t.Fatal("error = nil, want parse error")
	}
	if !strings.Contains(got.err.Error(), "missing //gorn:main") {
		t.Fatalf("error = %v, want missing //gorn:main", got.err)
	}
	if strings.Contains(got.stderr, "not implemented") {
		t.Fatalf("reached not-implemented notice despite parse failure:\n%s", got.stderr)
	}
}

// TestRunCLINormalRunSurfacesGenerateError checks that a plain run (no print
// flags) still validates via generation — an invalid script surfaces the
// generation error instead of the not-implemented notice.
func TestRunCLINormalRunSurfacesGenerateError(t *testing.T) {
	got := runCLIForTest(t, "run", "testdata/duplicate_import.gorn")
	if got.err == nil {
		t.Fatal("error = nil, want preamble import conflict")
	}
	if !strings.Contains(got.err.Error(), "conflicts with //gorn:preamble") {
		t.Fatalf("error = %v, want preamble import conflict", got.err)
	}
	if strings.Contains(got.stderr, "not implemented") {
		t.Fatalf("reached not-implemented notice despite invalid script:\n%s", got.stderr)
	}
}

func TestRunCLIPrintModEmitsRawGoMod(t *testing.T) {
	got := runCLIForTest(t, "run", "--print-mod", "testdata/clean.gorn")
	if got.err != nil {
		t.Fatal(got.err)
	}
	if !strings.HasPrefix(got.stdout, "module ") {
		t.Fatalf("--print-mod stdout is not a raw go.mod:\n%s", got.stdout)
	}
	if strings.Contains(got.stdout, "// ---") {
		t.Fatalf("--print-mod (single artifact) should have no header:\n%s", got.stdout)
	}
	if strings.Contains(got.stdout, "package main") {
		t.Fatalf("--print-mod stdout unexpectedly contains main.go:\n%s", got.stdout)
	}
}

func TestRunCLIPrintMainEmitsRawMain(t *testing.T) {
	got := runCLIForTest(t, "run", "--print-main", "testdata/clean.gorn")
	if got.err != nil {
		t.Fatal(got.err)
	}
	if !strings.Contains(got.stdout, "package main") || !strings.Contains(got.stdout, "func main()") {
		t.Fatalf("--print-main stdout is not a main.go:\n%s", got.stdout)
	}
	if strings.Contains(got.stdout, "// ---") {
		t.Fatalf("--print-main (single artifact) should have no header:\n%s", got.stdout)
	}
}

func TestRunCLIPrintModAndMainEmitBothWithHeaders(t *testing.T) {
	got := runCLIForTest(t, "run", "--print-mod", "--print-main", "testdata/clean.gorn")
	if got.err != nil {
		t.Fatal(got.err)
	}
	if !strings.Contains(got.stdout, "// --- go.mod ---") || !strings.Contains(got.stdout, "// --- main.go ---") {
		t.Fatalf("--print-mod --print-main missing headers:\n%s", got.stdout)
	}
	if !strings.Contains(got.stdout, "module ") || !strings.Contains(got.stdout, "package main") {
		t.Fatalf("--print-mod --print-main missing go.mod or main.go content:\n%s", got.stdout)
	}
}

func TestRunCLIErrorsDoNotRun(t *testing.T) {
	tests := [][]string{
		{"rn", "main.go"},
		{"--nope"},
		{"run", "missing.gorn"},
		{"missing.gorn"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			got := runCLIForTest(t, args...)
			if got.err == nil {
				t.Fatal("error = nil")
			}
			// Erroring before the run branch means the notice never prints.
			if strings.Contains(got.stderr, "not implemented") {
				t.Fatalf("reached run branch despite error:\n%s", got.stderr)
			}
		})
	}
}

func TestRunCLIFallbackErrorPrintsUsage(t *testing.T) {
	got := runCLIForTest(t, "blah")
	if got.err == nil {
		t.Fatal("error = nil")
	}
	if !strings.Contains(got.stdout, "Usage:") {
		t.Fatalf("fallback error did not print usage:\n%s", got.stdout)
	}
}

func TestRunCLIDoesNotFallbackForExplicitSubcommandErrors(t *testing.T) {
	got := runCLIForTest(t, "run")
	if got.err == nil {
		t.Fatal("error = nil")
	}
	if strings.Contains(got.err.Error(), "stat run") {
		t.Fatalf("explicit run fell back to script lookup: %v", got.err)
	}
}

func TestRunCLIStdinShorthand(t *testing.T) {
	cleanup := withStdin(t, "//gorn:main\nprintln(\"hi\")\n")
	defer cleanup()

	got := runCLIForTest(t, "-", "--print-main")
	if got.err != nil {
		t.Fatal(got.err)
	}
	if !strings.Contains(got.stdout, `println("hi")`) {
		t.Fatalf("stdin script not reflected in generated main:\n%s", got.stdout)
	}
}

func usageLine(help string) string {
	for line := range strings.SplitSeq(help, "\n") {
		if strings.HasPrefix(line, "Usage: ") {
			return line
		}
	}
	return ""
}
