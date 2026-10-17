package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstalledCLIProfileFlagsAndWarmValidation(t *testing.T) {
	bin := builtCLI(t)
	t.Setenv("GOENV", "off")
	t.Setenv("GOPROXY", "off")
	root := t.TempDir()
	t.Setenv("GORN_CACHE", root)
	dir := t.TempDir()
	path := writeScript(t, dir, "profile.gorn", "//go:build selected\n\npackage main\nimport \"fmt\"\nvar Message string\nfunc main(){fmt.Print(Message)}")
	t.Setenv("GOFLAGS", `-tags=z,selected,z -trimpath "-ldflags=-X 'main.Message=private value \\ literal'"`)
	cold := invokeCLI(t, bin, dir, "", "run", "-v", path)
	if cold.code != 0 || cold.stdout != `private value \\ literal` || !strings.Contains(cold.stderr, "cache: miss") {
		t.Fatalf("cold profile: %+v", cold)
	}
	t.Setenv("GOFLAGS", `-tags=selected,z -trimpath=true "-ldflags=-X 'main.Message=private value \\ literal'"`)
	t.Setenv("GORN_APPLICATION_TEST", "runtime-only")
	warm := invokeCLI(t, bin, dir, "", "run", "-v", path)
	if warm.code != 0 || warm.stdout != cold.stdout || !strings.Contains(warm.stderr, "cache: hit") {
		t.Fatalf("normalized warm profile: %+v", warm)
	}
	manifests, err := filepath.Glob(filepath.Join(root, "apps", "*", "*", "manifest.json"))
	if err != nil || len(manifests) != 1 {
		t.Fatalf("profile manifests: %v %v", manifests, err)
	}
	data, err := os.ReadFile(manifests[0])
	if err != nil || strings.Contains(string(data), "private value") || !strings.Contains(string(data), "linker_digest") {
		t.Fatalf("private profile provenance: %v %s", err, data)
	}
	t.Setenv("GOFLAGS", `-tags=selected "-ldflags=-X main.Message=changed"`)
	changed := invokeCLI(t, bin, dir, "", "run", "-v", path)
	if changed.code != 0 || changed.stdout != "changed" || !strings.Contains(changed.stderr, "cache: miss") {
		t.Fatalf("changed profile: %+v", changed)
	}
	for _, flag := range []string{"-race", "-overlay=private-path", "-modfile=private-path", "-toolexec=private-path", "-pgo=private-path"} {
		t.Setenv("GOFLAGS", flag)
		got := invokeCLI(t, bin, dir, "", "run", path)
		if got.code != 1 || got.stdout != "" || !strings.Contains(got.stderr, "unsupported GOFLAGS") || strings.Contains(got.stderr, "private-path") {
			t.Fatalf("warm unsupported flag %s: %+v", flag, got)
		}
	}
	t.Setenv("GOFLAGS", " ")
	for _, name := range []string{"CGO_ENABLED", "GO_EXTLINK_ENABLED", "GOCOMPILEDEBUG", "GOCLOBBERDEADHASH", "GOSSAFUNC", "GOSSADIR"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "1")
			got := invokeCLI(t, bin, dir, "", "run", path)
			if got.code != 1 || got.stdout != "" || !strings.Contains(got.stderr, name) {
				t.Fatalf("warm unsupported %s: %+v", name, got)
			}
		})
	}
	foreign := "windows"
	if runtime.GOOS == foreign {
		foreign = "linux"
	}
	t.Setenv("GOOS", foreign)
	got := invokeCLI(t, bin, dir, "", "run", path)
	if got.code != 1 || got.stdout != "" || !strings.Contains(got.stderr, "foreign targets") {
		t.Fatalf("foreign target was not rejected: %+v", got)
	}
}

func TestInstalledCLIGeneratorIdentityChangesWithSameVersion(t *testing.T) {
	first := builtCLI(t)
	second := builtCLI(t, "-ldflags=-X main.Version=dev")
	t.Setenv("GOENV", "off")
	t.Setenv("GOFLAGS", " ")
	t.Setenv("GORN_CACHE", t.TempDir())
	dir := t.TempDir()
	path := writeScript(t, dir, "generator.gorn", "package main\nfunc main(){}")
	for _, bin := range []string{first, second} {
		version := invokeCLI(t, bin, dir, "", "--version")
		if version.code != 0 || !strings.Contains(version.stdout, "dev") {
			t.Fatalf("test binaries must share version: %+v", version)
		}
		got := invokeCLI(t, bin, dir, "", "run", "-v", path)
		if got.code != 0 || !strings.Contains(got.stderr, "cache: miss") {
			t.Fatalf("generator bytes did not invalidate cache: %+v", got)
		}
	}
}

func TestSourceBuildInputsRejectRealDirectivesOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles generated programs")
	}
	t.Setenv("GORN_CACHE", t.TempDir())
	dir := t.TempDir()
	for _, source := range []string{
		"package main\nimport \"C\"\nfunc main(){}",
		"package main\nimport _ \"embed\"\n//go:embed data.txt\nvar data string\nfunc main(){}",
	} {
		path := writeScript(t, dir, "unsupported.gorn", source)
		got := runCLIForTest(t, "run", path)
		if got.err == nil || !strings.Contains(got.err.Error(), "unsupported") || !strings.Contains(got.err.Error(), path) || !strings.Contains(got.err.Error(), "at line") {
			t.Fatalf("source input accepted: %+v", got)
		}
		inspect := runCLIForTest(t, "run", "--print-main", path)
		if inspect.err != nil || !strings.Contains(inspect.stdout, source) {
			t.Fatalf("inspection lost source: %+v", inspect)
		}
	}
	path := writeScript(t, dir, "literal.gorn", "package main\n/* //go:embed missing */\nfunc main(){_ = `\n//go:embed missing\n`}")
	if got := runCLIForTest(t, "run", path); got.err != nil {
		t.Fatalf("directive-looking literal/comment rejected: %v", got.err)
	}
}

func TestInstalledCLISelectedToolchainPathChangesIdentity(t *testing.T) {
	bin := builtCLI(t)
	t.Setenv("GOENV", "off")
	t.Setenv("GOFLAGS", " ")
	t.Setenv("GORN_CACHE", t.TempDir())
	dir := t.TempDir()
	path := writeScript(t, dir, "toolchain.gorn", "package main\nfunc main(){}")
	first := invokeCLI(t, bin, dir, "", "run", "-v", path)
	if first.code != 0 || !strings.Contains(first.stderr, "cache: miss") {
		t.Fatalf("initial compiler: %+v", first)
	}
	root, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatal(err)
	}
	selected, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(selected) //nolint:gosec // G304: selected installed Go executable copied into a private test fixture.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()
	tools := t.TempDir()
	name := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dest, err := os.OpenFile(filepath.Join(tools, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700) //nolint:gosec // G302: copied Go executable must be executable by the test user.
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(dest, source)
	closeErr := dest.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("copy installed Go: %v %v", copyErr, closeErr)
	}
	t.Setenv("GOROOT", strings.TrimSpace(string(root)))
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, want := range []string{"cache: miss", "cache: hit"} {
		got := invokeCLI(t, bin, dir, "", "run", "-v", path)
		if got.code != 0 || !strings.Contains(got.stderr, want) {
			t.Fatalf("selected copied toolchain (%s): %+v", want, got)
		}
	}
}
