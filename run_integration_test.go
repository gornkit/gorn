package main

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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

func TestRunCLIInspectionNeverExecutesOrChangesCache(t *testing.T) {
	if testing.Short() {
		t.Skip("builds real cached scripts")
	}
	for _, state := range []string{"cold", "warm", "bypass", "corrupt manifest", "corrupt binary"} {
		t.Run(state, func(t *testing.T) {
			cachePath := filepath.Join(t.TempDir(), "cache")
			t.Setenv("GORN_CACHE", cachePath)
			dir := t.TempDir()
			path, marker := filepath.Join(dir, "inspect.gorn"), filepath.Join(dir, "executed")
			source := fmt.Sprintf("package main\nimport \"os\"\nfunc main(){if err := os.WriteFile(%q, []byte(\"ran\"), 0600); err != nil {panic(err)}}", marker)
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			if state != "cold" {
				got := runCLIForTest(t, "run", path)
				if got.err != nil {
					t.Fatal(got.err)
				}
				if err := os.Remove(marker); err != nil {
					t.Fatal(err)
				}
				manifests, err := filepath.Glob(filepath.Join(cachePath, "apps", "*", "*", "manifest.json"))
				if err != nil || len(manifests) != 1 {
					t.Fatalf("cached manifests: %v %v", manifests, err)
				}
				var corrupt string
				switch state {
				case "corrupt manifest":
					corrupt = manifests[0]
				case "corrupt binary":
					name := "app"
					if runtime.GOOS == "windows" {
						name += ".exe"
					}
					corrupt = filepath.Join(filepath.Dir(manifests[0]), "bin", name)
					// Replace the completed executable instead of truncating its
					// inode: Linux can briefly retain ETXTBSY after process exit.
					if err := os.Remove(corrupt); err != nil {
						t.Fatal(err)
					}
				}
				if corrupt != "" {
					if err := os.WriteFile(corrupt, []byte("corrupt"), 0o600); err != nil { //nolint:gosec // G703: controlled fixture cache path.
						t.Fatal(err)
					}
				}
			}
			before := cacheSnapshot(t, cachePath)
			// Inspect must work with no compiler available, even on a cache miss.
			t.Setenv("PATH", t.TempDir())
			for _, flags := range [][]string{{"--print-mod"}, {"--print-main"}, {"--print-mod", "--print-main"}} {
				for _, command := range [][]string{{"run"}, {}} {
					args := append([]string(nil), command...)
					args = append(args, flags...)
					if state == "bypass" {
						args = append(args, "--no-cache")
					}
					got := runCLIForTest(t, append(args, path)...)
					if got.err != nil {
						t.Fatal(got.err)
					}
					if _, err := os.Stat(marker); !os.IsNotExist(err) {
						t.Fatalf("inspection executed script: %v", err)
					}
					if len(flags) == 1 && flags[0] == "--print-mod" && got.stdout != "module gorn.local/app\n\ngo 1.26\n" {
						t.Fatalf("unexpected module output: %q", got.stdout)
					}
					if len(flags) == 1 && flags[0] == "--print-main" && strings.Contains(got.stdout, "// ---") {
						t.Fatalf("unexpected artifact header: %q", got.stdout)
					}
					if len(flags) == 2 && (!strings.Contains(got.stdout, "// --- go.mod ---\n") || !strings.Contains(got.stdout, "// --- main.go ---\n")) {
						t.Fatalf("missing artifact headers: %q", got.stdout)
					}
					if strings.Contains(strings.Join(flags, " "), "--print-main") && !strings.Contains(got.stdout, source) {
						t.Fatalf("missing source: %q", got.stdout)
					}
					if strings.Contains(strings.Join(flags, " "), "--print-mod") && !strings.Contains(got.stdout, "module gorn.local/app") {
						t.Fatalf("missing module: %q", got.stdout)
					}
					if after := cacheSnapshot(t, cachePath); before != after {
						t.Fatalf("inspection changed cache: before=%q after=%q", before, after)
					}
				}
			}
		})
	}
}

func cacheSnapshot(t *testing.T, root string) string {
	t.Helper()
	var snapshot strings.Builder
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) && path == root {
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(&snapshot, path)
		if !entry.IsDir() {
			data, err := os.ReadFile(path) //nolint:gosec // G304: controlled fixture cache paths.
			if err != nil {
				return err
			}
			fmt.Fprintf(&snapshot, "%x\n", sha256.Sum256(data))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.String()
}

func TestRunCLIInspectionNeedsNoCacheAccess(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "regular-file")
	if err := os.WriteFile(blocked, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GORN_CACHE", filepath.Join(blocked, "cache"))
	t.Setenv("HOME", blocked)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(blocked, "user-cache"))
	t.Setenv("LOCALAPPDATA", filepath.Join(blocked, "user-cache"))
	t.Setenv("TMPDIR", blocked)
	t.Setenv("TMP", blocked)
	t.Setenv("TEMP", blocked)
	t.Setenv("PATH", "")
	for _, flag := range []string{"--print-main", "--print-mod"} {
		got := runCLIForTest(t, "run", flag, "testdata/run_hello.gorn")
		if got.err != nil || got.stdout == "" {
			t.Fatalf("%s with inaccessible caches: err=%v stdout=%q", flag, got.err, got.stdout)
		}
	}
}
