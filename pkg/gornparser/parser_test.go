package gornparser_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gornkit/gorn/pkg/gornparser"
)

func TestParseSourceParsesScriptMetadataAndSections(t *testing.T) {
	source := []byte("#!/usr/bin/env gorn\n" +
		"//gorn:go 1.26\n" +
		"//gorn:module example.com/scripts/todo\n" +
		"//gorn:require github.com/charmbracelet/lipgloss v1.1.0\n" +
		"//gorn:require github.com/alexflint/go-arg v1.6.1\n" +
		"\n" +
		"import (\n" +
		"\t\"fmt\"\n" +
		")\n" +
		"\n" +
		"func hasTodo(path string) bool {\n" +
		"\treturn true\n" +
		"}\n" +
		"\n" +
		"//gorn:main\n" +
		"\n" +
		"fmt.Println(hasTodo(\"README.md\"))\n")

	script, err := gornparser.ParseSource("todo.gorn", source)
	if err != nil {
		t.Fatal(err)
	}

	if script.SourcePath != "todo.gorn" {
		t.Fatalf("Path = %q, want %q", script.SourcePath, "todo.gorn")
	}
	if script.GoVersion != "1.26" {
		t.Fatalf("GoVersion = %q, want 1.26", script.GoVersion)
	}
	if script.Module != "example.com/scripts/todo" {
		t.Fatalf("Module = %q, want example.com/scripts/todo", script.Module)
	}

	wantRequires := []gornparser.Require{
		{Path: "github.com/charmbracelet/lipgloss", Version: "v1.1.0"},
		{Path: "github.com/alexflint/go-arg", Version: "v1.6.1"},
	}
	if len(script.Requires) != len(wantRequires) {
		t.Fatalf("Requires = %#v, want %#v", script.Requires, wantRequires)
	}
	for i := range wantRequires {
		if script.Requires[i] != wantRequires[i] {
			t.Fatalf("Requires[%d] = %#v, want %#v", i, script.Requires[i], wantRequires[i])
		}
	}

	wantPackage := "import (\n" +
		"\t\"fmt\"\n" +
		")\n" +
		"\n" +
		"func hasTodo(path string) bool {\n" +
		"\treturn true\n" +
		"}\n" +
		"\n"
	if got := script.PackageContent; got != wantPackage {
		t.Fatalf("PackageLines joined = %q, want %q", got, wantPackage)
	}

	wantMain := "fmt.Println(hasTodo(\"README.md\"))\n"
	if got := script.MainContent; got != wantMain {
		t.Fatalf("MainLines joined = %q, want %q", got, wantMain)
	}
}

func TestParseSourceTracksSectionStartLines(t *testing.T) {
	source := []byte("#!/usr/bin/env gorn\n" +
		"//gorn:go 1.26\n" +
		"\n" +
		"import \"fmt\"\n" +
		"//gorn:main\n" +
		"fmt.Println(\"hello\")\n")

	script, err := gornparser.ParseSource("hello.gorn", source)
	if err != nil {
		t.Fatal(err)
	}
	if script.PackageStart == nil || *script.PackageStart != 4 {
		t.Fatalf("PackageStart = %v, want 4", intPtrValue(script.PackageStart))
	}
	if script.MainStart != 6 {
		t.Fatalf("MainStart = %d, want 6", script.MainStart)
	}
}

// TestParseSourceDropsLeadingBlanksAfterMain locks in that blank lines
// immediately after //gorn:main are dropped rather than preserved, keeping
// MainStart aligned with the first main line. Gorn never needs to
// reverse-generate the original source, so this formatting trivia is
// intentionally lost.
func TestParseSourceDropsLeadingBlanksAfterMain(t *testing.T) {
	source := []byte("//gorn:main\n" +
		"\n" +
		"\n" +
		"fmt.Println(\"hello\")\n")

	script, err := gornparser.ParseSource("blank-main.gorn", source)
	if err != nil {
		t.Fatal(err)
	}
	if script.MainStart != 4 {
		t.Fatalf("MainStart = %d, want 4", script.MainStart)
	}
	if got := script.MainContent; got != "fmt.Println(\"hello\")\n" {
		t.Fatalf("MainLines joined = %q, want %q", got, "fmt.Println(\"hello\")\n")
	}
}

func TestParseSourceAllowsShebangOnlyOnFirstLine(t *testing.T) {
	source := []byte("//gorn:main\n" +
		"#! this is main source, not a shebang\n")

	script, err := gornparser.ParseSource("shebang.gorn", source)
	if err != nil {
		t.Fatal(err)
	}

	want := "#! this is main source, not a shebang\n"
	if got := script.MainContent; got != want {
		t.Fatalf("MainLines joined = %q, want %q", got, want)
	}
}

func TestParseSourceRejectsInvalidScripts(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		wantErr  error
		wantLine int
	}{
		{
			name:    "empty script",
			source:  "",
			wantErr: gornparser.ErrEmptyScript,
		},
		{
			name:     "missing main",
			source:   "import \"fmt\"\n",
			wantErr:  gornparser.ErrMissingMain,
			wantLine: 1,
		},
		{
			name: "multiple main directives",
			source: "//gorn:main\n" +
				"println(\"first\")\n" +
				"//gorn:main\n",
			wantErr:  gornparser.ErrMultipleMain,
			wantLine: 3,
		},
		{
			name: "directive after main",
			source: "//gorn:main\n" +
				"//gorn:go 1.26\n",
			wantErr:  gornparser.ErrDirectiveAfterMain,
			wantLine: 2,
		},
		{
			name: "unknown directive",
			source: "//gorn:unknown value\n" +
				"//gorn:main\n",
			wantErr:  gornparser.ErrInvalidDirective,
			wantLine: 1,
		},
		{
			name: "require missing version",
			source: "//gorn:require github.com/example/tool\n" +
				"//gorn:main\n",
			wantErr:  gornparser.ErrInvalidRequire,
			wantLine: 1,
		},
		{
			name: "require rejects latest",
			source: "//gorn:require github.com/example/tool latest\n" +
				"//gorn:main\n",
			wantErr:  gornparser.ErrInvalidRequire,
			wantLine: 1,
		},
		{
			name: "duplicate go directive",
			source: "//gorn:go 1.26\n" +
				"//gorn:go 1.27\n" +
				"//gorn:main\n",
			wantErr:  gornparser.ErrDuplicateGo,
			wantLine: 2,
		},
		{
			name: "duplicate module directive",
			source: "//gorn:module example.com/scripts/todo\n" +
				"//gorn:module example.com/scripts/other\n" +
				"//gorn:main\n",
			wantErr:  gornparser.ErrDuplicateModule,
			wantLine: 2,
		},
		{
			name: "directive after real package content",
			source: "import \"fmt\"\n" +
				"//gorn:require github.com/example/tool v1.0.0\n" +
				"//gorn:main\n",
			wantErr:  gornparser.ErrDirectiveAfterPackage,
			wantLine: 2,
		},
		{
			name:     "main directive is the last line",
			source:   "//gorn:main\n",
			wantErr:  gornparser.ErrEmptyMain,
			wantLine: 1,
		},
		{
			name:     "main section is only blank lines",
			source:   "//gorn:main\n\n\n",
			wantErr:  gornparser.ErrEmptyMain,
			wantLine: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gornparser.ParseSource("bad.gorn", []byte(tt.source))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want errors.Is(_, %v)", err, tt.wantErr)
			}

			if tt.wantLine == 0 {
				return
			}

			var parseErr *gornparser.Error
			if !errors.As(err, &parseErr) {
				t.Fatalf("error type = %T, want *gornparser.Error", err)
			}
			if parseErr.Line != tt.wantLine {
				t.Fatalf("error line = %d, want %d", parseErr.Line, tt.wantLine)
			}
		})
	}
}

// TestParseSourceAllowsMultipleRequireDirectives guards against ever
// conflating //gorn:require (intentionally repeatable) with //gorn:go and
// //gorn:module (which must appear at most once).
func TestParseSourceAllowsMultipleRequireDirectives(t *testing.T) {
	source := []byte("//gorn:require github.com/charmbracelet/lipgloss v1.1.0\n" +
		"//gorn:require github.com/alexflint/go-arg v1.6.1\n" +
		"//gorn:main\n" +
		"println(\"hello\")\n")

	script, err := gornparser.ParseSource("multi-require.gorn", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(script.Requires) != 2 {
		t.Fatalf("Requires = %#v, want 2 entries", script.Requires)
	}
}

// TestParseSourceAllowsBlankLinesBetweenDirectives locks in that blank lines
// used as directive-block spacing don't count as package-section content,
// so they don't block a following directive. Once real content appears,
// the zone still closes permanently: a directive after that must still be
// rejected, even if more blank lines follow.
func TestParseSourceParsesPreambleDirective(t *testing.T) {
	source := []byte("//gorn:preamble\n" +
		"//gorn:main\n" +
		"println(\"hi\")\n")

	script, err := gornparser.ParseSource("preamble.gorn", source)
	if err != nil {
		t.Fatal(err)
	}
	if !script.UsePreamble {
		t.Fatal("UsePreamble = false, want true")
	}
}

func TestParseSourceRejectsPreambleWithArgs(t *testing.T) {
	source := []byte("//gorn:preamble extra\n" +
		"//gorn:main\n" +
		"println(\"hi\")\n")

	_, err := gornparser.ParseSource("preamble.gorn", source)
	if !errors.Is(err, gornparser.ErrInvalidDirective) {
		t.Fatalf("error = %v, want errors.Is(_, %v)", err, gornparser.ErrInvalidDirective)
	}
}

func TestParseSourceAllowsBlankLinesBetweenDirectives(t *testing.T) {
	source := []byte("\n" +
		"//gorn:go 1.26\n" +
		"\n" +
		"//gorn:module example.com/scripts/todo\n" +
		"\n" +
		"import \"fmt\"\n" +
		"//gorn:main\n" +
		"fmt.Println()\n")

	script, err := gornparser.ParseSource("blank-spacing.gorn", source)
	if err != nil {
		t.Fatal(err)
	}
	if script.GoVersion != "1.26" {
		t.Fatalf("GoVersion = %q, want 1.26", script.GoVersion)
	}
	if script.Module != "example.com/scripts/todo" {
		t.Fatalf("Module = %q, want example.com/scripts/todo", script.Module)
	}
	if script.PackageStart == nil || *script.PackageStart != 6 {
		t.Fatalf("PackageStart = %v, want 6", intPtrValue(script.PackageStart))
	}
	if got := script.PackageContent; got != "import \"fmt\"\n" {
		t.Fatalf("PackageLines joined = %q, want %q", got, "import \"fmt\"\n")
	}
}

func TestParseSourceStillRejectsDirectiveAfterRealContentDespiteBlanks(t *testing.T) {
	source := []byte("import \"fmt\"\n" +
		"\n" +
		"//gorn:require github.com/example/tool v1.0.0\n" +
		"//gorn:main\n")

	_, err := gornparser.ParseSource("bad.gorn", source)
	if !errors.Is(err, gornparser.ErrDirectiveAfterPackage) {
		t.Fatalf("error = %v, want errors.Is(_, %v)", err, gornparser.ErrDirectiveAfterPackage)
	}
}

func TestParseSourceRequiresExactDirectiveNames(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "mainly is not main",
			source: "//gorn:mainly\n" +
				"println(\"hello\")\n",
		},
		{
			name: "gopher is not go",
			source: "//gorn:gopher 1.26\n" +
				"//gorn:main\n",
		},
		{
			name: "requirement is not require",
			source: "//gorn:requirement github.com/example/tool v1.0.0\n" +
				"//gorn:main\n",
		},
		{
			name: "modules is not module",
			source: "//gorn:modules example.com/script\n" +
				"//gorn:main\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gornparser.ParseSource("bad.gorn", []byte(tt.source))
			if !errors.Is(err, gornparser.ErrInvalidDirective) {
				t.Fatalf("error = %v, want errors.Is(_, %v)", err, gornparser.ErrInvalidDirective)
			}
		})
	}
}

func TestParseFileReadsPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "script.gorn")
	source := []byte("//gorn:main\nprintln(\"from file\")\n")
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}

	script, err := gornparser.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if script.SourcePath != path {
		t.Fatalf("Path = %q, want %q", script.SourcePath, path)
	}
}

func TestParseFileReportsReadErrors(t *testing.T) {
	_, err := gornparser.ParseFile(filepath.Join(t.TempDir(), "missing.gorn"))
	if !errors.Is(err, gornparser.ErrFailedToReadFile) {
		t.Fatalf("error = %v, want errors.Is(_, %v)", err, gornparser.ErrFailedToReadFile)
	}
}

func intPtrValue(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
