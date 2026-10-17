package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCLIStandardMainBuildAndRun(t *testing.T) {
	if testing.Short() {
		t.Skip("builds real scripts")
	}
	t.Setenv("GORN_CACHE", t.TempDir())
	path := filepath.Join(t.TempDir(), "io script.gorn")
	source := `package main
import ("fmt"; "io"; "os")
func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil { panic(err) }
	fmt.Printf("args=%q\nstdin=%s\n", os.Args[1:], data)
	fmt.Fprintln(os.Stderr, "script stderr")
}`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"cold", "warm", "bypass"} {
		t.Run(mode, func(t *testing.T) {
			cleanup := withStdin(t, "input")
			defer cleanup()
			args := []string{"run", "--verbose"}
			if mode == "bypass" {
				args = append(args, "--no-cache")
			}
			args = append(args, path, "--", "--flag", "two words", "")
			got := runCLIForTest(t, args...)
			if got.err != nil {
				t.Fatal(got.err)
			}
			if got.stdout != "args=[\"--flag\" \"two words\" \"\"]\nstdin=input\n" || !strings.Contains(got.stderr, "script stderr\n") {
				t.Fatalf("stdout=%q stderr=%q", got.stdout, got.stderr)
			}
			want := map[string]string{"cold": "cache: miss", "warm": "cache: hit", "bypass": "cache: bypass"}[mode]
			if !strings.Contains(got.stderr, want) {
				t.Fatalf("missing %q in %q", want, got.stderr)
			}
		})
	}
	cleanup := withStdin(t, "package main\nimport \"fmt\"\nfunc main(){fmt.Print(\"stdin source\")}")
	defer cleanup()
	got := runCLIForTest(t, "run", "-")
	if got.err != nil || got.stdout != "stdin source" {
		t.Fatalf("stdin source: %v, stdout=%q", got.err, got.stdout)
	}
}

func TestRunCLIColdInspectionPreservesSource(t *testing.T) {
	t.Setenv("GORN_CACHE", t.TempDir())
	dir := t.TempDir()
	path, marker := filepath.Join(dir, "inspect.gorn"), filepath.Join(dir, "executed")
	source := fmt.Sprintf("package main\nimport \"os\"\nfunc main( ){_ = os.WriteFile(%q, []byte(`first\nsecond`), 0600)}", marker)
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, flags := range [][]string{{"--print-main"}, {"--print-mod"}, {"--print-main", "--print-mod"}} {
		args := append([]string{"run"}, flags...)
		got := runCLIForTest(t, append(args, path)...)
		if got.err != nil {
			t.Fatal(got.err)
		}
		if flags[0] == "--print-main" && !strings.Contains(got.stdout, source) {
			t.Fatalf("source was changed:\n%s", got.stdout)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("inspection executed script: %v", err)
		}
	}
}
