//nolint:gosec // Controlled loopback module proxy, generated scripts and private cache corruption.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gornkit/gorn/internal/cache"
)

func installedModuleProxy(t *testing.T) (*atomic.Int64, *atomic.Bool) {
	t.Helper()
	contents := map[string][]byte{}
	add := func(path, version, mod string, files map[string]string) {
		t.Helper()
		prefix := "/" + path + "/@v/" + version
		contents[prefix+".mod"] = []byte(mod)
		contents[prefix+".info"] = fmt.Appendf(nil, `{"Version":%q,"Time":"2026-01-01T00:00:00Z"}`, version)
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		files["go.mod"] = mod
		for name, data := range files {
			entry, err := writer.Create(path + "@" + version + "/" + name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := entry.Write([]byte(data)); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		contents[prefix+".zip"] = buffer.Bytes()
	}
	add("fixture.invalid/message", "v1.0.0", "module fixture.invalid/message\ngo 1.26\n", map[string]string{"message.go": `package message;const Prefix="hello, "`})
	add("fixture.invalid/greeting", "v1.2.3", "module fixture.invalid/greeting\ngo 1.26\nrequire fixture.invalid/message v1.0.0\n", map[string]string{"greeting.go": `package greeting;import "fixture.invalid/message";func Hello(name string)string{return message.Prefix+name}`})
	count, unavailable := &atomic.Int64{}, &atomic.Bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		if unavailable.Load() {
			http.Error(w, "proxy deliberately unavailable", http.StatusServiceUnavailable)
			return
		}
		data, ok := contents[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)
	t.Setenv("GOPROXY", server.URL)
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOPRIVATE", "")
	t.Setenv("GONOPROXY", "")
	t.Setenv("GONOSUMDB", "")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOTOOLCHAIN", "local")
	modcache := t.TempDir()
	t.Setenv("GOMODCACHE", modcache)
	t.Cleanup(func() {
		_ = filepath.WalkDir(modcache, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			mode := fs.FileMode(0o600)
			if entry.IsDir() {
				mode = 0o700
			}
			return os.Chmod(path, mode)
		})
	})
	return count, unavailable
}

const installedDependencySource = `//gorn:require fixture.invalid/greeting v1.2.3
package main
import("fmt";"os";"fixture.invalid/greeting")
func main(){fmt.Print(greeting.Hello(os.Args[1]))}`

func TestInstalledDependencyColdWarmBypassRepairAndExport(t *testing.T) {
	bin := builtCLI(t)
	requests, unavailable := installedModuleProxy(t)
	dir := t.TempDir()
	root := filepath.Join(dir, "cache")
	t.Setenv("GORN_CACHE", root)
	source := writeScript(t, dir, "dependency.gorn", installedDependencySource)
	cold := invokeCLI(t, bin, dir, "", "run", "-v", source, "--", "world")
	if cold.err != nil || cold.stdout != "hello, world" || !strings.Contains(cold.stderr, "cache: miss") || requests.Load() == 0 {
		t.Fatal(cold, requests.Load())
	}
	requested := requests.Load()
	unavailable.Store(true)
	for _, mode := range []string{"warm", "bypass", "manifest repair", "binary repair"} {
		args := []string{"run", "-v"}
		if mode == "bypass" {
			args = append(args, "--no-cache")
		}
		if strings.HasSuffix(mode, "repair") {
			manifests, err := filepath.Glob(filepath.Join(root, "apps", "*", "*", "manifest.json"))
			if err != nil || len(manifests) != 1 {
				t.Fatal(manifests, err)
			}
			target := manifests[0]
			if mode == "binary repair" {
				name := "app"
				if runtime.GOOS == "windows" {
					name += ".exe"
				}
				target = filepath.Join(filepath.Dir(target), "bin", name)
			}
			if err := os.WriteFile(target, []byte("corrupt"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		got := invokeCLI(t, bin, dir, "", append(args, source, "--", mode)...)
		want := "cache: miss"
		if mode == "warm" {
			want = "cache: hit"
		}
		if mode == "bypass" {
			want = "cache: bypass"
		}
		if got.err != nil || got.stdout != "hello, "+mode || !strings.Contains(got.stderr, want) {
			t.Fatal(mode, got)
		}
		if requests.Load() != requested {
			t.Fatal("populated/warm workflow contacted unavailable proxy", mode, requests.Load(), requested)
		}
	}
	listed := invokeCLI(t, bin, dir, "", "cache", "list", "--json")
	var inventory cache.Inventory
	if listed.err != nil || json.Unmarshal([]byte(listed.stdout), &inventory) != nil {
		t.Fatal(listed)
	}
	owned, quarantine := 0, 0
	for _, item := range inventory.Items {
		if item.Kind == "entry" {
			owned++
			if item.State != "valid" {
				t.Fatal(item)
			}
		}
		if item.Kind == "quarantine" {
			quarantine++
		}
	}
	if owned != 1 || quarantine != 2 {
		t.Fatal("repair inventory", inventory)
	}
	manifests, err := filepath.Glob(filepath.Join(root, "apps", "*", "*", "dependencies.json"))
	if err != nil || len(manifests) != 1 {
		t.Fatal(manifests, err)
	}
	graph, err := os.ReadFile(manifests[0])
	if err != nil || !bytes.Contains(graph, []byte("fixture.invalid/message")) || !bytes.Contains(graph, []byte("v1.2.3")) {
		t.Fatal("missing graph evidence", string(graph), err)
	}
	output := filepath.Join(dir, "exported")
	if runtime.GOOS == "windows" {
		output += ".exe"
	}
	built := invokeCLI(t, bin, dir, "", "build", source, "-o", output)
	if built.err != nil {
		t.Fatal(built)
	}
	if requests.Load() != requested {
		t.Fatal("warm export contacted unavailable proxy")
	}
	t.Setenv("PATH", t.TempDir())
	out, err := exec.Command(output, "standalone").CombinedOutput()
	if err != nil || string(out) != "hello, standalone" {
		t.Fatal(string(out), err)
	}
}

func TestInstalledConcurrentDependencyBuildsSharePublication(t *testing.T) {
	bin := builtCLI(t)
	installedModuleProxy(t)
	dir := t.TempDir()
	t.Setenv("GORN_CACHE", filepath.Join(dir, "cache"))
	source := writeScript(t, dir, "concurrent.gorn", installedDependencySource)
	var children []*exec.Cmd
	var stdout, stderr [2]bytes.Buffer
	for i := range 2 {
		child := exec.Command(bin, "--quiet", "run", "-v", source, "--", fmt.Sprint(i))
		child.Dir = dir
		child.Stdout = &stdout[i]
		child.Stderr = &stderr[i]
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		children = append(children, child)
		t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	}
	hits, misses := 0, 0
	for i, child := range children {
		if err := child.Wait(); err != nil || stdout[i].String() != "hello, "+fmt.Sprint(i) {
			t.Fatal(i, stdout[i].String(), stderr[i].String(), err)
		}
		if strings.Contains(stderr[i].String(), "cache: hit") {
			hits++
		}
		if strings.Contains(stderr[i].String(), "cache: miss") {
			misses++
		}
	}
	if hits != 1 || misses != 1 {
		t.Fatal("concurrent builds did not share one publication", stderr[0].String(), stderr[1].String())
	}
}
