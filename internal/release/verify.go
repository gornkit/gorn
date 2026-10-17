package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func verify(ctx context.Context, version, dir string, published bool) error {
	sumsData, err := os.ReadFile(filepath.Join(dir, "SHA256SUMS.txt")) // #nosec G304 G703 -- explicitly selected downloaded release directory.
	if err != nil {
		return err
	}
	sums, err := parseSums(string(sumsData))
	if err != nil {
		return err
	}
	readVerified := func(name string) ([]byte, error) {
		data, e := os.ReadFile(filepath.Join(dir, name)) // #nosec G304 G703 -- fixed asset names checked against release manifest.
		if e != nil {
			return nil, e
		}
		if sums[name] != digest(data) {
			return nil, fmt.Errorf("checksum mismatch: %s", name)
		}
		return data, nil
	}
	encoded, err := readVerified("BUILD_INFO.json")
	if err != nil {
		return err
	}
	var info buildInfo
	if err = json.Unmarshal(encoded, &info); err != nil {
		return err
	}
	if info.Schema != 1 || info.Version != version || len(info.Artifacts) != len(targets) || len(info.Revision) != 40 {
		return errors.New("invalid release build information")
	}
	if _, err = hex.DecodeString(info.Revision); err != nil {
		return err
	}
	if _, err = readVerified("RELEASE_NOTES.md"); err != nil {
		return err
	}
	if published {
		if info.Preview {
			return errors.New("preview assets cannot satisfy published-release verification")
		}
		expected, e := command(ctx, ".", environment(), "git", "rev-parse", "HEAD")
		if e != nil {
			return e
		}
		if info.Revision != expected {
			return errors.New("downloaded release revision differs from checked-out tag")
		}
	}
	var native *artifact
	for i, t := range targets {
		a := info.Artifacts[i]
		if a.target != t || a.Name != archiveName(version, t) || !validDigest(a.SHA256) || a.SHA256 != sums[a.Name] || !validDigest(a.BinarySHA256) {
			return errors.New("invalid target manifest or checksums")
		}
		if t == nativeTarget() {
			native = &a
		}
	}
	if native == nil {
		return fmt.Errorf("no release target for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	data, err := readVerified(native.Name)
	if err != nil {
		return err
	}
	files, err := unpack(native.target, data)
	if err != nil {
		return err
	}
	if digest(files[0].data) != native.BinarySHA256 {
		return errors.New("executable checksum mismatch")
	}
	temp, err := os.MkdirTemp("", "gorn-release-verify-")
	if err != nil {
		return err
	}
	defer cleanup(temp)
	executable := filepath.Join(temp, files[0].name)
	if err = os.WriteFile(executable, files[0].data, 0o755); err != nil { // #nosec G306 -- verified executable in private verification directory.
		return err
	}
	env := isolatedEnvironment(temp)
	if err = smoke(ctx, temp, executable, version, env); err != nil {
		return fmt.Errorf("downloaded archive smoke: %w", err)
	}
	if published {
		gobin := filepath.Join(temp, "installed")
		if err = os.Mkdir(gobin, 0o700); err != nil {
			return err
		}
		env = append(env, "GOBIN="+gobin)
		if _, err = command(ctx, temp, env, "go", "install", "github.com/gornkit/gorn@"+version); err != nil {
			return err
		}
		installed := filepath.Join(gobin, binaryName(nativeTarget()))
		if err = smoke(ctx, temp, installed, version, env); err != nil {
			return fmt.Errorf("published module smoke: %w", err)
		}
		if err = publishedHelper(ctx, temp, installed, version, env); err != nil {
			return err
		}
	}
	fmt.Fprintln(os.Stderr, "verified", version, native.OS, native.Arch, "published module:", published)
	return nil
}

func parseSums(s string) (map[string]string, error) {
	sums := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(s), "\n") {
		hash, name, ok := strings.Cut(line, "  ")
		if !ok || !validDigest(hash) || name == "" || filepath.Base(name) != name || strings.ContainsAny(name, "/\\") || sums[name] != "" {
			return nil, errors.New("invalid checksum list")
		}
		sums[name] = hash
	}
	if len(sums) != len(targets)+2 {
		return nil, errors.New("unexpected checksum membership")
	}
	return sums, nil
}

func validDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil && strings.ToLower(s) == s
}

func isolatedEnvironment(temp string) []string {
	// Normal public Go resolution and checksum validation, with no local replace.
	// All mutable caches belong to this disposable verification directory.
	return environment("GOCACHE="+filepath.Join(temp, "go-build"), "GOMODCACHE="+filepath.Join(temp, "go-mod"), "GOPATH="+filepath.Join(temp, "gopath"), "GOPROXY=https://proxy.golang.org,direct", "GOSUMDB=sum.golang.org", "GOPRIVATE=", "GONOSUMDB=", "GONOPROXY=", "GOINSECURE=", "GOAUTH=off", "GORN_CACHE="+filepath.Join(temp, "gorn-cache"), "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)
}

func smoke(ctx context.Context, temp, executable, version string, env []string) error {
	got, err := command(ctx, temp, env, executable, "--version")
	if err != nil {
		return err
	}
	if got != "gorn "+version {
		return fmt.Errorf("unexpected version %q", got)
	}
	source := filepath.Join(temp, "hello.gorn")
	if err = os.WriteFile(source, []byte("package main\nimport (\"fmt\"; \"os\")\nfunc main(){fmt.Println(\"hello,\",os.Args[1])}\n"), 0o600); err != nil {
		return err
	}
	got, err = command(ctx, temp, env, executable, "-q", "run", source, "--", "two words")
	if err != nil {
		return err
	}
	if got != "hello, two words" {
		return fmt.Errorf("unexpected script output %q", got)
	}
	exported := filepath.Join(temp, "hello")
	if runtime.GOOS == "windows" {
		exported += ".exe"
	}
	if _, err = command(ctx, temp, env, executable, "-q", "build", source, "-o", exported, "--force"); err != nil {
		return err
	}
	emptyPath := filepath.Join(temp, "empty-path")
	if err = os.MkdirAll(emptyPath, 0o700); err != nil {
		return err
	}
	noGo := append([]string{}, env...)
	for i, s := range noGo {
		if k, _, _ := strings.Cut(s, "="); strings.EqualFold(k, "PATH") {
			noGo[i] = "PATH=" + emptyPath
		}
	}
	got, err = command(ctx, temp, noGo, exported, "exported")
	if err != nil {
		return err
	}
	if got != "hello, exported" {
		return fmt.Errorf("unexpected export output %q", got)
	}
	return nil
}

func publishedHelper(ctx context.Context, temp, executable, version string, env []string) error {
	source := filepath.Join(temp, "published-helper.gorn")
	script := fmt.Sprintf(`//gorn:go 1.26.4
//gorn:require github.com/gornkit/gorn %s
//gorn:preamble
package main
import "runtime/debug"
func main() {
 info, ok := debug.ReadBuildInfo()
 if !ok { panic("missing build information") }
 found := false
 for _, dep := range info.Deps {
  if dep.Path == "github.com/gornkit/gorn/sh" { panic("bundled helper selected") }
  if dep.Path == "github.com/gornkit/gorn" {
   if dep.Version != %q || dep.Replace != nil { panic("wrong published helper selection") }
   found = true
  }
 }
 if !found { panic("published helper absent from compiled graph") }
 fmt.Println("published-helper", %q)
 fmt.Println(sh.PreambleSentinel)
 sh.Cmd("go", "version").Exec().OrExit()
}
`, version, version, version)
	if err := os.WriteFile(source, []byte(script), 0o600); err != nil { // #nosec G703 -- fixed script filename inside private MkdirTemp.
		return err
	}
	mod, err := command(ctx, temp, env, executable, "run", "--print-mod", source)
	if err != nil {
		return err
	}
	if !strings.Contains(mod, "github.com/gornkit/gorn "+version) || strings.Contains(mod, "replace") || strings.Contains(mod, "github.com/gornkit/gorn/sh v") {
		return fmt.Errorf("expected exact published parent helper, got:\n%s", mod)
	}
	got, err := command(ctx, temp, env, executable, "-q", "run", source)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(got, "published-helper "+version+"\ntrue\ngo version go") {
		return fmt.Errorf("unexpected helper output %q", got)
	}
	return nil
}

func cleanup(temp string) {
	// Go's downloaded module files are read-only. Change only our private tree,
	// never symlink targets or user caches, then retire it on all supported hosts.
	_ = filepath.WalkDir(temp, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.Type()&os.ModeSymlink == 0 {
			if entry.IsDir() {
				_ = os.Chmod(path, 0o700) // #nosec G302 G122 -- private temporary tree; no symlink traversal or external writers.
			} else {
				_ = os.Chmod(path, 0o600) // #nosec G122 -- private verification tree, no symlink traversal or external writers.
			}
		}
		return nil
	})
	_ = os.RemoveAll(temp)
}
