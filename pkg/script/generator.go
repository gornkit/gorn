package script

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"text/template"
)

const moduleDefault string = "gorn.local/app"

// defaultGoVersion is used when a script omits //gorn:go. It is a deliberate,
// hand-maintained floor — nothing enforces that it tracks gorn's own go.mod, so
// bump it by hand when raising the minimum Go version gorn generates against.
const defaultGoVersion string = "1.26"

//go:embed mod.gotmpl
var modTemplate string

//go:embed main.gotmpl
var mainTemplate string

var modTmpl = template.Must(template.New("mod").
	Option("missingkey=error").
	Parse(modTemplate))

var mainTmpl = template.Must(template.New("main").
	Option("missingkey=error").
	Funcs(template.FuncMap{
		// {{ indent "\t" .MainLines }}
		"indent": func(indentChar, source string) string {
			lines := strings.Split(source, "\n")
			for i, line := range lines {
				if line != "" {
					lines[i] = indentChar + line
				}
			}
			return strings.Join(lines, "\n")
		},
	}).
	Parse(mainTemplate))

// preamblePackages pairs each preamble import path with a symbol guaranteed to
// exist in that package, so the generated main file can reference the symbol
// via a keep-alive var and prevent the import from being flagged as unused.
var preamblePackages = []struct{ Path, Symbol string }{
	{"path/filepath", "filepath.Separator"},
	{"fmt", "fmt.Println"},
	{"os", "os.Stdout"},
	{"github.com/gornkit/gorn/sh", "sh.PreambleSentinel"},
	{"strconv", "strconv.IntSize"},
	{"strings", "strings.Contains"},
	{"time", "time.Nanosecond"},
}

// Generated holds the successful output of Generate: a formatted main file
// and a go.mod. Generate returns a non-nil error (of type *GenerateError)
// instead of a partial Generated when generation fails.
type Generated struct {
	MainFileFormatted []byte
	ModGenerated      []byte
}

// GenerateError reports a failure to generate compilable output for a script.
// It wraps the underlying cause and, for format failures, carries the raw
// unformatted main file so callers can still inspect it.
//
//   - For a preamble import conflict, Err unwraps to ErrPreambleImportConflict
//     and carries the offending import's original source line; Raw is nil.
//   - For a format failure, Err is the go/format error and Raw is the raw,
//     unformatted generated main file.
type GenerateError struct {
	Err error
	Raw []byte
}

func (e *GenerateError) Error() string { return e.Err.Error() }

func (e *GenerateError) Unwrap() error { return e.Err }

func Generate(f *File) (*Generated, error) {
	if f.UsePreamble {
		if err := checkPreambleConflicts(f); err != nil {
			return nil, err
		}
	}

	modData := struct {
		Module    string
		GoVersion string
		Requires  []Require
	}{
		Module:    f.Module,
		GoVersion: f.GoVersion,
		Requires:  f.Requires,
	}

	if modData.Module == "" {
		modData.Module = moduleDefault
	}
	if modData.GoVersion == "" {
		modData.GoVersion = defaultGoVersion
	}

	var modBuf bytes.Buffer
	if err := modTmpl.Execute(&modBuf, &modData); err != nil {
		return nil, err
	}

	mainData := struct {
		SourcePath      string
		MainContent     string
		PackageContent  string
		MainStart       int
		PackageStart    *int
		PreambleImports string
		PreambleVars    string
	}{
		SourcePath:     f.SourcePath,
		PackageContent: f.PackageContent,
		MainContent:    f.MainContent,
		MainStart:      f.MainStart,
		PackageStart:   f.PackageStart,
	}

	if f.UsePreamble {
		var pkgsBuilder strings.Builder
		var varsBuilder strings.Builder
		for _, pair := range preamblePackages {
			fmt.Fprintf(&pkgsBuilder, "%q\n", pair.Path)
			fmt.Fprintf(&varsBuilder, "_ = %s\n", pair.Symbol)
		}

		mainData.PreambleImports = pkgsBuilder.String()
		mainData.PreambleVars = varsBuilder.String()
	}

	var mainBuf bytes.Buffer
	if err := mainTmpl.Execute(&mainBuf, &mainData); err != nil {
		return nil, err
	}

	mainGenerated := mainBuf.Bytes()
	formattedMain, err := format.Source(mainGenerated)
	if err != nil {
		return nil, &GenerateError{Err: err, Raw: mainGenerated}
	}

	fset := token.NewFileSet()
	// Parse the formatted main file to ensure it is valid Go code.
	if _, err := parser.ParseFile(fset, f.SourcePath, formattedMain, parser.SkipObjectResolution); err != nil {
		return nil, &GenerateError{Err: err, Raw: mainGenerated}
	}

	return &Generated{
		MainFileFormatted: formattedMain,
		ModGenerated:      modBuf.Bytes(),
	}, nil
}

// checkPreambleConflicts rejects a script that both uses //gorn:preamble and
// imports one of the preamble packages itself. Detection is by import path
// (any alias counts), matching the ambient-contract model: preamble packages
// are provided, so importing one is redundant at best and a redeclaration at
// worst. The error points at the offending import's original source line.
//
// This only inspects imports (parser.ImportsOnly); it does not type-check.
// If the package section does not parse, the error is left for the real build
// to surface against //line-mapped output.
func checkPreambleConflicts(s *File) error {
	if s.PackageStart == nil {
		return nil
	}

	// Prepend a synthetic package clause so the fragment parses as a file.
	// The clause occupies line 1, and PackageContent begins at *PackageStart
	// in the original source, so an import parsed at line N maps to original
	// line (*PackageStart + N - 2).
	fset := token.NewFileSet()
	f, err := parser.ParseFile(
		fset,
		s.SourcePath,
		"package main\n"+s.PackageContent,
		parser.ImportsOnly)
	if err != nil {
		return nil
	}

	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if !isPreamblePackage(path) {
			continue
		}
		origLine := *s.PackageStart + fset.Position(imp.Pos()).Line - 2
		return &GenerateError{
			Err: lineError(origLine, fmt.Errorf("%w: %s", ErrPreambleImportConflict, path)),
		}
	}

	return nil
}

func isPreamblePackage(path string) bool {
	for _, pair := range preamblePackages {
		if pair.Path == path {
			return true
		}
	}
	return false
}
