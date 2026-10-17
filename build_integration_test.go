//nolint:gosec // Controlled installed CLI, generated executables, and local output fixtures.
package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstalledBuildColdWarmBypassAndStandalone(t *testing.T) {
	bin := builtCLI(t)
	dir := t.TempDir()
	cacheRoot := filepath.Join(dir, "cache")
	t.Setenv("GORN_CACHE", cacheRoot)
	t.Setenv("GOPROXY", "off")
	source := writeScript(t, dir, "standalone.gorn", `package main
import("fmt";"os")
func init(){if err:=os.WriteFile(os.Getenv("EXPORT_MARKER"),[]byte("ran"),0600);err!=nil{panic(err)}}
func main(){fmt.Print("standalone:",os.Args[1])}`)
	marker := filepath.Join(dir, "executed")
	t.Setenv("EXPORT_MARKER", marker)
	for _, mode := range []string{"cold", "warm", "bypass"} {
		t.Run(mode, func(t *testing.T) {
			output := filepath.Join(dir, "tool-"+mode)
			args := []string{"-v", "build", source, "-o", output}
			if mode == "bypass" {
				args = append(args, "--no-cache")
			}
			got := invokeCLI(t, bin, dir, "", args...)
			if got.err != nil || got.stdout != "" {
				t.Fatalf("build: %+v", got)
			}
			want := "miss"
			if mode == "warm" {
				want = "hit"
			}
			if mode == "bypass" {
				want = "bypass"
			}
			if !strings.Contains(got.stderr, "cache: "+want) {
				t.Fatalf("expected %s: %s", want, got.stderr)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("build executed script init/main:", err)
			}
			if runtime.GOOS == "windows" {
				output += ".exe"
			}
			other := t.TempDir()
			cmd := exec.Command(output, "literal value")
			cmd.Dir = other
			cmd.Env = append(os.Environ(), "PATH=", "GORN_CACHE="+filepath.Join(other, "unavailable"))
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			text, err := cmd.Output()
			if err != nil || string(text) != "standalone:literal value" {
				t.Fatalf("standalone: %q %v %s", text, err, stderr.String())
			}
			if err := os.Remove(marker); err != nil {
				t.Fatal(err)
			}
		})
	}
	entries, err := os.ReadDir(filepath.Join(cacheRoot, "tmp"))
	if err != nil || len(entries) != 0 {
		t.Fatal("bypass stage leaked:", entries, err)
	}
	// Build source from stdin with the same source collection contract as run.
	output := filepath.Join(dir, "stdin.custom")
	got := invokeCLI(t, bin, dir, "package main\nfunc main(){}", "build", "-", "-o", output)
	if got.err != nil || got.stdout != "" {
		t.Fatal(got)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatal("explicit extension changed:", err)
	}
}

func TestInstalledBuildRefusesOverwritesAndPreservesFailures(t *testing.T) {
	bin := builtCLI(t)
	dir := t.TempDir()
	t.Setenv("GORN_CACHE", filepath.Join(dir, "cache"))
	t.Setenv("GOPROXY", "off")
	source := writeScript(t, dir, "app.gorn", "package main\nfunc main(){}")
	bad := writeScript(t, dir, "bad.gorn", "package main\nfunc main(){missing()}")
	output := filepath.Join(dir, "out.exe")
	if err := os.WriteFile(output, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"build", source, "-o", output}, {"build", bad, "-o", output, "--force"}, {"build", source, "-o", source, "--force"}, {"build", source, "-o", dir, "--force"}} {
		got := invokeCLI(t, bin, dir, "", args...)
		if got.err == nil {
			t.Fatalf("unsafe build succeeded: %v", args)
		}
		text, err := os.ReadFile(output)
		if err != nil || string(text) != "original" {
			t.Fatal("existing output changed:", string(text), err)
		}
	}
	got := invokeCLI(t, bin, dir, "", "build", source, "-o", output, "--force")
	if got.err != nil {
		t.Fatal(got)
	}
	data, err := os.ReadFile(output)
	if err != nil || bytes.Equal(data, []byte("original")) {
		t.Fatal("force failed to replace:", err)
	}
	for _, args := range [][]string{{"build"}, {"build", source}, {"build", source, "-o", output, "extra"}} {
		got := invokeCLI(t, bin, dir, "", args...)
		if got.code != 2 || got.stdout != "" {
			t.Fatalf("usage contract: %v %+v", args, got)
		}
	}
	help := invokeCLI(t, bin, dir, "", "build", "--help")
	if help.err != nil || !strings.Contains(help.stdout, "--output") || strings.Contains(help.stdout, "stub") {
		t.Fatal(help)
	}
}

func TestInstalledBuildBundledHelperWithoutGoAtRuntime(t *testing.T) {
	bin := builtCLI(t)
	dir := t.TempDir()
	t.Setenv("GORN_CACHE", filepath.Join(dir, "cache"))
	t.Setenv("GOPROXY", "off")
	source := writeScript(t, dir, "helper.gorn", `package main
import("fmt";"github.com/gornkit/gorn/sh")
func main(){result:=sh.Cmd("gorn-export-deliberately-missing").Exec();if !result.OK(){fmt.Print("bundled")}}`)
	output := filepath.Join(dir, "helper.exe")
	result := invokeCLI(t, bin, dir, "", "build", source, "-o", output)
	if result.err != nil {
		t.Fatal(result)
	}
	cmd := exec.Command(output)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "PATH=")
	data, err := cmd.CombinedOutput()
	if err != nil || string(data) != "bundled" {
		t.Fatalf("standalone helper: %q %v", data, err)
	}
}
