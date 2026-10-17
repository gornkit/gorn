// Command release builds distribution archives and verifies published installs.
// It is contributor tooling, not a public Gorn command or library.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

type target struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

var targets = []target{{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"}}

type artifact struct {
	target
	Name         string `json:"name"`
	SHA256       string `json:"sha256"`
	BinarySHA256 string `json:"binary_sha256"`
}
type buildInfo struct {
	Preview   bool       `json:"preview"`
	Schema    int        `json:"schema"`
	Version   string     `json:"version"`
	Revision  string     `json:"revision"`
	GoVersion string     `json:"go_version"`
	Artifacts []artifact `json:"artifacts"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("expected build or verify")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	version := flags.String("version", "", "canonical v1 tag")
	dir := flags.String("dir", "", "new output directory (build), downloaded asset directory (verify)")
	preview := flags.Bool("preview", false, "allow untagged/dirty local build; never publish these assets")
	published := flags.Bool("published", false, "also test actual public module installation and helper")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || !validVersion(*version) || *dir == "" {
		return errors.New("require -version canonical v1 tag and -dir, with no positional arguments")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	switch args[0] {
	case "build":
		if *published {
			return errors.New("-published applies only to verify")
		}
		return build(ctx, *version, *dir, *preview)
	case "verify":
		if *preview {
			return errors.New("-preview applies only to build")
		}
		return verify(ctx, *version, *dir, *published)
	default:
		return errors.New("expected build or verify")
	}
}

func validVersion(v string) bool {
	return semver.IsValid(v) && semver.Canonical(v) == v && semver.Major(v) == "v1" && semver.Build(v) == ""
}

func command(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	fmt.Fprintln(os.Stderr, name, strings.Join(args, " "))
	// Executables and arguments are fixed contributor operations; never shell code.
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 G702 -- fixed contributor operations; arguments are never shell code.
	cmd.Dir = dir
	cmd.Env = env
	cmd.WaitDelay = 5 * time.Second
	data, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w\n%s", name, err, data)
	}
	return strings.TrimSpace(string(data)), nil
}

func environment(overrides ...string) []string {
	env := os.Environ()
	for _, v := range append([]string{"GOTOOLCHAIN=local", "GOWORK=off", "GOENV=off", "GOFLAGS= ", "CGO_ENABLED=0", "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT=", "GOFIPS140=off", "GOCACHEPROG="}, overrides...) {
		key := strings.SplitN(v, "=", 2)[0]
		env = slices.DeleteFunc(env, func(s string) bool { k, _, _ := strings.Cut(s, "="); return strings.EqualFold(k, key) })
		env = append(env, v)
	}
	return env
}
func digest(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func binaryName(t target) string {
	if t.OS == "windows" {
		return "gorn.exe"
	}
	return "gorn"
}

func archiveName(version string, t target) string {
	ext := ".tar.gz"
	if t.OS == "windows" {
		ext = ".zip"
	}
	return "gorn_" + version + "_" + t.OS + "_" + t.Arch + ext
}

func build(ctx context.Context, version, dir string, preview bool) error {
	env := environment()
	rev, err := command(ctx, ".", env, "git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if !preview {
		status, e := command(ctx, ".", env, "git", "status", "--porcelain")
		if e != nil {
			return e
		}
		if status != "" {
			return errors.New("release build requires a clean checkout")
		}
		tagged, e := command(ctx, ".", env, "git", "rev-parse", version+"^{commit}")
		if e != nil {
			return e
		}
		if tagged != rev {
			return errors.New("release tag must resolve to HEAD")
		}
	}
	stamp, err := command(ctx, ".", env, "git", "show", "-s", "--format=%ct", "HEAD")
	if err != nil {
		return err
	}
	when, err := time.ParseDuration(stamp + "s")
	if err != nil {
		return err
	}
	timestamp := time.Unix(0, 0).Add(when).UTC()
	gov, err := command(ctx, ".", env, "go", "env", "GOVERSION")
	if err != nil {
		return err
	}
	license, err := os.ReadFile("LICENSE")
	if err != nil {
		return err
	}
	notes, err := os.ReadFile("docs/releases/" + strings.SplitN(version, "-", 2)[0] + ".md") // #nosec G703 -- version is validated canonical semver before build.
	if err != nil {
		return err
	}
	notes, err = releaseNotes(notes)
	if err != nil {
		return err
	}
	if err = os.Mkdir(dir, 0o700); err != nil { // #nosec G703 -- explicit contributor-selected output directory; mkdir refuses existing paths.
		return fmt.Errorf("new output directory: %w", err)
	}
	temp, err := os.MkdirTemp("", "gorn-release-build-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temp) }()
	info := buildInfo{Preview: preview, Schema: 1, Version: version, Revision: rev, GoVersion: gov}
	for _, t := range targets {
		fmt.Fprintln(os.Stderr, "building", t.OS+"/"+t.Arch)
		executable := filepath.Join(temp, binaryName(t))
		_, err = command(ctx, ".", environment("GOOS="+t.OS, "GOARCH="+t.Arch), "go", "build", "-trimpath", "-ldflags=-s -w -X main.Version="+version, "-o", executable, ".")
		if err != nil {
			return err
		}
		data, e := os.ReadFile(executable) // #nosec G304 -- fixed binary name inside private build directory.
		if e != nil {
			return e
		}
		name := archiveName(version, t)
		install := []byte(fmt.Sprintf("Gorn %s (%s/%s)\nPut %s on PATH. Go 1.26.4+ must remain on PATH for run/build.\nCheck SHA256SUMS.txt before extraction. gorn --version verifies the label.\nDocs: https://github.com/gornkit/gorn#readme\n", version, t.OS, t.Arch, binaryName(t)))
		packed, e := pack(t, timestamp, data, license, install)
		if e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(dir, name), packed, 0o644); e != nil { // #nosec G306 G703 -- public assets in the explicitly selected new output directory.
			return e
		}
		info.Artifacts = append(info.Artifacts, artifact{t, name, digest(packed), digest(data)})
	}
	encoded, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "BUILD_INFO.json"), append(encoded, '\n'), 0o644); err != nil { // #nosec G306 G703 -- public assets in the explicitly selected new output directory.
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "RELEASE_NOTES.md"), notes, 0o644); err != nil { // #nosec G306 G703 -- public assets in the explicitly selected new output directory.
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var sums strings.Builder
	for _, entry := range entries {
		data, e := os.ReadFile(filepath.Join(dir, entry.Name())) // #nosec G304 G703 -- own newly created asset directory.
		if e != nil {
			return e
		}
		fmt.Fprintf(&sums, "%s  %s\n", digest(data), entry.Name())
	}
	return os.WriteFile(filepath.Join(dir, "SHA256SUMS.txt"), []byte(sums.String()), 0o644) // #nosec G306 G703 -- public assets in the explicitly selected new output directory.
}

func releaseNotes(data []byte) ([]byte, error) {
	if !strings.HasPrefix(string(data), "---\n") {
		return nil, errors.New("release notes require frontmatter")
	}
	_, body, ok := strings.Cut(string(data[4:]), "\n---\n")
	if !ok {
		return nil, errors.New("unterminated release frontmatter")
	}
	return []byte(strings.TrimSpace(body) + "\n"), nil
}
func nativeTarget() target { return target{runtime.GOOS, runtime.GOARCH} }
