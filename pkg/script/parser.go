package script

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gornkit/gorn/pkg/app"
)

type ParserError string

const (
	ErrInvalidPackage         ParserError = "script must declare package main"
	ErrMissingMain            ParserError = "missing func main()"
	ErrMultipleMain           ParserError = "multiple main functions"
	ErrInvalidMain            ParserError = "main must have a body and no parameters, results, or type parameters"
	ErrDirectiveAfterPackage  ParserError = "gorn directive after package clause"
	ErrInvalidDirective       ParserError = "invalid gorn directive"
	ErrInvalidRequire         ParserError = "invalid //gorn:require directive"
	ErrDuplicateGo            ParserError = "duplicate //gorn:go directive"
	ErrDuplicateModule        ParserError = "duplicate //gorn:module directive"
	ErrDuplicatePreamble      ParserError = "duplicate //gorn:preamble directive"
	ErrPreambleImportConflict ParserError = "import conflicts with //gorn:preamble package"
)

func (e ParserError) Error() string { return string(e) }

type LineError struct {
	Line int
	Err  error
}

func (e *LineError) Error() string             { return e.Err.Error() + " at line " + strconv.Itoa(e.Line) }
func (e *LineError) Unwrap() error             { return e.Err }
func lineError(line int, err error) *LineError { return &LineError{Line: line, Err: err} }

// File is one complete Go main file and its gorn metadata. Content preserves
// the source bytes, except a first-line shebang (or UTF-8 BOM) is blanked with
// spaces without changing offsets or line endings. Do not modify it after Parse.
type File struct {
	SourcePath  string
	GoVersion   string
	Module      string
	Requires    []Require
	UsePreamble bool
	Content     []byte

	sourceName string
	fset       *token.FileSet
	syntax     *ast.File
}

type Require struct{ Path, Version string }

// Parse validates the Go file and entry point and reads standalone metadata
// comments from the header before package main. Go syntax errors retain their
// original source positions and underlying scanner.ErrorList.
func Parse(s *app.Source) (*File, error) {
	name := s.Path()
	if name != "/dev/stdin" {
		var err error
		name, err = filepath.Abs(name)
		if err != nil {
			return nil, err
		}
	}
	f := &File{SourcePath: s.Path(), sourceName: name, Content: bytes.Clone(s.Data()), fset: token.NewFileSet()}
	if bytes.HasPrefix(f.Content, []byte("#!")) {
		for i, b := range f.Content {
			if b == '\n' {
				break
			}
			if b != '\r' {
				f.Content[i] = ' '
			}
		}
	}
	if bytes.HasPrefix(f.Content, []byte{0xef, 0xbb, 0xbf}) {
		copy(f.Content, "   ")
	}
	var err error
	f.syntax, err = parser.ParseFile(f.fset, name, f.Content, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	for _, group := range f.syntax.Comments {
		for _, comment := range group.List {
			if !strings.HasPrefix(comment.Text, "//gorn:") {
				continue
			}
			pos := f.position(comment.Pos())
			start := bytes.LastIndexByte(f.Content[:pos.Offset], '\n') + 1
			if len(bytes.TrimSpace(f.Content[start:pos.Offset])) != 0 {
				continue
			}
			if comment.Pos() > f.syntax.Package {
				return nil, lineError(pos.Line, ErrDirectiveAfterPackage)
			}
			if err := f.applyDirective(strings.TrimPrefix(comment.Text, "//gorn:")); err != nil {
				return nil, lineError(pos.Line, err)
			}
		}
	}
	if f.syntax.Name.Name != "main" {
		return nil, lineError(f.position(f.syntax.Name.Pos()).Line, ErrInvalidPackage)
	}
	var main *ast.FuncDecl
	for _, decl := range f.syntax.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != "main" {
			continue
		}
		if main != nil {
			return nil, lineError(f.position(fn.Name.Pos()).Line, ErrMultipleMain)
		}
		main = fn
	}
	if main == nil {
		return nil, lineError(f.position(f.syntax.Package).Line, ErrMissingMain)
	}
	if main.Body == nil || main.Type.Params.NumFields() != 0 || main.Type.Results.NumFields() != 0 || main.Type.TypeParams.NumFields() != 0 {
		return nil, lineError(f.position(main.Name.Pos()).Line, ErrInvalidMain)
	}
	return f, nil
}

func (f *File) position(pos token.Pos) token.Position { return f.fset.PositionFor(pos, false) }

// packageEnd includes the package terminator and its trailing comments. Keeping
// comments in place avoids promoting a trailing //line into a column-one
// directive. Implicit semicolons inside block comments must not split them.
func (f *File) packageEnd() int {
	var scan scanner.Scanner
	file := token.NewFileSet().AddFile(f.sourceName, -1, len(f.Content))
	scan.Init(file, f.Content, nil, scanner.ScanComments)
	nameEnd := f.position(f.syntax.Name.End()).Offset
	commentEnd := 0
	for {
		pos, tok, _ := scan.Scan()
		offset := file.Offset(pos)
		if tok == token.COMMENT && offset >= nameEnd {
			commentEnd = f.commentEnd(offset)
		}
		if tok == token.SEMICOLON {
			end := max(offset+1, commentEnd)
			for end < len(f.Content) && f.Content[end-1] != '\n' {
				pos, tok, _ = scan.Scan()
				offset = file.Offset(pos)
				if tok != token.COMMENT || bytes.ContainsRune(f.Content[end:offset], '\n') || len(bytes.TrimSpace(f.Content[end:offset])) != 0 {
					break
				}
				end = f.commentEnd(offset)
			}
			return end
		}
		if tok == token.EOF {
			return nameEnd
		}
	}
}

func (f *File) commentEnd(offset int) int {
	if bytes.HasPrefix(f.Content[offset:], []byte("//")) {
		if newline := bytes.IndexByte(f.Content[offset:], '\n'); newline >= 0 {
			return offset + newline + 1
		}
		return len(f.Content)
	}
	return offset + bytes.Index(f.Content[offset:], []byte("*/")) + 2
}

func (f *File) applyDirective(directive string) error {
	parts := strings.Fields(directive)
	if len(parts) == 0 {
		return ErrInvalidDirective
	}
	switch parts[0] {
	case "go":
		if len(parts) != 2 {
			return ErrInvalidDirective
		}
		if f.GoVersion != "" {
			return ErrDuplicateGo
		}
		f.GoVersion = parts[1]
	case "module":
		if len(parts) != 2 {
			return ErrInvalidDirective
		}
		if f.Module != "" {
			return ErrDuplicateModule
		}
		f.Module = parts[1]
	case "require":
		if len(parts) != 3 || parts[2] == "latest" {
			return ErrInvalidRequire
		}
		f.Requires = append(f.Requires, Require{Path: parts[1], Version: parts[2]})
	case "preamble":
		if len(parts) != 1 {
			return ErrInvalidDirective
		}
		if f.UsePreamble {
			return ErrDuplicatePreamble
		}
		f.UsePreamble = true
	default:
		return ErrInvalidDirective
	}
	return nil
}

// Dump writes source and metadata for verbose diagnostics on a best-effort basis.
func (f *File) Dump(w io.Writer) {
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
	p("--- parsed script ---\nPath: %s\nGoVersion: %q\nModule: %q\nUsePreamble: %t\nRequires: %+v\n--- source ---\n%s",
		f.SourcePath, f.GoVersion, f.Module, f.UsePreamble, f.Requires, f.Content)
}
