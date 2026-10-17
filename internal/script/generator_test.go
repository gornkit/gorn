package script_test

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/gornkit/gorn/internal/app"
	"github.com/gornkit/gorn/internal/script"
)

func TestGeneratePreservesSourceAndModule(t *testing.T) {
	source := "//gorn:go 1.26\n//gorn:module example.com/task\n//gorn:require example.com/tool v1.2.3\npackage main\nfunc main( ) {  println(`first\n  second\n\nthird`) }"
	f, err := parseSource("task.gorn", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	g, err := script.Generate(f)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(g.MainGenerated, []byte(source)) {
		t.Fatalf("source changed:\n%s", g.MainGenerated)
	}
	want := "module example.com/task\n\ngo 1.26\nrequire (\n\texample.com/tool v1.2.3\n)\n"
	if string(g.ModGenerated) != want {
		t.Fatalf("module=%q, want %q", g.ModGenerated, want)
	}
	f, err = parseSource("default.gorn", []byte("package main;func main(){}"))
	if err != nil {
		t.Fatal(err)
	}
	g, err = script.Generate(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(g.ModGenerated) != "module gorn.local/app\n\ngo 1.26\n" {
		t.Fatalf("defaults: %q", g.ModGenerated)
	}
}

func TestGenerateRejectsPreambleImportConflicts(t *testing.T) {
	for _, imp := range []string{`"fmt"`, `f "fmt"`, `_ "strings"`, `. "time"`} {
		source := "//gorn:preamble\npackage main\nimport " + imp + "\nfunc main(){}"
		f, err := parseSource("conflict.gorn", []byte(source))
		if err != nil {
			t.Fatal(err)
		}
		_, err = script.Generate(f)
		var lineErr *script.LineError
		var genErr *script.GenerateError
		if !errors.Is(err, script.ErrPreambleImportConflict) || !errors.As(err, &lineErr) || lineErr.Line != 3 || !errors.As(err, &genErr) || genErr.Raw != nil {
			t.Fatalf("import %s: %v", imp, err)
		}
	}
}

func TestGeneratePreamblePreservesAttachedComments(t *testing.T) {
	source := "//gorn:preamble\npackage main\n\n/* cgo preamble */\nimport \"C\"\n//go:noinline\n// helper docs\nfunc helper(){}\nfunc main(){}"
	f, err := parseSource("comments.gorn", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	g, err := script.Generate(f)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), "generated.go", g.MainGenerated, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range parsed.Decls {
		if imp, ok := decl.(*ast.GenDecl); ok && imp.Tok == token.IMPORT {
			for _, spec := range imp.Specs {
				if spec.(*ast.ImportSpec).Path.Value == `"C"` && (imp.Doc == nil || !strings.Contains(imp.Doc.Text(), "cgo preamble")) {
					t.Fatal("lost cgo comment attachment")
				}
			}
		}
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "helper" {
			if fn.Doc == nil || len(fn.Doc.List) != 2 || fn.Doc.List[0].Text != "//go:noinline" {
				t.Fatal("lost helper directive attachment")
			}
		}
	}
}

func TestGeneratedPrograms(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles generated programs")
	}
	literal := "first\n  second\n\n//gorn:main\n//gorn:go 1.99\nlast"
	for _, tt := range []struct{ name, source, want string }{
		{"raw", "package main\nimport \"fmt\"\nfunc main( ){fmt.Printf(\"%q\", `" + literal + "`)}", strconv.Quote(literal)},
		{"preamble raw", "//gorn:preamble\npackage main\nfunc main( ){fmt.Printf(\"%q\", `" + literal + "`)}", strconv.Quote(literal)},
		{"preamble unused", "//gorn:preamble\npackage main\nfunc main(){}", ""},
		{"compact", "//gorn:preamble\npackage main;func main(){fmt.Print(\"compact\")}", "compact"},
		{"package comment", "//gorn:preamble\npackage main /* package\ncomment */\n//go:noinline\nfunc helper(){fmt.Print(\"ok\")}\nfunc main(){helper()}", "ok"},
		{"extra imports", "//gorn:preamble\npackage main\nimport \"bytes\"\nfunc main(){var b bytes.Buffer;b.WriteString(\"ok\");fmt.Print(b.String())}", "ok"},
		{"shebang crlf", "#!arbitrary\r\n//gorn:preamble\r\npackage main\r\nfunc main(){fmt.Print(\"ok\")}\r\n", "ok"},
		{"bom", "\ufeffpackage main\nfunc main(){}", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir, g := generatedModule(t, tt.source)
			writeFile(t, filepath.Join(dir, "main.gorn.go"), string(g.MainGenerated))
			runGo(t, dir, "mod", "tidy")
			runGo(t, dir, "build", "-o", "program"+exeSuffix(), ".")
			out, err := exec.Command(filepath.Join(dir, "program"+exeSuffix())).CombinedOutput() //nolint:gosec // G204: executable built from a controlled fixture in t.TempDir.
			if err != nil || string(out) != tt.want {
				t.Fatalf("run: %v output=%q, want %q", err, out, tt.want)
			}
			if tt.name == "raw" {
				writeFile(t, filepath.Join(dir, "reference.go"), tt.source)
				runGo(t, dir, "build", "-o", "reference"+exeSuffix(), "reference.go")
				ref, err := exec.Command(filepath.Join(dir, "reference"+exeSuffix())).CombinedOutput() //nolint:gosec // G204: controlled reference executable in t.TempDir.
				if err != nil || !bytes.Equal(out, ref) {
					t.Fatalf("reference: %v %q != %q", err, ref, out)
				}
			}
		})
	}
}

func TestGeneratedCompilerDiagnostics(t *testing.T) {
	if testing.Short() {
		t.Skip("invokes compiler")
	}
	for name, source := range map[string]string{
		"plain":                      "package main\nfunc main(){helper()}\nfunc helper(){ missing() }\n",
		"preamble":                   "//gorn:preamble\npackage main\nfunc main(){helper()}\nfunc helper(){ missing() }\n",
		"compact":                    "//gorn:preamble\npackage main;func main(){missing()}",
		"trailing comment":           "//gorn:preamble\npackage main//line other.go:99\nfunc main(){missing()}",
		"explicit semicolon comment": "//gorn:preamble\npackage main;//line other.go:99\nfunc main(){missing()}",
		"package comment":            "//gorn:preamble\npackage main /* package\ncomment */func main(){missing()}",
	} {
		t.Run(name, func(t *testing.T) {
			dir, g := generatedModule(t, source)
			writeFile(t, filepath.Join(dir, "main.gorn.go"), string(g.MainGenerated))
			runGo(t, dir, "mod", "tidy")
			cmd := exec.Command("go", "build", ".")
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatal("build succeeded")
			}
			offset := strings.Index(source, "missing()")
			line := strings.Count(source[:offset], "\n") + 1
			column := offset - strings.LastIndex(source[:offset], "\n")
			// The Go command may shorten absolute filenames for display.
			want := fmt.Sprintf("original source.gorn:%d:%d", line, column)
			if !bytes.Contains(g.MainGenerated, []byte("//line "+filepath.Join(dir, "original source.gorn")+":")) {
				t.Fatal("generated source anchor must use an absolute path")
			}
			if !strings.Contains(string(out), want) {
				t.Fatalf("diagnostic missing %q:\n%s", want, out)
			}
		})
	}
}

func TestGenerateRetainsExplicitSourceMapping(t *testing.T) {
	if testing.Short() {
		t.Skip("invokes compiler")
	}
	source := "//gorn:preamble\n//line custom.go:40\npackage main\nfunc main(){missing()}"
	dir, g := generatedModule(t, source)
	writeFile(t, filepath.Join(dir, "main.gorn.go"), string(g.MainGenerated))
	runGo(t, dir, "mod", "tidy")
	cmd := exec.Command("go", "build", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "custom.go:41: undefined: missing") {
		t.Fatalf("explicit source mapping lost: %v\n%s", err, out)
	}
}

func TestFormatSpecificationExamplesCompile(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles documentation examples")
	}
	for _, path := range []string{"../../docs/specs/gorn-script-format.md", "../../README.md"} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			doc, err := os.ReadFile(path) //nolint:gosec // G304: fixed authoritative documentation paths, not caller input.
			if err != nil {
				t.Fatal(err)
			}
			examples := regexp.MustCompile("(?s)```go\\n(.*?)\\n```").FindAllSubmatch(doc, -1)
			if len(examples) == 0 {
				t.Fatal("no Go examples found")
			}
			for i, example := range examples {
				t.Run(strconv.Itoa(i+1), func(t *testing.T) {
					dir, g := generatedModule(t, string(example[1]))
					writeFile(t, filepath.Join(dir, "main.gorn.go"), string(g.MainGenerated))
					runGo(t, dir, "mod", "tidy")
					runGo(t, dir, "build", "-o", "program"+exeSuffix(), ".")
				})
			}
		})
	}
}

func generatedModule(t *testing.T, source string) (string, *script.Generated) {
	t.Helper()
	dir := t.TempDir()
	// Preserve ordinary Go VCS stamping while isolating compiler-oracle builds
	// from unrelated/invalid ancestor .git markers on a developer's temp root.
	git := exec.Command("git", "-c", "init.defaultBranch=gorn-fixture", "init", "--quiet", "--template=", dir) //nolint:gosec // G204: private t.TempDir fixture, with no repository hooks/templates.
	if out, err := git.CombinedOutput(); err != nil {
		t.Fatalf("initialize private Git compiler fixture: %v\n%s", err, out)
	}
	path := filepath.Join(dir, "original source.gorn")
	writeFile(t, path, source)
	s, err := app.New(path, []byte(source))
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
	mod := string(g.ModGenerated)
	if g.BundleHelper {
		bundle, err := script.LoadHelper(os.DirFS("../.."))
		if err != nil {
			t.Fatal(err)
		}
		if err := bundle.WriteTo(filepath.Join(dir, script.HelperDir)); err != nil {
			t.Fatal(err)
		}
	}

	writeFile(t, filepath.Join(dir, "go.mod"), mod)
	return dir, g
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil { //nolint:gosec // G703: fixture paths are controlled by the tests.
		t.Fatal(err)
	}
}

func runGo(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("go", args...) //nolint:gosec // G204: arguments are controlled by the tests.
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go %v: %v\n%s", args, err, out)
	}
}
