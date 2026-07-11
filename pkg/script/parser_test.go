package script_test

import (
	"errors"
	"testing"

	"github.com/gornkit/gorn/pkg/script"
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

	file, err := parseSource("todo.gorn", source)
	if err != nil {
		t.Fatal(err)
	}

	if file.SourcePath != "todo.gorn" {
		t.Fatalf("Path = %q, want %q", file.SourcePath, "todo.gorn")
	}
	if file.GoVersion != "1.26" {
		t.Fatalf("GoVersion = %q, want 1.26", file.GoVersion)
	}
	if file.Module != "example.com/scripts/todo" {
		t.Fatalf("Module = %q, want example.com/scripts/todo", file.Module)
	}

	wantRequires := []script.Require{
		{Path: "github.com/charmbracelet/lipgloss", Version: "v1.1.0"},
		{Path: "github.com/alexflint/go-arg", Version: "v1.6.1"},
	}
	if len(file.Requires) != len(wantRequires) {
		t.Fatalf("Requires = %#v, want %#v", file.Requires, wantRequires)
	}
	for i := range wantRequires {
		if file.Requires[i] != wantRequires[i] {
			t.Fatalf("Requires[%d] = %#v, want %#v", i, file.Requires[i], wantRequires[i])
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
	if got := file.PackageContent; got != wantPackage {
		t.Fatalf("PackageLines joined = %q, want %q", got, wantPackage)
	}

	wantMain := "fmt.Println(hasTodo(\"README.md\"))\n"
	if got := file.MainContent; got != wantMain {
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

	file, err := parseSource("hello.gorn", source)
	if err != nil {
		t.Fatal(err)
	}
	if file.PackageStart == nil || *file.PackageStart != 4 {
		t.Fatalf("PackageStart = %v, want 4", intPtrValue(file.PackageStart))
	}
	if file.MainStart != 6 {
		t.Fatalf("MainStart = %d, want 6", file.MainStart)
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

	file, err := parseSource("blank-main.gorn", source)
	if err != nil {
		t.Fatal(err)
	}
	if file.MainStart != 4 {
		t.Fatalf("MainStart = %d, want 4", file.MainStart)
	}
	if got := file.MainContent; got != "fmt.Println(\"hello\")\n" {
		t.Fatalf("MainLines joined = %q, want %q", got, "fmt.Println(\"hello\")\n")
	}
}

func TestParseSourceAllowsShebangOnlyOnFirstLine(t *testing.T) {
	source := []byte("//gorn:main\n" +
		"#! this is main source, not a shebang\n")

	file, err := parseSource("shebang.gorn", source)
	if err != nil {
		t.Fatal(err)
	}

	want := "#! this is main source, not a shebang\n"
	if got := file.MainContent; got != want {
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
			name:     "missing main",
			source:   "import \"fmt\"\n",
			wantErr:  script.ErrMissingMain,
			wantLine: 1,
		},
		{
			name: "multiple main directives",
			source: "//gorn:main\n" +
				"println(\"first\")\n" +
				"//gorn:main\n",
			wantErr:  script.ErrMultipleMain,
			wantLine: 3,
		},
		{
			name: "directive after main",
			source: "//gorn:main\n" +
				"//gorn:go 1.26\n",
			wantErr:  script.ErrDirectiveAfterMain,
			wantLine: 2,
		},
		{
			name: "unknown directive",
			source: "//gorn:unknown value\n" +
				"//gorn:main\n",
			wantErr:  script.ErrInvalidDirective,
			wantLine: 1,
		},
		{
			name: "require missing version",
			source: "//gorn:require github.com/example/tool\n" +
				"//gorn:main\n",
			wantErr:  script.ErrInvalidRequire,
			wantLine: 1,
		},
		{
			name: "require rejects latest",
			source: "//gorn:require github.com/example/tool latest\n" +
				"//gorn:main\n",
			wantErr:  script.ErrInvalidRequire,
			wantLine: 1,
		},
		{
			name: "duplicate go directive",
			source: "//gorn:go 1.26\n" +
				"//gorn:go 1.27\n" +
				"//gorn:main\n",
			wantErr:  script.ErrDuplicateGo,
			wantLine: 2,
		},
		{
			name: "duplicate module directive",
			source: "//gorn:module example.com/scripts/todo\n" +
				"//gorn:module example.com/scripts/other\n" +
				"//gorn:main\n",
			wantErr:  script.ErrDuplicateModule,
			wantLine: 2,
		},
		{
			name: "directive after real package content",
			source: "import \"fmt\"\n" +
				"//gorn:require github.com/example/tool v1.0.0\n" +
				"//gorn:main\n",
			wantErr:  script.ErrDirectiveAfterPackage,
			wantLine: 2,
		},
		{
			name:     "main directive is the last line",
			source:   "//gorn:main\n",
			wantErr:  script.ErrEmptyMain,
			wantLine: 1,
		},
		{
			name:     "main section is only blank lines",
			source:   "//gorn:main\n\n\n",
			wantErr:  script.ErrEmptyMain,
			wantLine: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSource("bad.gorn", []byte(tt.source))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want errors.Is(_, %v)", err, tt.wantErr)
			}

			if tt.wantLine == 0 {
				return
			}

			var parseErr *script.LineError
			if !errors.As(err, &parseErr) {
				t.Fatalf("error type = %T, want *script.LineError", err)
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

	file, err := parseSource("multi-require.gorn", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Requires) != 2 {
		t.Fatalf("Requires = %#v, want 2 entries", file.Requires)
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

	file, err := parseSource("preamble.gorn", source)
	if err != nil {
		t.Fatal(err)
	}
	if !file.UsePreamble {
		t.Fatal("UsePreamble = false, want true")
	}
}

func TestParseSourceRejectsPreambleWithArgs(t *testing.T) {
	source := []byte("//gorn:preamble extra\n" +
		"//gorn:main\n" +
		"println(\"hi\")\n")

	_, err := parseSource("preamble.gorn", source)
	if !errors.Is(err, script.ErrInvalidDirective) {
		t.Fatalf("error = %v, want errors.Is(_, %v)", err, script.ErrInvalidDirective)
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

	file, err := parseSource("blank-spacing.gorn", source)
	if err != nil {
		t.Fatal(err)
	}
	if file.GoVersion != "1.26" {
		t.Fatalf("GoVersion = %q, want 1.26", file.GoVersion)
	}
	if file.Module != "example.com/scripts/todo" {
		t.Fatalf("Module = %q, want example.com/scripts/todo", file.Module)
	}
	if file.PackageStart == nil || *file.PackageStart != 6 {
		t.Fatalf("PackageStart = %v, want 6", intPtrValue(file.PackageStart))
	}
	if got := file.PackageContent; got != "import \"fmt\"\n" {
		t.Fatalf("PackageLines joined = %q, want %q", got, "import \"fmt\"\n")
	}
}

func TestParseSourceStillRejectsDirectiveAfterRealContentDespiteBlanks(t *testing.T) {
	source := []byte("import \"fmt\"\n" +
		"\n" +
		"//gorn:require github.com/example/tool v1.0.0\n" +
		"//gorn:main\n")

	_, err := parseSource("bad.gorn", source)
	if !errors.Is(err, script.ErrDirectiveAfterPackage) {
		t.Fatalf("error = %v, want errors.Is(_, %v)", err, script.ErrDirectiveAfterPackage)
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
			_, err := parseSource("bad.gorn", []byte(tt.source))
			if !errors.Is(err, script.ErrInvalidDirective) {
				t.Fatalf("error = %v, want errors.Is(_, %v)", err, script.ErrInvalidDirective)
			}
		})
	}
}

func intPtrValue(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
