package buildprofile

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDiscoverFreezesSettingsAndRedactsProvenance(t *testing.T) {
	config := filepath.Join(t.TempDir(), "goenv")
	if err := os.WriteFile(config, []byte("GOFLAGS=-tags=selected -trimpath\nGOPROXY=https://fixture-user:fixture-password@proxy.invalid?key=fixture-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOENV", config)
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOPROXY", "")
	p, err := Discover("1.26", "helper-a")
	if err != nil {
		t.Fatal(err)
	}
	if p.Record().Tags != "selected" || !p.Record().Trimpath || p.Record().Identity != p.Record().Fingerprint() {
		t.Fatalf("wrong profile: %+v", p.Record())
	}
	if err := os.WriteFile(config, []byte("GOFLAGS=-toolexec=must-not-run\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := queryGo(p.GoPath(), p.Environment())
	if err != nil || strings.TrimSpace(values["GOFLAGS"]) != "" || values["GOPROXY"] != "https://fixture-user:fixture-password@proxy.invalid?key=fixture-token" {
		t.Fatalf("profile reread mutable configuration: %v", err)
	}
	encoded, err := json.Marshal(p.Record())
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fixture-user", "fixture-password", "fixture-token"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("provenance leaked resolver credential %s", secret)
		}
	}
	env := p.Environment()
	env[0] = "mutated=true"
	if p.Environment()[0] == env[0] {
		t.Fatal("environment accessor exposed mutable storage")
	}
}

func TestDiscoverIdentityTracksSupportedFlagsNotRuntimeEnvironment(t *testing.T) {
	t.Setenv("GOENV", "off")
	t.Setenv("GOFLAGS", " ")
	base, err := Discover("1.26", "helper-a")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GORN_APPLICATION_TEST", "ordinary-runtime-value")
	again, err := Discover("1.26", "helper-a")
	if err != nil || base.Identity() != again.Identity() {
		t.Fatalf("runtime environment invalidated build: %v", err)
	}
	t.Setenv("GOFLAGS", `"-ldflags=-X main.Message=private-linker-value"`)
	changed, err := Discover("1.26", "helper-a")
	if err != nil || changed.Identity() == base.Identity() {
		t.Fatalf("linker change did not invalidate profile: %v", err)
	}
	encoded, _ := json.Marshal(changed.Record())
	if strings.Contains(string(encoded), "private-linker-value") || changed.Record().LinkerDigest == "" {
		t.Fatal("linker provenance was not hashed")
	}
	helper, err := Discover("1.26", "helper-b")
	if err != nil || helper.Identity() == changed.Identity() {
		t.Fatalf("helper change did not invalidate profile: %v", err)
	}
}

func TestUnsupportedCompilerInputsRejectedBeforeDiscoveryBuilds(t *testing.T) {
	t.Setenv("GOENV", "off")
	t.Setenv("GOFLAGS", " ")
	for _, name := range []string{"CGO_ENABLED", "CC", "GO_EXTLINK_ENABLED", "GOCOMPILEDEBUG", "GOCLOBBERDEADHASH", "GOSSAFUNC", "GOSSADIR", "GOEXPERIMENT", "GOFIPS140", "GOCACHEPROG"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "1")
			if _, err := Discover("1.26", ""); err == nil {
				t.Fatalf("accepted %s", name)
			}
		})
	}
}

func TestWindowsMixedCaseCompilerSetting(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows environment names are case insensitive")
	}
	t.Setenv("GOENV", "off")
	t.Setenv("cgo_enabled", "1")
	if _, err := Discover("1.26", ""); err == nil || !strings.Contains(err.Error(), "CGO_ENABLED") {
		t.Fatalf("mixed-case explicit cgo bypassed validation: %v", err)
	}
}

func TestSDKDefaultsUserConfigAndProcessPrecedence(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.env"), []byte("# defaults\nGOFLAGS=-tags=sdk\nGOPROXY=https://sdk.invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	user := filepath.Join(t.TempDir(), "goenv")
	if err := os.WriteFile(user, []byte("GOFLAGS=-tags=user\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		config string
		env    []string
		want   string
	}{
		{"off", nil, "-tags=sdk"},
		{user, nil, "-tags=user"},
		{user, []string{"GOFLAGS="}, "-tags=user"},
		{user, []string{"GOFLAGS= "}, " "},
		{user, []string{"GOFLAGS=-tags=process"}, "-tags=process"},
	} {
		got, err := configured(map[string]string{"GOROOT": root, "GOENV": tc.config}, tc.env)
		if err != nil || got["GOFLAGS"] != tc.want || got["GOPROXY"] != "https://sdk.invalid" {
			t.Fatalf("precedence: %v %v", got, err)
		}
	}
}

func TestEmptySavedSettingCannotSilentlyReactivateSDKDefault(t *testing.T) {
	config := filepath.Join(t.TempDir(), "goenv")
	for _, name := range []string{"GOPROXY", "GOSUMDB"} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(config, []byte(name+"=\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GOENV", config)
			t.Setenv(name, "")
			t.Setenv("GOFLAGS", " ")
			_, err := Discover("1.26", "")
			if err == nil || !strings.Contains(err.Error(), "cannot freeze Go setting "+name) {
				t.Fatalf("empty %s silently changed to SDK default: %v", name, err)
			}
		})
	}
}

func TestDirectGoNeedsNoSecondExecutableInGOROOT(t *testing.T) {
	t.Setenv("GOENV", "off")
	t.Setenv("GOFLAGS", " ")
	original, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, name := range []string{"src", "pkg"} {
		if err := os.Symlink(filepath.Join(strings.TrimSpace(string(original)), name), filepath.Join(root, name)); err != nil {
			t.Skipf("data-only SDK fixture requires directory symlinks: %v", err)
		}
	}
	t.Setenv("GOROOT", root)
	p, err := Discover("1.26", "")
	if err != nil {
		t.Fatalf("valid direct Go executable rejected: %v", err)
	}
	if p.Record().GOROOT != root {
		t.Fatalf("GOROOT changed: %s", p.Record().GOROOT)
	}
	if _, err := os.Stat(filepath.Join(root, "bin", "go")); !os.IsNotExist(err) {
		t.Fatalf("fixture unexpectedly has another Go executable: %v", err)
	}
}

func TestInstalledCompilerFloorPrecedesIdentity(t *testing.T) {
	t.Setenv("GOENV", "off")
	t.Setenv("GOFLAGS", " ")
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	values, err := queryGo(goPath, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	values["GOVERSION"] = "go1.26.3"
	data, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "probe.go")
	if err := os.WriteFile(source, []byte("package main\nimport(\"fmt\";\"os\")\nfunc main(){fmt.Print(os.Getenv(\"GORN_TEST_OLD_GO_PROBE\"))}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(dir, "go")
	if runtime.GOOS == "windows" {
		launcher += ".exe"
	}
	build := exec.Command(goPath, "build", "-o", launcher, source) //nolint:gosec // G204: controlled installed Go and private fixture paths.
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build SDK probe: %v\n%s", err, out)
	}
	t.Setenv("GORN_TEST_OLD_GO_PROBE", string(data))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, sourceGo := range []string{"1.20", "1.26"} {
		if _, err := Discover(sourceGo, ""); err == nil || !strings.Contains(err.Error(), "gorn requires Go "+MinimumGoVersion) || !strings.Contains(err.Error(), "go1.26.3") {
			t.Fatalf("unsupported compiler reached identity/lookup with source %s: %v", sourceGo, err)
		}
	}
}

func TestCompilerFloorMatchesParentModule(t *testing.T) {
	data, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "go" {
			if fields[1] != MinimumGoVersion {
				t.Fatalf("installed/compiler floor %s differs from module floor %s", MinimumGoVersion, fields[1])
			}
			return
		}
	}
	t.Fatal("parent module has no Go minimum")
}
