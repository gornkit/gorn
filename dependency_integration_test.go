package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstalledCLIBundledHelperOffline(t *testing.T) {
	bin := builtCLI(t)
	t.Setenv("GORN_CACHE", t.TempDir())
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOMODCACHE", t.TempDir())
	t.Setenv("GOFLAGS", "")
	dir := t.TempDir()
	path := writeScript(t, dir, "helper.gorn", "//gorn:preamble\npackage main\nfunc main(){fmt.Print(sh.PreambleSentinel)}")
	for _, mode := range []string{"cold", "warm", "bypass"} {
		args := []string{"run", "-v"}
		if mode == "bypass" {
			args = append(args, "--no-cache")
		}
		got := invokeCLI(t, bin, dir, "", append(args, path)...)
		want := map[string]string{"cold": "cache: miss", "warm": "cache: hit", "bypass": "cache: bypass"}[mode]
		if got.code != 0 || got.stdout != "true" || !strings.Contains(got.stderr, want) {
			t.Fatalf("%s: %+v", mode, got)
		}
	}
	// Inspection exposes the module plan and needs neither compiler nor cache.
	t.Setenv("PATH", "")
	got := invokeCLI(t, bin, dir, "", "run", "--print-mod", path)
	if got.code != 0 || !strings.Contains(got.stdout, "github.com/gornkit/gorn/sh v0.0.0") || !strings.Contains(got.stdout, "replace github.com/gornkit/gorn/sh => ./gorn-runtime") {
		t.Fatalf("inspection: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "gorn-runtime")); !os.IsNotExist(err) {
		t.Fatalf("inspection materialized helper beside source: %v", err)
	}
}
