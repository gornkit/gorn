// Command startupbench measures fixed Gorn workflows with private caches.
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gornkit/gorn/internal/cache"
)

type distribution struct {
	SamplesMS []float64 `json:"samples_ms"`
	MinMS     float64   `json:"min_ms"`
	MedianMS  float64   `json:"median_ms"`
	P95MS     float64   `json:"p95_ms"`
	MeanMS    float64   `json:"mean_ms"`
	MaxMS     float64   `json:"max_ms"`
}
type measurement struct {
	Mode string `json:"mode"`
	distribution
	ProxyRequests int64 `json:"proxy_requests"`
}
type fixtureResult struct {
	Fixture      string        `json:"fixture"`
	SourceSHA256 string        `json:"source_sha256"`
	Measurements []measurement `json:"measurements"`
}
type metadata struct {
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	OSVersion      string `json:"os_version"`
	CPUQuota       string `json:"cpu_quota,omitempty"`
	MemoryLimit    string `json:"memory_limit,omitempty"`
	CPU            string `json:"cpu"`
	LogicalCPUs    int    `json:"logical_cpus"`
	GoMaxProcs     int    `json:"gomaxprocs"`
	Memory         string `json:"memory"`
	GoVersion      string `json:"go_version"`
	CLIVersion     string `json:"cli_version"`
	CLISHA256      string `json:"cli_sha256"`
	CLIRevision    string `json:"cli_revision"`
	SourceRevision string `json:"source_revision"`
	SourceModified string `json:"source_modified"`
	CLIModified    string `json:"cli_modified"`
}
type report struct {
	Schema     int             `json:"schema"`
	RecordedAt string          `json:"recorded_at"`
	Metadata   metadata        `json:"metadata"`
	Fixtures   []fixtureResult `json:"fixtures"`
}
type options struct {
	CLI    string
	Output string
	Cold   int
	Reuse  int
	Warm   int
	Bypass int
}

func main() {
	var o options
	flag.StringVar(&o.CLI, "gorn", "./gorn", "built CLI to measure")
	flag.StringVar(&o.Output, "output", "", "JSON output path (stdout when empty)")
	flag.IntVar(&o.Cold, "cold", 3, "empty-Go-cache samples per fixture (zero skips)")
	flag.IntVar(&o.Reuse, "reuse", 10, "populated-Go-cache cold samples per fixture")
	flag.IntVar(&o.Warm, "warm", 50, "warm and direct-binary samples per fixture")
	flag.IntVar(&o.Bypass, "bypass", 10, "bypass samples per fixture (zero skips)")
	flag.Parse()
	if o.Cold < 0 || o.Reuse < 1 || o.Warm < 1 || o.Bypass < 0 || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "invalid sample counts or positional arguments")
		os.Exit(2)
	}
	result, err := benchmark(o)
	if err == nil {
		var data []byte
		data, err = json.MarshalIndent(result, "", "  ")
		if err == nil {
			data = append(data, '\n')
			if o.Output == "" {
				_, err = os.Stdout.Write(data)
			} else {
				err = os.WriteFile(o.Output, data, 0o600)
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "startupbench:", err)
		os.Exit(1)
	}
}

func benchmark(o options) (result report, resultErr error) {
	cli, err := filepath.Abs(o.CLI)
	if err != nil {
		return result, err
	}
	root, err := os.MkdirTemp("", "gorn-startup-")
	if err != nil {
		return result, err
	}
	defer func() { resultErr = errors.Join(resultErr, removeWork(root)) }()
	if runtime.GOOS == "windows" && filepath.Ext(cli) == "" {
		if _, err := os.Stat(cli + ".exe"); err == nil {
			cli += ".exe"
		} else {
			data, err := os.ReadFile(cli) //nolint:gosec // G304: explicit CLI input; copy only into the private benchmark workspace.
			if err != nil {
				return result, err
			}
			cli = filepath.Join(root, "gorn.exe")
			if err := os.WriteFile(cli, data, 0o600); err != nil { //nolint:gosec // G703: destination reset to a fixed filename inside private MkdirTemp, not the input CLI path.
				return result, err
			}
		}
	}

	result = report{Schema: 1, RecordedAt: time.Now().UTC().Format(time.RFC3339), Fixtures: []fixtureResult{}}
	result.Metadata, err = describe(cli)
	if err != nil {
		return result, err
	}
	proxy, requests, err := dependencyProxy()
	if err != nil {
		return result, err
	}
	defer proxy.Close()
	fixtures := []struct {
		name, source string
		dependency   bool
	}{
		{"tiny", "package main\nfunc main(){}\n", false},
		{"preamble", "//gorn:preamble\npackage main\nfunc main(){if !sh.PreambleSentinel{panic(\"helper\")};fmt.Print(\"\")}\n", false},
		{"dependency", "//gorn:require fixture.invalid/startup v1.0.0\npackage main\nimport \"fixture.invalid/startup\"\nfunc main(){if startup.Value()!=\"ok\"{panic(\"dependency\")}}\n", true},
	}
	for _, fixture := range fixtures {
		dir := filepath.Join(root, fixture.name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return result, err
		}
		source := filepath.Join(dir, "source.gorn")
		if err := os.WriteFile(source, []byte(fixture.source), 0o600); err != nil {
			return result, err
		}
		sum := sha256.Sum256([]byte(fixture.source))
		item := fixtureResult{Fixture: fixture.name, SourceSHA256: hex.EncodeToString(sum[:]), Measurements: []measurement{}}
		proxyURL := "off"
		if fixture.dependency {
			proxyURL = proxy.URL
		}
		if o.Cold > 0 {
			startRequests := requests.Load()
			var samples []float64
			for i := range o.Cold {
				work := filepath.Join(dir, fmt.Sprintf("empty-%d", i))
				env := benchEnvironment(work, proxyURL)
				env = withEnv(env, "GORN_CACHE", filepath.Join(work, "gorn"))
				duration, err := timed(cli, env, "--quiet", "run", source)
				if err != nil {
					return result, err
				}
				samples = append(samples, duration)
				activity(fixture.name, "cold_empty_go", i+1, o.Cold, duration)
				if err := removeWork(work); err != nil {
					return result, err
				}
			}
			item.Measurements = append(item.Measurements, measurement{Mode: "cold_empty_go", distribution: summarize(samples), ProxyRequests: requests.Load() - startRequests})
		}
		work := filepath.Join(dir, "populated")
		env := benchEnvironment(work, proxyURL)
		cachePath := filepath.Join(work, "gorn")
		env = withEnv(env, "GORN_CACHE", cachePath)
		fmt.Fprintf(os.Stderr, "%s: priming private Go/Gorn caches\n", fixture.name)
		if _, err := timed(cli, env, "--quiet", "run", source); err != nil {
			return result, err
		}
		inventory, err := cache.Root(cachePath).Inspect(context.Background())
		if err != nil {
			return result, err
		}
		var executable, builtAt string
		for _, entry := range inventory.Items {
			if entry.Kind == "entry" && entry.State == "valid" {
				executable = filepath.Join(entry.Path, "bin", "app"+exeSuffix())
				builtAt = entry.BuiltAt
			}
		}
		if executable == "" {
			return result, fmt.Errorf("%s: primer produced no valid cached program", fixture.name)
		}
		for _, mode := range []struct {
			name  string
			count int
		}{{"cold_populated_go", o.Reuse}, {"warm", o.Warm}, {"bypass", o.Bypass}, {"direct_binary", o.Warm}} {
			if mode.count == 0 {
				continue
			}
			startRequests := requests.Load()
			var samples []float64
			for i := range mode.count {
				runEnv := env
				command := cli
				args := []string{"--quiet", "run", source}
				if mode.name == "cold_populated_go" {
					runEnv = withEnv(env, "GORN_CACHE", filepath.Join(work, fmt.Sprintf("fresh-%d", i)))
				}
				if mode.name == "bypass" {
					args = []string{"--quiet", "run", "--no-cache", source}
				}
				if mode.name == "direct_binary" {
					command = executable
					args = nil
				}
				duration, err := timed(command, runEnv, args...)
				if err != nil {
					return result, err
				}
				samples = append(samples, duration)
				activity(fixture.name, mode.name, i+1, mode.count, duration)
			}
			count := requests.Load() - startRequests
			if mode.name == "warm" {
				if count != 0 {
					return result, fmt.Errorf("warm %s issued %d proxy requests", fixture.name, count)
				}
				after, err := cache.Root(cachePath).Inspect(context.Background())
				if err != nil {
					return result, err
				}
				found := false
				for _, entry := range after.Items {
					if entry.Kind == "entry" && entry.State == "valid" && entry.BuiltAt == builtAt {
						found = true
					}
				}
				if !found {
					return result, fmt.Errorf("warm %s replaced its cached publication", fixture.name)
				}
			}
			item.Measurements = append(item.Measurements, measurement{Mode: mode.name, distribution: summarize(samples), ProxyRequests: count})
		}
		result.Fixtures = append(result.Fixtures, item)
		if err := removeWork(dir); err != nil {
			return result, err
		}
	}
	return result, nil
}

func summarize(samples []float64) distribution {
	sorted := append([]float64(nil), samples...)
	slices.Sort(sorted)
	var sum float64
	for _, sample := range samples {
		sum += sample
	}
	median := sorted[len(sorted)/2]
	if len(sorted)%2 == 0 {
		median = (sorted[len(sorted)/2-1] + median) / 2
	}
	return distribution{SamplesMS: samples, MinMS: sorted[0], MedianMS: median, P95MS: sorted[int(math.Ceil(float64(len(sorted))*0.95))-1], MeanMS: sum / float64(len(samples)), MaxMS: sorted[len(sorted)-1]}
}

func activity(fixture, mode string, sample, total int, ms float64) {
	fmt.Fprintf(os.Stderr, "%s %s %d/%d: %.2fms\n", fixture, mode, sample, total, ms)
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func timed(command string, env []string, args ...string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...) //nolint:gosec // Contributor-selected CLI or a program built from this tool's fixed fixture source.
	cmd.Env = env
	cmd.Stdin = strings.NewReader("")
	cmd.Stdout = io.Discard
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.WaitDelay = 5 * time.Second
	started := time.Now()
	err := cmd.Run()
	elapsed := float64(time.Since(started)) / float64(time.Millisecond)
	if err != nil {
		return elapsed, fmt.Errorf("%s: %w: %s", filepath.Base(command), err, stderr.String())
	}
	return elapsed, nil
}

func withEnv(env []string, name, value string) []string {
	result := make([]string, 0, len(env)+1)
	for _, pair := range env {
		key, _, _ := strings.Cut(pair, "=")
		equal := key == name
		if runtime.GOOS == "windows" {
			equal = strings.EqualFold(key, name)
		}
		if !equal {
			result = append(result, pair)
		}
	}
	return append(result, name+"="+value)
}

func benchEnvironment(root, proxy string) []string {
	env := os.Environ()
	for name, value := range map[string]string{
		"GOCACHE": filepath.Join(root, "build"), "GOMODCACHE": filepath.Join(root, "modules"), "GOPATH": filepath.Join(root, "path"), "GOPROXY": proxy, "GOSUMDB": "off",
		"GOENV": "off", "GOWORK": "off", "GOTOOLCHAIN": "local", "GOFLAGS": " ", "CGO_ENABLED": "0", "GOOS": runtime.GOOS, "GOARCH": runtime.GOARCH,
		"GOPRIVATE": "", "GONOPROXY": "", "GONOSUMDB": "", "CC": "", "CXX": "", "GOEXPERIMENT": "", "GOFIPS140": "off", "GOCACHEPROG": "", "GODEBUG": "", "GO_EXTLINK_ENABLED": "", "GOCOMPILEDEBUG": "", "GOCLOBBERDEADHASH": "", "GOSSAFUNC": "", "GOSSADIR": "",
	} {
		env = withEnv(env, name, value)
	}
	return env
}

func removeWork(root string) error {
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
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
		return os.Chmod(path, mode) //nolint:gosec // Private benchmark tree only; linked targets are skipped.
	})
	return errors.Join(err, os.RemoveAll(root)) //nolint:gosec // Private MkdirTemp tree or its computed fixture subtree, never user caches.
}

func describe(cli string) (metadata, error) {
	result := metadata{OS: runtime.GOOS, Arch: runtime.GOARCH, LogicalCPUs: runtime.NumCPU(), GoMaxProcs: runtime.GOMAXPROCS(0), CPU: os.Getenv("PROCESSOR_IDENTIFIER")}
	if data, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
		result.SourceRevision = strings.TrimSpace(string(data))
	}
	if data, err := exec.Command("git", "status", "--porcelain").Output(); err == nil {
		result.SourceModified = fmt.Sprint(len(bytes.TrimSpace(data)) > 0)
	}

	data, err := os.ReadFile(cli) //nolint:gosec // G304: contributor-selected CLI inspected for benchmark provenance.
	if err != nil {
		return result, err
	}
	sum := sha256.Sum256(data)
	result.CLISHA256 = hex.EncodeToString(sum[:])
	info, err := buildinfo.ReadFile(cli)
	if err != nil {
		return result, err
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			result.CLIRevision = setting.Value
		}
		if setting.Key == "vcs.modified" {
			result.CLIModified = setting.Value
		}
	}
	version, err := exec.Command(cli, "--version").Output() //nolint:gosec // G204: configured CLI version probe.
	if err != nil {
		return result, err
	}
	result.CLIVersion = strings.TrimSpace(string(version))
	version, err = exec.Command("go", "version").Output()
	if err != nil {
		return result, err
	}
	result.GoVersion = strings.TrimSpace(string(version))
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "model name") {
				_, result.CPU, _ = strings.Cut(line, ":")
				result.CPU = strings.TrimSpace(result.CPU)
				break
			}
		}
	}
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				result.Memory = strings.TrimSpace(strings.TrimPrefix(line, "MemTotal:"))
				break
			}
		}
	}
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if value, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
				result.OSVersion = strings.Trim(value, "\"")
				break
			}
		}
	}
	if data, err := os.ReadFile("/sys/fs/cgroup/cpu.max"); err == nil {
		result.CPUQuota = strings.TrimSpace(string(data))
	}
	if data, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		result.MemoryLimit = strings.TrimSpace(string(data))
	}
	if runtime.GOOS == "windows" {
		if data, err := exec.Command("cmd.exe", "/D", "/C", "ver").Output(); err == nil {
			result.OSVersion = strings.TrimSpace(string(data))
		}
	}
	if runtime.GOOS == "darwin" {
		if data, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
			result.CPU = strings.TrimSpace(string(data))
		}
		if data, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
			result.Memory = strings.TrimSpace(string(data)) + " bytes"
		}
		if data, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
			result.OSVersion = strings.TrimSpace(string(data))
		}
	}
	return result, nil
}

func dependencyProxy() (*httptest.Server, *atomic.Int64, error) {
	const path, version, mod = "fixture.invalid/startup", "v1.0.0", "module fixture.invalid/startup\ngo 1.26\n"
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for name, text := range map[string]string{"go.mod": mod, "startup.go": "package startup\nfunc Value()string{return \"ok\"}\n"} {
		entry, err := writer.Create(path + "@" + version + "/" + name)
		if err != nil {
			return nil, nil, err
		}
		if _, err := entry.Write([]byte(text)); err != nil {
			return nil, nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, nil, err
	}
	files := map[string][]byte{"/" + path + "/@v/" + version + ".mod": []byte(mod), "/" + path + "/@v/" + version + ".info": []byte(`{"Version":"v1.0.0","Time":"2026-01-01T00:00:00Z"}`), "/" + path + "/@v/" + version + ".zip": data.Bytes()}
	requests := &atomic.Int64{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		data, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	return server, requests, nil
}
