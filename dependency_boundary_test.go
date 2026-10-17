package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

type graphPackage struct {
	ImportPath string
	Standard   bool
	Imports    []string
}

func packageGraph(t *testing.T, paths ...string) []graphPackage {
	t.Helper()
	args := append([]string{"list", "-deps", "-json"}, paths...)
	cmd := exec.Command("go", args...) //nolint:gosec // Fixed repository package patterns, no user-selected commands.
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOENV=off", "GOTOOLCHAIN=local", "GOFLAGS=")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("dependency graph: %v\n%s", err, stderr.String())
	}
	decoder := json.NewDecoder(&stdout)
	var packages []graphPackage
	for {
		var pkg graphPackage
		if err := decoder.Decode(&pkg); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		packages = append(packages, pkg)
	}
	if len(packages) == 0 {
		t.Fatal("empty dependency graph")
	}
	return packages
}

func TestDependencyBoundaries(t *testing.T) {
	const helper = "github.com/gornkit/gorn/sh"
	for _, pkg := range packageGraph(t, "./sh") {
		if !pkg.Standard && pkg.ImportPath != helper {
			t.Errorf("sh dependency %s is not stdlib or self", pkg.ImportPath)
		}
	}
	for _, pkg := range packageGraph(t, ".", "./internal/...") {
		if pkg.ImportPath == helper || strings.HasPrefix(pkg.ImportPath, helper+"/") {
			t.Errorf("CLI graph imports helper %s", pkg.ImportPath)
		}
		for _, path := range pkg.Imports {
			if path == helper || strings.HasPrefix(path, helper+"/") {
				t.Errorf("%s imports %s; CLI/helper independence violated", pkg.ImportPath, path)
			}
		}
	}
}
