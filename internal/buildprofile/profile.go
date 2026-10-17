// Package buildprofile freezes the supported installed-Go configuration once
// for cache identity, subprocess execution, and redacted provenance.
package buildprofile

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/version"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gornkit/gorn/internal/process"
	"github.com/gornkit/gorn/internal/progress"
)

// Record is safe to write as provenance. Linker values and resolver settings
// are represented by digests, never their potentially secret plaintext.
type Record struct {
	Schema           int    `json:"schema"`
	Identity         string `json:"identity"`
	GoPath           string `json:"go_path"`
	GoSHA256         string `json:"go_sha256"`
	GoVersion        string `json:"go_version"`
	GOROOT           string `json:"goroot"`
	GOOS             string `json:"goos"`
	GOARCH           string `json:"goarch"`
	Architecture     string `json:"architecture"`
	SourceGo         string `json:"source_go"`
	Tags             string `json:"tags,omitempty"`
	Trimpath         bool   `json:"trimpath"`
	CGOEnabled       bool   `json:"cgo_enabled"`
	LinkerDigest     string `json:"linker_digest,omitempty"`
	ResolverDigest   string `json:"resolver_digest"`
	GeneratorSHA256  string `json:"generator_sha256"`
	DependencyPolicy string `json:"dependency_policy"`
	HelperDigest     string `json:"helper_digest,omitempty"`
}

// Profile owns immutable execution configuration. Accessors return copies.
type Profile struct {
	record Record
	env    []string
	flags  buildFlags
}

func (r Record) Fingerprint() string {
	r.Identity = ""
	encoded, _ := json.Marshal(r) // A fixed struct of scalars cannot fail JSON encoding.
	return digest(encoded)
}

func (p *Profile) Record() Record        { return p.record }
func (p *Profile) Identity() string      { return p.record.Identity }
func (p *Profile) GoPath() string        { return p.record.GoPath }
func (p *Profile) Environment() []string { return append([]string(nil), p.env...) }

func (p *Profile) BuildArgs(output string) []string {
	args := []string{"build", "-mod=readonly", "-buildvcs=false", "-pgo=off"}
	args = append(args, p.flags.args()...)
	return append(args, "-o", output, ".")
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func digestFile(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // G304: selected installed toolchain or executing CLI path.
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, contextReader{ctx, f}); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

var capturedSettings = []string{
	"GOROOT", "GOOS", "GOARCH", "GOAMD64", "GOARM", "GOARM64", "GO386", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM",
	"GOPATH", "GOMODCACHE", "GOCACHE", "GOTMPDIR", "GOPROXY", "GOSUMDB", "GONOSUMDB", "GONOPROXY", "GOPRIVATE", "GOINSECURE", "GOAUTH", "GOVCS",
}

func queryGo(path string, env []string) (map[string]string, error) {
	return queryGoContext(context.Background(), path, env)
}

func queryGoContext(ctx context.Context, path string, env []string) (map[string]string, error) {
	// Query stable settings explicitly. The all-settings form also requests
	// unrelated costly compiler information such as GOGCCFLAGS.
	args := append([]string{"env", "-json", "GOENV", "GOVERSION", "GOFLAGS", "GOEXPERIMENT", "GOFIPS140", "GOCACHEPROG"}, capturedSettings...)
	cmd := exec.Command(path, args...) //nolint:gosec // G204: installed Go discovery, no build or user-supplied command.
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := process.Run(ctx, cmd); err != nil {
		// Discovery can encounter malformed private settings. Do not dump raw
		// configuration; the preserved cause still distinguishes exec failures.
		return nil, fmt.Errorf("discover installed Go configuration: %w", err)
	}
	var values map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &values); err != nil {
		return nil, fmt.Errorf("decode installed Go configuration: %w", err)
	}
	return values, nil
}

func setEnv(env []string, name, value string) []string {
	result := make([]string, 0, len(env)+1)
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, name) {
			result = append(result, entry)
		}
	}
	return append(result, name+"="+value)
}

func configured(values map[string]string, env []string) (map[string]string, error) {
	result := make(map[string]string)
	paths := []string{}
	if root := values["GOROOT"]; root != "" {
		paths = append(paths, filepath.Join(root, "go.env"))
	}
	if user := values["GOENV"]; user != "" && user != "off" {
		paths = append(paths, user)
	}
	// The SDK defaults come first, user-file entries override them (including
	// empty values), and nonempty process values have the highest precedence.
	for _, path := range paths {
		data, err := os.ReadFile(path) //nolint:gosec // G304: Go's selected configuration file, read only for explicit-setting validation.
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read selected GOENV: %w", err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			key, value, ok := strings.Cut(line, "=")
			if ok && key != "" && key[0] >= 'A' && key[0] <= 'Z' {
				result[key] = value
			}
		}
	}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		if runtime.GOOS == "windows" {
			key = strings.ToUpper(key)
		}
		// Empty Go environment values fall back to the persisted setting.
		if value != "" {
			result[key] = value
		}
	}
	return result, nil
}

// Discover selects the installed SDK (including through a tool-manager launcher),
// then executes that SDK directly. It performs no module downloads or builds.
func Discover(sourceGo, helperDigest string) (*Profile, error) {
	return DiscoverContext(context.Background(), sourceGo, helperDigest)
}

// MinimumGoVersion is the supported installed compiler floor, distinct from a
// script's language minimum. Keep it aligned with the parent module's go line.
const MinimumGoVersion = "1.26.4"

// DiscoverContext is Discover with cancellation of discovery subprocesses and hashing.
func DiscoverContext(ctx context.Context, sourceGo, helperDigest string) (*Profile, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil discovery context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := progress.Phase(ctx, "discovering installed Go"); err != nil {
		return nil, err
	}
	launcher, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("discover Go: %w", err)
	}
	launcher, err = filepath.Abs(launcher)
	if err != nil {
		return nil, err
	}
	base := os.Environ()
	probe := setEnv(setEnv(setEnv(setEnv(base, "GOTOOLCHAIN", "local"), "GOWORK", "off"), "GO111MODULE", "on"), "GOFLAGS", " ")
	values, err := queryGoContext(ctx, launcher, probe)
	if err != nil {
		return nil, err
	}
	explicit, err := configured(values, base)
	if err != nil {
		return nil, err
	}
	if err := validateSettings(values, explicit); err != nil {
		return nil, err
	}
	flags, err := parseFlags(explicit["GOFLAGS"])
	if err != nil {
		return nil, err
	}
	if !version.IsValid(values["GOVERSION"]) {
		return nil, fmt.Errorf("installed Go must report a release, beta, or RC version")
	}
	if version.Compare(values["GOVERSION"], "go"+MinimumGoVersion) < 0 {
		return nil, fmt.Errorf("gorn requires Go %s or newer; installed toolchain is %s", MinimumGoVersion, values["GOVERSION"])
	}
	if version.Compare(values["GOVERSION"], "go"+sourceGo) < 0 {
		return nil, fmt.Errorf("source requires Go %s; installed toolchain is %s", sourceGo, values["GOVERSION"])
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	var goPath string
	// A real Go executable selected directly from PATH remains the selected
	// command, including a fork/copy sharing a GOROOT. A tool-manager launcher
	// instead identifies the SDK executable through its reported GOROOT.
	if selected, err := filepath.EvalSymlinks(launcher); err == nil {
		if info, err := buildinfo.ReadFile(selected); err == nil && info.Path == "cmd/go" {
			goPath = selected
		}
	}
	if goPath == "" {
		goPath, err = filepath.EvalSymlinks(filepath.Join(values["GOROOT"], "bin", "go"+suffix))
		if err != nil {
			return nil, fmt.Errorf("locate selected installed Go SDK: %w", err)
		}
	}
	goPath, err = filepath.Abs(goPath)
	if err != nil {
		return nil, err
	}
	info, err := buildinfo.ReadFile(goPath)
	if err != nil || info.Path != "cmd/go" {
		return nil, fmt.Errorf("selected GOROOT must contain an installed Go executable")
	}
	goSHA, err := digestFile(ctx, goPath)
	if err != nil {
		return nil, fmt.Errorf("hash selected Go executable: %w", err)
	}
	generator, err := os.Executable()
	if err != nil {
		return nil, err
	}
	generatorSHA, err := digestFile(ctx, generator)
	if err != nil {
		return nil, fmt.Errorf("hash executing gorn: %w", err)
	}
	// Capture settings that Go persists, then disable future GOENV reads. Non-Go
	// process environment remains available for normal authentication helpers.
	env := append([]string(nil), base...)
	settings := make(map[string]string)
	for _, name := range capturedSettings {
		value := values[name]
		settings[name] = value
		env = setEnv(env, name, value)
	}
	for name, value := range map[string]string{
		"GOENV": "off", "GOWORK": "off", "GOTOOLCHAIN": "local", "GO111MODULE": "on", "CGO_ENABLED": "0", "GOFLAGS": " ",
		"GOEXPERIMENT": "", "GOFIPS140": "off", "GOCACHEPROG": "", "GODEBUG": "", "GO_EXTLINK_ENABLED": "0",
	} {
		env = setEnv(env, name, value)
	}
	actual, err := queryGoContext(ctx, goPath, env)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"GOVERSION", "GOROOT", "GOOS", "GOARCH"} {
		if actual[name] != values[name] {
			return nil, fmt.Errorf("installed SDK disagrees with launcher about %s", name)
		}
	}
	for _, name := range capturedSettings {
		if actual[name] != values[name] {
			return nil, fmt.Errorf("cannot freeze Go setting %s without changing its effective value; use an explicit supported nonempty value or reset the setting", name)
		}
	}
	for name, expected := range map[string]string{"GOFLAGS": " ", "GOEXPERIMENT": "", "GOFIPS140": "off", "GOCACHEPROG": ""} {
		if actual[name] != expected {
			return nil, fmt.Errorf("cannot freeze controlled Go setting %s; reset the unsupported SDK/user configuration", name)
		}
	}
	resolverJSON, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	architecture := ""
	for _, name := range []string{"GOAMD64", "GOARM", "GOARM64", "GO386", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM"} {
		if values[name] != "" {
			architecture += name + "=" + values[name] + ";"
		}
	}
	record := Record{
		Schema: 1, GoPath: goPath, GoSHA256: goSHA, GoVersion: actual["GOVERSION"], GOROOT: actual["GOROOT"], GOOS: actual["GOOS"], GOARCH: actual["GOARCH"],
		Architecture: architecture, SourceGo: sourceGo, Tags: flags.tags, Trimpath: flags.trimpath, ResolverDigest: digest(resolverJSON),
		GeneratorSHA256: generatorSHA, DependencyPolicy: "pinned-graph-v1", HelperDigest: helperDigest,
	}
	if flags.linker != "" {
		record.LinkerDigest = digest([]byte(flags.linker))
	}
	record.Identity = record.Fingerprint()
	return &Profile{record: record, env: env, flags: flags}, nil
}

func validateSettings(values, explicit map[string]string) error {
	if values["GOOS"] != runtime.GOOS || values["GOARCH"] != runtime.GOARCH {
		return fmt.Errorf("foreign targets are unsupported: requested %s/%s, host is %s/%s", values["GOOS"], values["GOARCH"], runtime.GOOS, runtime.GOARCH)
	}
	if value := explicit["CGO_ENABLED"]; value != "" && value != "0" {
		return fmt.Errorf("explicit CGO_ENABLED must be 0; this profile supports pure Go")
	}
	for _, name := range []string{"CC", "CXX", "AR", "GCCGO", "PKG_CONFIG", "CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS", "CGO_FFLAGS", "CGO_LDFLAGS", "GO_EXTLINK_ENABLED", "GOCOMPILEDEBUG", "GOCLOBBERDEADHASH", "GOSSAFUNC", "GOSSADIR"} {
		if explicit[name] != "" {
			return fmt.Errorf("unsupported cgo/compiler setting %s", name)
		}
	}
	if values["GOEXPERIMENT"] != "" || values["GOFIPS140"] != "" && values["GOFIPS140"] != "off" || values["GOCACHEPROG"] != "" {
		return fmt.Errorf("custom GOEXPERIMENT, GOFIPS140, and GOCACHEPROG settings are unsupported")
	}
	for _, name := range []string{"GOAMD64", "GOARM", "GOARM64", "GO386", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM"} {
		if value, present := values[name]; present && explicit[name] != "" && explicit[name] != value {
			return fmt.Errorf("invalid architecture setting %s", name)
		}
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
