package cache

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gornkit/gorn/internal/app"
	"github.com/gornkit/gorn/internal/buildprofile"
	"github.com/gornkit/gorn/internal/script"
	"golang.org/x/mod/module"
)

// fileProxy is a real Go module proxy with no network or local replacements.
func fileProxy(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	proxyPath := filepath.ToSlash(root)
	// URL paths need a leading slash for Windows drive-letter paths.
	if !strings.HasPrefix(proxyPath, "/") {
		proxyPath = "/" + proxyPath
	}
	proxyURL := (&url.URL{Scheme: "file", Path: proxyPath}).String()
	t.Setenv("GOPROXY", proxyURL)
	t.Setenv("GOSUMDB", "off")
	modcache := t.TempDir()
	t.Setenv("GOMODCACHE", modcache)
	// Go extracts immutable module directories; make only this private fixture
	// writable before testing's TempDir cleanup (including Windows file modes).
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
			return os.Chmod(path, mode) //nolint:gosec // G122: private fixture with no concurrent writers or untrusted paths.
		})
	})
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	return root
}

func proxyModule(t *testing.T, root, path, version, mod string, files map[string]string) {
	t.Helper()
	escapedPath, err := module.EscapePath(path)
	if err != nil {
		t.Fatal(err)
	}
	escapedVersion, err := module.EscapeVersion(version)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, filepath.FromSlash(escapedPath), "@v")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, escapedVersion+name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeFixture(".mod", mod)
	writeFixture(".info", fmt.Sprintf(`{"Version":%q,"Time":"2026-01-01T00:00:00Z"}`, version))
	f, err := os.Create(filepath.Join(dir, escapedVersion+".zip")) //nolint:gosec // G304: controlled file-proxy fixture.
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	all := make(map[string]string, len(files)+1)
	for name, text := range files {
		all[name] = text
	}
	all["go.mod"] = mod
	for name, text := range all {
		w, err := zw.Create(path + "@" + version + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func buildFixture(t *testing.T, source string) (Entry, error) {
	t.Helper()
	s, err := app.New(filepath.Join(t.TempDir(), "source.gorn"), []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	f, err := script.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	g, err := script.Generate(f)
	if err != nil {
		t.Fatal(err)
	}
	var bundle *script.HelperBundle
	if g.BundleHelper {
		bundle, err = script.LoadHelper(os.DirFS("../.."))
		if err != nil {
			t.Fatal(err)
		}
	}
	helperDigest := ""
	if bundle != nil {
		helperDigest = bundle.Digest()
	}
	profile, err := buildprofile.Discover(g.GoVersion, helperDigest)
	if err != nil {
		return Entry{}, err
	}
	s = s.WithBuildIdentity(profile.Identity())
	entry, err := Root(t.TempDir()).Build(s, g, true, bundle, profile)
	if err == nil {
		t.Cleanup(func() { _ = entry.Remove() })
	}
	return entry, err
}

func TestPinnedGraphExpandsLazyTransitivesAndMVS(t *testing.T) {
	if testing.Short() {
		t.Skip("builds offline module graph")
	}
	proxy := fileProxy(t)
	proxyModule(t, proxy, "test.invalid/a", "v1.0.0", "module test.invalid/a\ngo 1.26\nrequire test.invalid/b v1.0.0\n", map[string]string{"a.go": "package a\nimport \"test.invalid/b\"\nfunc Value()string{return b.Value()}"})
	proxyModule(t, proxy, "test.invalid/d", "v1.0.0", "module test.invalid/d\ngo 1.26\nrequire test.invalid/b v1.1.0\n", map[string]string{"d.go": "package d"})
	proxyModule(t, proxy, "test.invalid/b", "v1.0.0", "module test.invalid/b\ngo 1.26\n", map[string]string{"b.go": "package b\nfunc Value()string{return \"wrong\"}"})
	proxyModule(t, proxy, "test.invalid/b", "v1.1.0", "module test.invalid/b\ngo 1.26\nrequire test.invalid/c v1.0.0\n", map[string]string{"b.go": "package b\nimport \"test.invalid/c\"\nfunc Value()string{return c.Value()}"})
	// Module cycles are legal; the package dependency graph remains acyclic.
	proxyModule(t, proxy, "test.invalid/c", "v1.0.0", "module test.invalid/c\ngo 1.26\nrequire test.invalid/a v1.0.0\n", map[string]string{"c.go": "package c\nfunc Value()string{return \"resolved\"}"})
	entry, err := buildFixture(t, "//gorn:require test.invalid/a v1.0.0\n//gorn:require test.invalid/d v1.0.0\npackage main\nimport (\"fmt\";\"test.invalid/a\")\nfunc main(){fmt.Print(a.Value())}")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(entry.AppPath).CombinedOutput() //nolint:gosec // G204: executable built from controlled fixture.
	if err != nil || string(out) != "resolved" {
		t.Fatalf("run: %v %q", err, out)
	}
	data, err := os.ReadFile(filepath.Join(entry.AppRoot, "dependencies.json"))
	if err != nil {
		t.Fatal(err)
	}
	var resolution Resolution
	if err := json.Unmarshal(data, &resolution); err != nil {
		t.Fatal(err)
	}
	if len(resolution.Declared) != 2 || len(resolution.Selected) != 4 {
		t.Fatalf("lost declared/selected distinction: %s", data)
	}
	for _, m := range resolution.Selected {
		if m.Path == "test.invalid/b" && m.Version != "v1.1.0" {
			t.Fatalf("MVS upgrade lost: %+v", m)
		}
	}
	sum, err := os.ReadFile(filepath.Join(entry.AppRoot, "go.sum"))
	if err != nil || !strings.Contains(string(sum), "test.invalid/c v1.0.0 h1:") {
		t.Fatalf("missing checksum provenance: %v %s", err, sum)
	}
	// A fresh generated build can resolve from populated Go caches offline.
	t.Setenv("GOPROXY", "off")
	if _, err := buildFixture(t, "//gorn:require test.invalid/a v1.0.0\n//gorn:require test.invalid/d v1.0.0\npackage main\nimport _ \"test.invalid/a\"\nfunc main(){}"); err != nil {
		t.Fatalf("populated offline graph: %v", err)
	}
	// Incidental transitive selection never authorizes a direct source import.
	_, err = buildFixture(t, "//gorn:require test.invalid/a v1.0.0\npackage main\nimport _ \"test.invalid/b\"\nfunc main(){}")
	if err == nil || !strings.Contains(err.Error(), "at line 3") || !strings.Contains(err.Error(), "//gorn:require") {
		t.Fatalf("undeclared transitive direct import accepted: %v", err)
	}
}

func TestDependencyPolicyRejectsUndeclaredDirectImport(t *testing.T) {
	if testing.Short() {
		t.Skip("uses installed Go")
	}
	fileProxy(t) // An empty proxy proves no version discovery can rescue it.
	_, err := buildFixture(t, "package main\nimport _ \"test.invalid/missing\"\nfunc main(){}")
	if err == nil || !strings.Contains(err.Error(), "at line 2") || !strings.Contains(err.Error(), "//gorn:require") {
		t.Fatalf("missing direct declaration: %v", err)
	}
}

func TestMissingTransitivePackageDoesNotDiscoverLatest(t *testing.T) {
	if testing.Short() {
		t.Skip("builds offline module graph")
	}
	proxy := fileProxy(t)
	proxyModule(t, proxy, "test.invalid/broken", "v1.0.0", "module test.invalid/broken\ngo 1.26\n", map[string]string{"broken.go": "package broken\nimport _ \"test.invalid/undeclared\""})
	// This module exists in the proxy but is absent from the declared graph.
	proxyModule(t, proxy, "test.invalid/undeclared", "v1.0.0", "module test.invalid/undeclared\ngo 1.26\n", map[string]string{"undeclared.go": "package undeclared"})
	_, err := buildFixture(t, "//gorn:require test.invalid/broken v1.0.0\npackage main\nimport _ \"test.invalid/broken\"\nfunc main(){}")
	if err == nil || !strings.Contains(err.Error(), "//gorn:require") || !strings.Contains(err.Error(), "test.invalid/undeclared") {
		t.Fatalf("missing graph import did not fail with a declaration remedy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("GOMODCACHE"), "test.invalid", "undeclared@v1.0.0")); !os.IsNotExist(err) { //nolint:gosec // G703: GOMODCACHE is set to a private TempDir in this test.
		t.Fatalf("undeclared module was downloaded: %v", err)
	}
}

func TestBundledAndPublishedHelpers(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles helpers")
	}
	proxy := fileProxy(t)
	for _, source := range []string{
		"//gorn:preamble\npackage main\nfunc main(){fmt.Print(sh.PreambleSentinel)}",
		"package main\nimport \"github.com/gornkit/gorn/sh\"\nfunc main(){_ = sh.PreambleSentinel}",
	} {
		entry, err := buildFixture(t, source)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(entry.AppRoot, "dependencies.json"))
		if err != nil || !strings.Contains(string(data), "helper_digest") {
			t.Fatalf("missing bundled identity: %v %s", err, data)
		}
	}
	proxyModule(t, proxy, script.RunnerModule, "v1.0.0", "module github.com/gornkit/gorn\ngo 1.26\n", map[string]string{"sh/sh.go": "package sh\nconst External = 42"})
	entry, err := buildFixture(t, "//gorn:require github.com/gornkit/gorn v1.0.0\npackage main\nimport (\"fmt\";\"github.com/gornkit/gorn/sh\")\nfunc main(){fmt.Print(sh.External)}")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(entry.AppPath).CombinedOutput() //nolint:gosec // G204: controlled fixture.
	if err != nil || string(out) != "42" {
		t.Fatalf("published helper: %v %q", err, out)
	}
	if _, err := os.Stat(filepath.Join(entry.AppRoot, script.HelperDir)); !os.IsNotExist(err) {
		t.Fatalf("external mode contains bundle: %v", err)
	}
	proxyModule(t, proxy, "test.invalid/parent", "v1.0.0", "module test.invalid/parent\ngo 1.26\nrequire github.com/gornkit/gorn v1.0.0\n", map[string]string{"parent.go": "package parent"})
	_, err = buildFixture(t, "//gorn:preamble\n//gorn:require test.invalid/parent v1.0.0\npackage main\nfunc main(){}")
	if err == nil || !strings.Contains(err.Error(), "explicitly //gorn:require github.com/gornkit/gorn") {
		t.Fatalf("ambiguous helper graph accepted: %v", err)
	}
}

func TestResolutionPreservesSourceGoFloor(t *testing.T) {
	if testing.Short() {
		t.Skip("resolves modules")
	}
	proxy := fileProxy(t)
	proxyModule(t, proxy, "test.invalid/newgo", "v1.0.0", "module test.invalid/newgo\ngo 1.26.4\n", map[string]string{"newgo.go": "package newgo"})
	_, err := buildFixture(t, "//gorn:go 1.26\n//gorn:require test.invalid/newgo v1.0.0\npackage main\nimport _ \"test.invalid/newgo\"\nfunc main(){}")
	if err == nil || !strings.Contains(err.Error(), "raise //gorn:go") {
		t.Fatalf("source floor silently raised: %v", err)
	}
	if _, err := buildFixture(t, "//gorn:go 1.26.4\n//gorn:require test.invalid/newgo v1.0.0\npackage main\nimport _ \"test.invalid/newgo\"\nfunc main(){}"); err != nil {
		t.Fatalf("explicit floor rejected: %v", err)
	}
}

func TestEmptyEffectiveFlagsDoNotReactivatePersistedGOFLAGS(t *testing.T) {
	if testing.Short() {
		t.Skip("uses installed Go configuration")
	}
	fileProxy(t)
	dir := t.TempDir()
	config := filepath.Join(dir, "goenv")
	if err := os.WriteFile(config, []byte("GOFLAGS=-modfile="+filepath.Join(dir, "must-not-be-read.mod")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOENV", config)
	t.Setenv("GOFLAGS", " ")
	if _, err := buildFixture(t, "package main\nfunc main(){}"); err != nil {
		t.Fatalf("persisted flags reactivated after explicit empty override: %v", err)
	}
	t.Setenv("GOFLAGS", "") // Empty falls back to the persisted flags and must fail.
	if _, err := buildFixture(t, "package main\nfunc main(){}"); err == nil || !strings.Contains(err.Error(), "unsupported GOFLAGS") {
		t.Fatalf("persisted unsafe flags were not rejected: %v", err)
	}
}

func TestDependencyDiagnosticRedaction(t *testing.T) {
	text := `Get "https://name:password@private.invalid/mod?token=secret": failed` //nolint:gosec // G101: synthetic credential-redaction fixture.
	got := sanitizeDiagnostic(text)
	for _, secret := range []string{"name", "password", "secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("leaked %s: %s", secret, got)
		}
	}
	if !strings.Contains(got, "private.invalid") || !strings.Contains(got, "failed") {
		t.Fatalf("lost useful diagnostic: %s", got)
	}
}

func TestPrivateProxyFailuresRedactCredentials(t *testing.T) {
	if testing.Short() {
		t.Skip("uses Go against a local TLS proxy")
	}
	fileProxy(t)
	var connections atomic.Int32
	proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fixture proxy unavailable", http.StatusServiceUnavailable)
	}))
	proxy.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	proxy.Config.ErrorLog = log.New(io.Discard, "", 0)
	proxy.StartTLS()
	defer proxy.Close()
	address, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	address.User = url.UserPassword("fixture-user", "fixture-pass")
	address.RawQuery = "token=fixture-token"
	t.Setenv("GOPROXY", address.String())
	t.Setenv("GONOPROXY", "none")
	_, err = buildFixture(t, "//gorn:require test.invalid/private v1.0.0\npackage main\nimport _ \"test.invalid/private\"\nfunc main(){}")
	if err == nil || connections.Load() == 0 {
		t.Fatalf("did not exercise private proxy transport failure: connections=%d err=%v", connections.Load(), err)
	}
	for _, secret := range []string{"fixture-user", "fixture-pass", "fixture-token"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("credential leaked in resolver error: %s", secret)
		}
	}
}

func TestProxyBodyRedactsEveryCredentialURL(t *testing.T) {
	if testing.Short() {
		t.Skip("uses Go against a local proxy")
	}
	fileProxy(t)
	const body = "upstreams=https://first-user:first-secret@a.invalid/a,https://second-user:second-secret@b.invalid/b" //nolint:gosec // G101: synthetic proxy diagnostic credentials.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, body, http.StatusServiceUnavailable)
	}))
	defer proxy.Close()
	t.Setenv("GOPROXY", proxy.URL)
	t.Setenv("GONOPROXY", "none")
	_, err := buildFixture(t, "//gorn:require test.invalid/private v1.0.0\npackage main\nimport _ \"test.invalid/private\"\nfunc main(){}")
	if err == nil || !strings.Contains(err.Error(), "a.invalid") || !strings.Contains(err.Error(), "b.invalid") {
		t.Fatalf("did not exercise compound proxy diagnostic: %v", err)
	}
	for _, secret := range []string{"first-user", "first-secret", "second-user", "second-secret"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("proxy credential leaked: %s", secret)
		}
	}
}
