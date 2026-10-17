//nolint:gosec // Controlled installed CLI, held runtime, cache files and output contracts.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gornkit/gorn/internal/cache"
)

func TestInstalledCacheReadOnlyCommandsWithoutGo(t *testing.T) {
	bin := builtCLI(t)
	dir := t.TempDir()
	root := filepath.Join(dir, "missing", "cache")
	t.Setenv("GORN_CACHE", root)
	t.Setenv("PATH", t.TempDir())
	for _, args := range [][]string{{"cache", "path"}, {"cache", "path", "--json"}, {"cache", "list", "--json"}, {"cache", "clean", "--all", "--dry-run", "--json"}, {"cache", "prune", "--max-bytes", "0", "--dry-run", "--json"}, {"cache", "prune", "--max-age", "0s", "--dry-run", "--json"}} {
		got := invokeCLI(t, bin, dir, "", args...)
		if got.err != nil || got.stderr != "" {
			t.Fatalf("%v: %+v", args, got)
		}
		if args[len(args)-1] == "--json" {
			var decoded map[string]any
			if err := json.Unmarshal([]byte(got.stdout), &decoded); err != nil || decoded["schema"] != float64(1) {
				t.Fatal(got.stdout, err)
			}
		} else if strings.TrimSpace(got.stdout) != root {
			t.Fatal(got.stdout, root)
		}
	}
	if _, err := os.Stat(filepath.Dir(root)); !os.IsNotExist(err) {
		t.Fatal("read-only cache commands created root:", err)
	}
	for _, args := range [][]string{{"cache", "clean", "--all"}, {"cache", "clean", "--yes"}, {"cache", "clean", "a", "--all", "--yes"}, {"cache", "prune", "--yes"}, {"cache", "prune", "--max-bytes", "-1", "--yes"}, {"cache", "prune", "--max-age", "-1s", "--yes"}} {
		got := invokeCLI(t, bin, dir, "", args...)
		if got.code != 2 || got.stdout != "" {
			t.Fatalf("invalid maintenance request: %v %+v", args, got)
		}
	}
	help := invokeCLI(t, bin, dir, "", "cache")
	if help.err != nil || !strings.Contains(help.stdout, "prune") || strings.Contains(help.stdout, "stub") {
		t.Fatal(help)
	}
}

func TestInstalledCacheSkipsActualRunThenPrunes(t *testing.T) {
	bin := builtCLI(t)
	dir := t.TempDir()
	root := filepath.Join(dir, "cache")
	t.Setenv("GORN_CACHE", root)
	t.Setenv("GOPROXY", "off")
	marker := filepath.Join(dir, "running")
	t.Setenv("CACHE_RUN_MARKER", marker)
	source := writeScript(t, dir, "held.gorn", `package main
import("io";"os")
func main(){if err:=os.WriteFile(os.Getenv("CACHE_RUN_MARKER"),nil,0600);err!=nil{panic(err)};_,_=io.Copy(io.Discard,os.Stdin)}`)
	child := exec.Command(bin, "--quiet", "run", source)
	input, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	child.Stderr = &stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close(); _ = child.Process.Kill(); _ = child.Wait() }()
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("runtime never started:", stderr.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Setenv("PATH", t.TempDir())
	listed := invokeCLI(t, bin, dir, "", "cache", "list", "--json")
	if listed.err != nil {
		t.Fatal(listed)
	}
	var inventory cache.Inventory
	if err := json.Unmarshal([]byte(listed.stdout), &inventory); err != nil {
		t.Fatal(err)
	}
	entries := 0
	for _, item := range inventory.Items {
		if item.Kind == "entry" {
			entries++
			if item.State != "valid" || item.Profile == nil || item.Source != source {
				t.Fatal(item)
			}
		}
	}
	if entries != 1 {
		t.Fatal(inventory)
	}
	cleaned := invokeCLI(t, bin, dir, "", "cache", "clean", "--all", "--yes", "--json")
	if cleaned.err != nil {
		t.Fatal(cleaned)
	}
	var report cache.MaintenanceReport
	if err := json.Unmarshal([]byte(cleaned.stdout), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Actions) != 1 || report.Actions[0].State != "skipped-active" {
		t.Fatal(report)
	}
	unmet := invokeCLI(t, bin, dir, "", "cache", "prune", "--max-bytes", "0", "--yes", "--json")
	if unmet.code != 1 || json.Unmarshal([]byte(unmet.stdout), &report) != nil || !report.UnmetBudget {
		t.Fatal(unmet)
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err != nil {
		t.Fatal("held runtime failed:", err, stderr.String())
	}
	pruned := invokeCLI(t, bin, dir, "", "cache", "prune", "--max-bytes", "0", "--yes", "--json")
	if pruned.err != nil || json.Unmarshal([]byte(pruned.stdout), &report) != nil || report.UnmetBudget || report.After.Managed.Bytes != 0 {
		t.Fatal(pruned)
	}
	if len(report.Actions) != 1 || report.Actions[0].State != "removed" {
		t.Fatal(report)
	}
}
