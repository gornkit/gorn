//nolint:gosec // Controlled local fixtures, intentional aliases, executable modes, and output publication.
package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestExecutableExportPolicy(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "replace"}[force], func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "cache-bin")
			dest := filepath.Join(dir, "out.exe")
			if err := os.WriteFile(input, []byte("complete binary"), 0o700); err != nil {
				t.Fatal(err)
			}
			if force {
				if err := os.WriteFile(dest, []byte("old"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := exportExecutable(context.Background(), input, dest, "", force); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(dest)
			if err != nil || string(data) != "complete binary" {
				t.Fatal(string(data), err)
			}
			if runtime.GOOS != "windows" {
				info, err := os.Stat(dest)
				if err != nil || info.Mode().Perm() != 0o755 {
					t.Fatal("executable permissions:", info, err)
				}
			}
			staged, err := filepath.Glob(filepath.Join(dir, ".gorn-export-*"))
			if err != nil || len(staged) != 0 {
				t.Fatalf("staged files leaked: %v %v", staged, err)
			}
		})
	}
}

func TestExportFailuresPreserveDestination(t *testing.T) {
	for _, kind := range []string{"already exists", "canceled", "missing input", "directory", "source", "hardlink source", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "cache-bin")
			dest := filepath.Join(dir, "out.exe")
			source := filepath.Join(dir, "source.gorn")
			for path, text := range map[string]string{input: "new", dest: "old", source: "source"} {
				if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			force := true
			ctx := context.Background()
			scriptPath := ""
			switch kind {
			case "already exists":
				force = false
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "missing input":
				input += "-missing"
			case "directory":
				if err := os.Remove(dest); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(dest, 0o700); err != nil {
					t.Fatal(err)
				}
			case "source":
				scriptPath = source
				dest = source
			case "hardlink source":
				scriptPath = source
				if err := os.Remove(dest); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(source, dest); err != nil {
					t.Skip(err)
				}
			case "symlink":
				if err := os.Remove(dest); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(source, dest); err != nil {
					t.Skip(err)
				}
			}
			err := exportExecutable(ctx, input, dest, scriptPath, force)
			if err == nil {
				t.Fatal("unsafe/failed export succeeded")
			}
			if kind == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if kind != "directory" {
				data, err := os.ReadFile(dest)
				want := "old"
				if kind == "source" || kind == "hardlink source" || kind == "symlink" {
					want = "source"
				}
				if err != nil || string(data) != want {
					t.Fatalf("destination changed: %q %v", data, err)
				}
			}
			stages, _ := filepath.Glob(filepath.Join(dir, ".gorn-export-*"))
			if len(stages) != 0 {
				t.Fatal("failed export left staging:", stages)
			}
		})
	}
}

func TestConcurrentExportsDoNotClobber(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.exe")
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, text := range []string{"one", "two"} {
		input := filepath.Join(dir, text)
		if err := os.WriteFile(input, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		wg.Go(func() { <-start; results <- exportExecutable(context.Background(), input, dest, "", false) })
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful publications=%d", successes)
	}
	data, err := os.ReadFile(dest)
	if err != nil || (string(data) != "one" && string(data) != "two") {
		t.Fatal("partial destination:", string(data), err)
	}
}

func TestOutputPathNamingAndInvalidParents(t *testing.T) {
	dir := t.TempDir()
	path, err := outputPath(filepath.Join(dir, "tool"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "tool")
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	if path != want {
		t.Fatal(path, want)
	}
	explicit := filepath.Join(dir, "tool.custom")
	if path, err := outputPath(explicit); err != nil || path != explicit {
		t.Fatal(path, err)
	}
	for _, path := range []string{"", dir, filepath.Join(dir, "absent", "out")} {
		if _, err := outputPath(path); err == nil {
			t.Fatalf("accepted invalid output %q", path)
		}
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(dir, "target"), link); err == nil {
		if _, err := outputPath(link); err == nil {
			t.Fatal("renamed requested symlink with .exe suffix")
		}
	}
}

func TestConcurrentPublicationDoesNotReplaceWinner(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "published")
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"first", "second"} {
		staged := filepath.Join(dir, name)
		if err := os.WriteFile(staged, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		wg.Go(func() { <-start; results <- publishExecutable(staged, destination, false) })
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("publication successes=%d", successes)
	}
	data, err := os.ReadFile(destination)
	if err != nil || (string(data) != "first" && string(data) != "second") {
		t.Fatal(string(data), err)
	}
}

func TestFailedNativePublicationPreservesIncumbent(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "existing")
	if err := os.WriteFile(destination, []byte("incumbent"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := publishExecutable(filepath.Join(dir, "missing-candidate"), destination, true); err == nil {
		t.Fatal("missing candidate replaced incumbent")
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "incumbent" {
		t.Fatal("publication failure changed incumbent:", string(data), err)
	}
}
