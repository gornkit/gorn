package cache

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/version"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/gornkit/gorn/pkg/script"
	"golang.org/x/mod/modfile"
)

// Resolution records declarations separately from Go's selected graph. The
// effective go.mod and go.sum beside it are the checksum authority.
type Resolution struct {
	Schema       int              `json:"schema"`
	Declared     []script.Require `json:"declared"`
	Selected     []SelectedModule `json:"selected"`
	HelperDigest string           `json:"helper_digest,omitempty"`
}

type SelectedModule struct {
	Path      string `json:"path"`
	Version   string `json:"version"`
	GoVersion string `json:"go_version,omitempty"`
}

type moduleInfo struct {
	Path, Version, GoVersion string
	Main                     bool
	Replace                  *moduleInfo
	Error                    *struct{ Err string }
}

type resolver struct {
	dir, goPath string
	env         []string
}

func newResolver(dir string) (*resolver, error) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("discover Go: %w", err)
	}
	goPath, err = filepath.Abs(goPath)
	if err != nil {
		return nil, err
	}
	r := &resolver{dir: dir, goPath: goPath, env: os.Environ()}
	// Generated modules must not join the user's workspace or download another
	// toolchain. The complete build-profile allowlist is maintained separately.
	r.env = replaceEnv(r.env, "GOWORK", "off")
	r.env = replaceEnv(r.env, "GOTOOLCHAIN", "local")
	r.env = replaceEnv(r.env, "GO111MODULE", "on")
	flags, err := r.command("env", "GOFLAGS")
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(flags)) != 0 {
		return nil, errors.New("dependency policy: nonempty GOFLAGS is not supported by this build profile")
	}
	// Go treats an empty environment value as absent and then reads GOENV.
	// Whitespace is an explicit empty flag list that cannot reactivate stored
	// flags after this validation (for example a persisted -modfile/-toolexec).
	r.env = replaceEnv(r.env, "GOFLAGS", " ")
	return r, nil
}

func replaceEnv(env []string, name, value string) []string {
	prefix := name + "="
	result := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.EqualFold(strings.SplitN(entry, "=", 2)[0], name) {
			result = append(result, entry)
		}
	}
	return append(result, prefix+value)
}

func (r *resolver) command(args ...string) ([]byte, error) {
	cmd := exec.Command(r.goPath, args...) //nolint:gosec // G204: discovered installed Go with internally constructed arguments.
	cmd.Dir, cmd.Env = r.dir, r.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("go %s: %w\n%s", strings.Join(args, " "), err, sanitizeDiagnostic(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// sanitizeDiagnostic removes URL credentials, which Go may include in errors
// from private proxies. Query strings can contain credentials too.
var (
	diagnosticCredentials = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/@\s"'<>]*@`)
	diagnosticURL         = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^\s"'<>]+`)
)

func sanitizeDiagnostic(text string) string {
	// Redact each authority independently before matching whole URLs. Proxy
	// bodies can join several URLs with commas and no intervening whitespace.
	text = diagnosticCredentials.ReplaceAllString(text, "${1}[redacted]@")
	return diagnosticURL.ReplaceAllStringFunc(text, func(url string) string {
		if query := strings.IndexByte(url, '?'); query >= 0 {
			return url[:query] + "?[redacted]"
		}
		return url
	})
}

func (r *resolver) resolve(g *script.Generated, bundle *script.HelperBundle) (*Resolution, error) {
	std, err := r.command("list", "std")
	if err != nil {
		return nil, err
	}
	standard := make(map[string]bool)
	for _, path := range strings.Fields(string(std)) {
		standard[path] = true
	}
	for _, imp := range g.Imports {
		covered := standard[imp.Path] || g.BundleHelper && imp.Path == script.HelperModule
		for _, req := range g.Declared {
			covered = covered || imp.Path == req.Path || strings.HasPrefix(imp.Path, req.Path+"/")
		}
		if !covered {
			return nil, fmt.Errorf("%s: %w", g.SourcePath, &script.LineError{Line: imp.Line, Err: fmt.Errorf("import %q needs an explicit //gorn:require <module> <concrete-version>", imp.Path)})
		}
	}
	result := &Resolution{Schema: 1, Declared: slices.Clone(g.Declared)}
	if bundle != nil {
		result.HelperDigest = bundle.Digest()
	}
	for {
		_, commandErr := r.command("mod", "download", "all")
		_, _, err := r.readModule(g.GoVersion)
		if err != nil {
			return nil, err
		}
		if commandErr != nil {
			return nil, commandErr
		}
		data, err := r.command("list", "-m", "-json", "all")
		if err != nil {
			return nil, err // Never consume partial JSON after a failed Go command.
		}
		mf, original, err := r.readModule(g.GoVersion)
		if err != nil {
			return nil, err
		}
		var selected []SelectedModule
		decoder := json.NewDecoder(bytes.NewReader(data))
		for {
			var m moduleInfo
			if err := decoder.Decode(&m); errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				return nil, fmt.Errorf("decode selected module graph: %w", err)
			}
			if m.Error != nil {
				return nil, fmt.Errorf("resolve %s: %s", m.Path, sanitizeDiagnostic(m.Error.Err))
			}
			if m.Main {
				continue
			}
			if g.BundleHelper && m.Path == script.RunnerModule {
				return nil, errors.New("bundled helper conflicts with selected github.com/gornkit/gorn; explicitly //gorn:require github.com/gornkit/gorn <concrete-version> to use its published helper")
			}
			floor := m.GoVersion
			if m.Replace != nil {
				if !g.BundleHelper || m.Path != script.HelperModule || m.Replace.Path != "./"+script.HelperDir {
					return nil, fmt.Errorf("unexpected replacement for %s", m.Path)
				}
				floor = m.Replace.GoVersion
			}
			if floor != "" && version.Compare("go"+floor, "go"+g.GoVersion) > 0 {
				return nil, fmt.Errorf("dependency %s requires Go %s; raise //gorn:go %s (source declares %s)", m.Path, floor, floor, g.GoVersion)
			}
			selected = append(selected, SelectedModule{Path: m.Path, Version: m.Version, GoVersion: floor})
			if err := mf.AddRequire(m.Path, m.Version); err != nil {
				return nil, err
			}
		}
		mf.Cleanup()
		formatted, err := mf.Format()
		if err != nil {
			return nil, err
		}
		if bytes.Equal(formatted, original) && slices.Equal(selected, result.Selected) {
			return result, nil
		}
		result.Selected = selected
		if err := os.WriteFile(filepath.Join(r.dir, "go.mod"), formatted, 0o600); err != nil {
			return nil, err
		}
	}
}

func (r *resolver) readModule(floor string) (*modfile.File, []byte, error) {
	data, err := os.ReadFile(filepath.Join(r.dir, "go.mod"))
	if err != nil {
		return nil, nil, err
	}
	mf, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil, nil, err
	}
	if mf.Go == nil || mf.Go.Version != floor || mf.Toolchain != nil {
		return nil, nil, fmt.Errorf("dependency resolution changed source Go minimum; raise //gorn:go to satisfy dependency requirements (source declares %s)", floor)
	}
	return mf, data, nil
}

func (r *resolver) saveResolution(result *Resolution) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(r.dir, "dependencies.json"), data, 0o600); err != nil {
		return err
	}
	// Stdlib-only programs have no checksums, but retain a predictable artifact.
	f, err := os.OpenFile(filepath.Join(r.dir, "go.sum"), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}
