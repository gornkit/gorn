//nolint:gosec // Controlled test executables, release markers, and private fixture paths.
package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type activityBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *activityBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}
func (b *activityBuffer) text() string { b.mu.Lock(); defer b.mu.Unlock(); return b.String() }

func waitActivity(t *testing.T, out *activityBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for !strings.Contains(out.text(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("missing %q in activity:\n%s", want, out.text())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestInstalledCLIProgressAndQuiet(t *testing.T) {
	bin := builtCLI(t)
	dir := t.TempDir()
	t.Setenv("GORN_CACHE", filepath.Join(dir, "cache"))
	path := writeScript(t, dir, "progress.gorn", `package main
import("fmt";"os")
func main(){fmt.Print("out");fmt.Fprintln(os.Stderr,"child")}`)
	for _, mode := range []string{"cold", "warm", "bypass", "-q", "--quiet", "--quiet=true"} {
		args := []string{"run"}
		quiet := strings.HasPrefix(mode, "-")
		if quiet {
			args = append([]string{mode}, args...)
		}
		if mode == "bypass" {
			args = append(args, "--no-cache")
		}
		cmd := exec.Command(bin, append(args, path)...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\n%s", mode, err, stderr.String())
		}
		if stdout.String() != "out" {
			t.Fatalf("stdout=%q", stdout.String())
		}
		if quiet {
			if stderr.String() != "child\n" {
				t.Fatalf("quiet stderr=%q", stderr.String())
			}
		} else {
			for _, phase := range []string{"reading source", "discovering installed Go", "checking build cache", "running script"} {
				if !strings.Contains(stderr.String(), "gorn: "+phase+"...\n") {
					t.Fatalf("%s missing %q:\n%s", mode, phase, stderr.String())
				}
			}
			compiled := strings.Contains(stderr.String(), "gorn: compiling script...")
			if compiled != (mode != "warm") {
				t.Fatalf("%s compilation activity:\n%s", mode, stderr.String())
			}
		}
	}
	broken := writeScript(t, dir, "broken.gorn", "package main\nfunc main(){undefined()}\n")
	cmd := exec.Command(bin, "run", "--quiet", broken)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err == nil {
		t.Fatal("invalid program succeeded")
	}
	if stdout.Len() != 0 || strings.Contains(stderr.String(), "reading source") || strings.Count(stderr.String(), "undefined: undefined") != 1 || strings.Contains(stderr.String(), "invalid generated main.go") {
		t.Fatalf("quiet compiler error:\n%s", stderr.String())
	}
}

func TestInstalledCLIImmediateSourceHeartbeatAndRuntimeHandoff(t *testing.T) {
	bin := builtCLI(t)
	t.Setenv("GORN_CACHE", t.TempDir())
	cmd := exec.Command(bin, "run", "-")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close() }()
	var stdout, stderr activityBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	waitActivity(t, &stderr, "gorn: reading source...\n")
	if stdout.text() != "" {
		t.Fatal("progress polluted stdout")
	}
	waitActivity(t, &stderr, "gorn: still reading source...\n")
	if _, err := input.Write([]byte(`package main
import("fmt";"os";"time")
func main(){fmt.Fprintln(os.Stderr,"runtime begins");time.Sleep(2100*time.Millisecond);fmt.Print("done")}`)); err != nil {
		t.Fatal(err)
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("run: %v\n%s", err, stderr.text())
	}
	if stdout.text() != "done" || strings.Contains(stderr.text(), "still running script") || !strings.HasSuffix(stderr.text(), "runtime begins\n") {
		t.Fatalf("runtime handoff: stdout=%q\n%s", stdout.text(), stderr.text())
	}
}

func TestInstalledCLIStreamsSanitizedDependencyFailure(t *testing.T) {
	bin := builtCLI(t)
	release := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(release) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		http.Error(w, "fixture failure https://user:private-token@example.test/file?query-token=hidden", http.StatusInternalServerError)
	}))
	defer func() { finish(); server.Close() }()
	dir := t.TempDir()
	t.Setenv("GORN_CACHE", filepath.Join(dir, "cache"))
	t.Setenv("GOMODCACHE", t.TempDir())
	t.Setenv("GOPROXY", server.URL)
	t.Setenv("GOSUMDB", "off")
	path := writeScript(t, dir, "dependency.gorn", "//gorn:require example.test/failure v1.0.0\npackage main\nimport _ \"example.test/failure\"\nfunc main(){}")
	cmd := exec.Command(bin, "run", path)
	var stdout, stderr activityBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	waitActivity(t, &stderr, "gorn: downloading declared dependencies...")
	waitActivity(t, &stderr, "gorn: still downloading declared dependencies...")
	finish()
	if err := cmd.Wait(); err == nil {
		t.Fatal("proxy failure succeeded")
	}
	text := stderr.text()
	if stdout.text() != "" || !strings.Contains(text, "[redacted]@example.test/file?[redacted]") || strings.Contains(text, "private-token") || strings.Contains(text, "query-token") || strings.Contains(text, "hidden") {
		t.Fatalf("streamed error:\n%s", text)
	}
	// The final phase summary carries only the cause; diagnostics are not replayed.
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "gorn: error:") && strings.Contains(line, "fixture failure") {
			t.Fatalf("diagnostic replayed: %s", line)
		}
	}
}
