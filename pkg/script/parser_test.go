package script_test

import (
	"bytes"
	"errors"
	"go/scanner"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gornkit/gorn/pkg/app"
	"github.com/gornkit/gorn/pkg/script"
)

func TestParseMetadataAndPreserveSource(t *testing.T) {
	source := "#!anything\r\n// ordinary header\r\n//gorn:go 1.26\r\n\r\n/* header */\r\n//gorn:module example.com/task\r\n//gorn:require example.com/a v1.2.3\r\n//gorn:require example.com/b v0.0.0-20260101000000-abcdefabcdef\r\n//gorn:preamble\r\npackage main\r\nfunc main() { helper() }\r\nfunc helper() {}"
	s, err := app.New("task.gorn", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	f, err := script.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	if f.SourcePath != "task.gorn" || f.GoVersion != "1.26" || f.Module != "example.com/task" || !f.UsePreamble {
		t.Fatalf("metadata: %+v", f)
	}
	if len(f.Requires) != 2 || f.Requires[0] != (script.Require{Path: "example.com/a", Version: "v1.2.3"}) || f.Requires[1].Path != "example.com/b" {
		t.Fatalf("requires: %+v", f.Requires)
	}
	want := strings.Repeat(" ", len("#!anything")) + source[len("#!anything"):]
	if string(f.Content) != want {
		t.Fatalf("content changed: %q", f.Content)
	}
	if !bytes.Equal(s.Data(), []byte(source)) {
		t.Fatal("Parse modified Source bytes")
	}
}

func TestParseOrdinaryGo(t *testing.T) {
	for name, source := range map[string]string{
		"empty":         "package main\nfunc main() {}",
		"helpers":       "package main\nfunc before(){}\nfunc main(){before();after()}\nfunc after(){}\n",
		"compact":       "package main;func main(){}",
		"empty results": "package main;func main()(){}",
		"method":        "package main;type T int;func(T) main(){};func main(){}",
		"literal":       "package main\nvar text = `\n//gorn:main\n//gorn:go 1.99\n`\nfunc main(){}",
		"block":         "/*\n//gorn:unknown\n*/\npackage main\n/*\n//gorn:go 1.99\n*/\nfunc main(){}",
		"trailing":      "package main //gorn:unknown\nfunc main(){ println(1) //gorn:unknown\n}",
		"bom":           "\ufeffpackage main\nfunc main(){}",
	} {
		t.Run(name, func(t *testing.T) {
			f, err := parseSource("good.gorn", []byte(source))
			if err != nil {
				t.Fatal(err)
			}
			if f.GoVersion != "" {
				t.Fatal("non-metadata changed GoVersion")
			}
		})
	}
}

func TestParseRejectsFormatErrors(t *testing.T) {
	valid := "package main\nfunc main(){}\n"
	for _, tt := range []struct {
		name, source string
		want         error
		line         int
	}{
		{"package", "package other\nfunc main(){}", script.ErrInvalidPackage, 1},
		{"missing main", "package main\nfunc helper(){}", script.ErrMissingMain, 1},
		{"method only", "package main\ntype T int\nfunc(T) main(){}", script.ErrMissingMain, 1},
		{"duplicate main", "package main\nfunc main(){}\nfunc main(){}", script.ErrMultipleMain, 3},
		{"parameters", "package main\nfunc main(x int){}", script.ErrInvalidMain, 2},
		{"result", "package main\nfunc main() int{return 0}", script.ErrInvalidMain, 2},
		{"generic", "package main\nfunc main[T any](){}", script.ErrInvalidMain, 2},
		{"no body", "package main\nfunc main()", script.ErrInvalidMain, 2},
		{"late directive", "package main\n//gorn:go 1.26\nfunc main(){}", script.ErrDirectiveAfterPackage, 2},
		{"body directive", "package main\nfunc main(){\n//gorn:module x\n}", script.ErrDirectiveAfterPackage, 3},
		{"unknown", "//gorn:unknown\n" + valid, script.ErrInvalidDirective, 1},
		{"removed main", "//gorn:main\n" + valid, script.ErrInvalidDirective, 1},
		{"empty directive", "//gorn:\n" + valid, script.ErrInvalidDirective, 1},
		{"exact name", "//gorn:mainly\n" + valid, script.ErrInvalidDirective, 1},
		{"go args", "//gorn:go\n" + valid, script.ErrInvalidDirective, 1},
		{"module args", "//gorn:module a b\n" + valid, script.ErrInvalidDirective, 1},
		{"require args", "//gorn:require a\n" + valid, script.ErrInvalidRequire, 1},
		{"latest", "//gorn:require a latest\n" + valid, script.ErrInvalidRequire, 1},
		{"duplicate go", "//gorn:go 1.26\n//gorn:go 1.27\n" + valid, script.ErrDuplicateGo, 2},
		{"duplicate module", "//gorn:module a\n//gorn:module b\n" + valid, script.ErrDuplicateModule, 2},
		{"preamble args", "//gorn:preamble x\n" + valid, script.ErrInvalidDirective, 1},
		{"duplicate preamble", "//gorn:preamble\n//gorn:preamble\n" + valid, script.ErrDuplicatePreamble, 2},
		{"physical positions", "//line other.go:99\npackage main\n//gorn:go 1.26\nfunc main(){}", script.ErrDirectiveAfterPackage, 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSource("bad.gorn", []byte(tt.source))
			if !errors.Is(err, tt.want) {
				t.Fatalf("error=%v, want %v", err, tt.want)
			}
			var lineErr *script.LineError
			if !errors.As(err, &lineErr) || lineErr.Line != tt.line {
				t.Fatalf("error=%v, want line %d", err, tt.line)
			}
		})
	}
}

func TestParseSyntaxErrorsKeepOriginalPositions(t *testing.T) {
	for _, source := range []string{
		"#!/usr/bin/env gorn\npackage main\nfunc main(){\n if {\n}\n",
		"#!/usr/bin/env gorn\r\npackage main\r\nfunc main(){\r\n if {\r\n}\r\n",
		"import \"fmt\"\nfunc main(){}",
		"package main\n#!not a first-line shebang\nfunc main(){}",
	} {
		_, err := parseSource("bad source.gorn", []byte(source))
		var list scanner.ErrorList
		if !errors.As(err, &list) || len(list) == 0 {
			t.Fatalf("error=%T %v, want Go syntax errors", err, err)
		}
		want, absErr := filepath.Abs("bad source.gorn")
		if absErr != nil {
			t.Fatal(absErr)
		}
		if list[0].Pos.Filename != want {
			t.Fatalf("filename=%q, want %q", list[0].Pos.Filename, want)
		}
		if strings.Contains(source, " if {") && list[0].Pos.Line != 4 {
			t.Fatalf("position=%v, want line 4", list[0].Pos)
		}
	}
}
