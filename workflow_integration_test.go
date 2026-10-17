//nolint:gosec // Maintained examples and a built CLI executed in private directories.
package main

import (
	"bytes"
	"encoding/json"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestInstalledOnboardingMaintainedExamples(t *testing.T) {
	bin := builtCLI(t)
	dir := t.TempDir()
	root := filepath.Join(dir, "cache")
	t.Setenv("GORN_CACHE", root)
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOFLAGS", "")
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```go\\n(.*?)\\n```").FindAllSubmatch(readme, -1)
	if len(blocks) < 2 {
		t.Fatal("README must contain maintained hello/helper source")
	}
	examples := []string{"hello.gorn", "helper.gorn"}
	var hello string
	for i, name := range examples {
		path, err := filepath.Abs(filepath.Join("examples", name))
		if err != nil {
			t.Fatal(err)
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		maintained, err := format.Source(source)
		if err != nil {
			t.Fatal(err)
		}
		documented, err := format.Source(blocks[i][1])
		if err != nil || !bytes.Equal(maintained, documented) {
			t.Fatal("README example drift", name, err)
		}
		args := []string{"run", path}
		if i == 0 {
			hello = path
			args = append(args, "--", "Ada")
		}
		got := invokeCLI(t, bin, dir, "", args...)
		if got.err != nil || got.stderr != "" {
			t.Fatal(name, got)
		}
		if i == 0 && got.stdout != "hello, Ada\n" {
			t.Fatal(got)
		}
		if i == 1 && !strings.HasPrefix(got.stdout, "go version go") {
			t.Fatal("helper exact command", got)
		}
	}
	for _, args := range [][]string{{hello, "--", "Ada"}, {"run", "--no-cache", hello, "--", "Ada"}} {
		got := invokeCLI(t, bin, dir, "", args...)
		if got.err != nil || got.stdout != "hello, Ada\n" || got.stderr != "" {
			t.Fatal(got)
		}
	}
	output := filepath.Join(dir, "hello")
	if runtime.GOOS == "windows" {
		output += ".exe"
	}
	got := invokeCLI(t, bin, dir, "", "build", hello, "-o", output)
	if got.err != nil {
		t.Fatal(got)
	}
	for _, flag := range []string{"--print-main", "--print-mod"} {
		got := invokeCLI(t, bin, dir, "", "run", flag, hello)
		if got.err != nil || got.stdout == "" {
			t.Fatal(flag, got)
		}
	}
	before := cacheSnapshot(t, root)
	for _, args := range [][]string{{"cache", "path"}, {"cache", "list", "--json"}, {"cache", "clean", "--all", "--dry-run", "--json"}, {"--help"}, {"cache", "--help"}} {
		got := invokeCLI(t, bin, dir, "", args...)
		if got.err != nil || got.stdout == "" {
			t.Fatal(args, got)
		}
		if args[len(args)-1] == "--json" {
			var envelope map[string]any
			if json.Unmarshal([]byte(got.stdout), &envelope) != nil || envelope["schema"] != float64(1) {
				t.Fatal(got)
			}
		}
	}
	if after := cacheSnapshot(t, root); after != before {
		t.Fatal("onboarding inspection mutated cache")
	}
	if runtime.GOOS != "windows" {
		source, err := os.ReadFile(hello)
		if err != nil {
			t.Fatal(err)
		}
		executable := filepath.Join(dir, "executable.gorn")
		if err := os.WriteFile(executable, append([]byte("#!/usr/bin/env gorn\n"), source...), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
		child := exec.Command(executable, "--quiet", "--", "Ada")
		child.Dir = dir
		var out, stderr bytes.Buffer
		child.Stdout = &out
		child.Stderr = &stderr
		// Quiet avoids routine preparation output in the shebang process.
		// Runner flags precede the separator and therefore use normal shorthand rules.
		if err := child.Run(); err != nil || out.String() != "hello, Ada\n" || stderr.Len() != 0 {
			t.Fatal("Unix shebang", out.String(), stderr.String(), err)
		}
	}
	t.Setenv("PATH", t.TempDir())
	out, err := exec.Command(output, "Ada").CombinedOutput()
	if err != nil || string(out) != "hello, Ada\n" {
		t.Fatal("standalone onboarding output", string(out), err)
	}
}
