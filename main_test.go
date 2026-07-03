package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

type runResult struct {
	out  string
	code int
	err  error
}

type exitCode int

func runCLIForTest(t *testing.T, args ...string) (result runResult) {
	t.Helper()

	var cliOut bytes.Buffer
	var stdout bytes.Buffer
	result.code = -1
	cleanup := captureStdout(t, &stdout)
	defer func() {
		cleanup()
		result.out = cliOut.String() + stdout.String()
		if v := recover(); v != nil {
			code, ok := v.(exitCode)
			if !ok {
				panic(v)
			}
			result.code = int(code)
		}
	}()

	result.err = RunCLI(args, &cliOut, func(code int) {
		panic(exitCode(code))
	})
	return result
}

func captureStdout(t *testing.T, out *bytes.Buffer) func() {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(out, r)
		close(done)
	}()

	return func() {
		_ = w.Close()
		os.Stdout = old
		<-done
		_ = r.Close()
	}
}

func TestRunCLIVersion(t *testing.T) {
	got := runCLIForTest(t, "--version")
	if got.code != 0 {
		t.Fatalf("exit code = %d, want 0", got.code)
	}
	if strings.TrimSpace(got.out) != (CLI{}).Version() {
		t.Fatalf("version output = %q", got.out)
	}
}

func TestRunCLIHelp(t *testing.T) {
	top := runCLIForTest(t)
	if top.err != nil {
		t.Fatal(top.err)
	}
	if !strings.Contains(top.out, "Commands:") {
		t.Fatalf("top help missing commands:\n%s", top.out)
	}

	run := runCLIForTest(t, "run", "--help")
	if run.code != 0 {
		t.Fatalf("exit code = %d, want 0", run.code)
	}
	usage := usageLine(run.out)
	for _, token := range []string{"--print-gen", "script", "args"} {
		if !strings.Contains(usage, token) {
			t.Fatalf("usage %q missing %q", usage, token)
		}
	}
	if strings.Index(usage, "--print-gen") > strings.Index(usage, "script") {
		t.Fatalf("flag appears after args in usage: %q", usage)
	}
}

func TestRunCLIExplicitAndShorthandRunMatch(t *testing.T) {
	explicit := runCLIForTest(t, "run", "main.go", "--", "--flag")
	shorthand := runCLIForTest(t, "main.go", "--", "--flag")
	if explicit.err != nil {
		t.Fatal(explicit.err)
	}
	if shorthand.err != nil {
		t.Fatal(shorthand.err)
	}
	if explicit.out != shorthand.out {
		t.Fatalf("explicit and shorthand output differ:\nexplicit:\n%s\nshorthand:\n%s", explicit.out, shorthand.out)
	}
}

func TestRunCLIGlobalVerboseAppliesToRunAndShorthand(t *testing.T) {
	explicit := runCLIForTest(t, "--verbose", "run", "main.go")
	shorthand := runCLIForTest(t, "--verbose", "main.go")
	plain := runCLIForTest(t, "run", "main.go")
	if explicit.err != nil {
		t.Fatal(explicit.err)
	}
	if shorthand.err != nil {
		t.Fatal(shorthand.err)
	}
	if plain.err != nil {
		t.Fatal(plain.err)
	}
	if explicit.out != shorthand.out {
		t.Fatalf("verbose explicit and shorthand output differ:\nexplicit:\n%s\nshorthand:\n%s", explicit.out, shorthand.out)
	}
	if explicit.out == plain.out {
		t.Fatalf("verbose output matched plain output:\n%s", explicit.out)
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
			if strings.Contains(got.out, "Running script:") {
				t.Fatalf("command ran despite error:\n%s", got.out)
			}
		})
	}
}

func TestRunCLIFallbackErrorPrintsUsage(t *testing.T) {
	got := runCLIForTest(t, "blah")
	if got.err == nil {
		t.Fatal("error = nil")
	}
	if strings.Contains(got.out, "Running script:") {
		t.Fatalf("command ran despite error:\n%s", got.out)
	}
	if !strings.Contains(got.out, "Usage:") {
		t.Fatalf("fallback error did not print usage:\n%s", got.out)
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
	if strings.Contains(got.out, "Running script:") {
		t.Fatalf("command ran despite explicit subcommand error:\n%s", got.out)
	}
}

func TestRunCLIStdinShorthand(t *testing.T) {
	got := runCLIForTest(t, "-", "--", "--flag")
	if got.err != nil {
		t.Fatal(got.err)
	}
	if !strings.Contains(got.out, "Running script: -") || !strings.Contains(got.out, "--flag") {
		t.Fatalf("stdin shorthand output missing script or arg:\n%s", got.out)
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
