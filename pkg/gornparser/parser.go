package gornparser

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	so "github.com/gornkit/gorn/pkg/source"
)

type ParserError string

const (
	ErrMissingMain            ParserError = "missing //gorn:main directive"
	ErrMultipleMain           ParserError = "multiple //gorn:main directives"
	ErrDirectiveAfterMain     ParserError = "gorn directive after //gorn:main"
	ErrDirectiveAfterPackage  ParserError = "gorn directive after package section content has started"
	ErrInvalidDirective       ParserError = "invalid gorn directive"
	ErrInvalidRequire         ParserError = "invalid //gorn:require directive"
	ErrDuplicateGo            ParserError = "duplicate //gorn:go directive"
	ErrDuplicateModule        ParserError = "duplicate //gorn:module directive"
	ErrEmptyMain              ParserError = "empty //gorn:main section"
	ErrPreambleImportConflict ParserError = "import conflicts with //gorn:preamble package"
)

func (e ParserError) Error() string { return string(e) }

type Error struct {
	Line int
	Err  error
}

func (e *Error) Error() string {
	return e.Err.Error() + " at line " + strconv.Itoa(e.Line)
}

func (e *Error) Unwrap() error { return e.Err }

func lineError(line int, err error) *Error {
	return &Error{Line: line, Err: err}
}

// Script is the parsed representation of a .gorn source file.
type Script struct {
	// SourcePath is the path passed to ParseFile/ParseSource ("-" for stdin).
	SourcePath string

	GoVersion   string
	Module      string
	Requires    []Require
	UsePreamble bool

	// PackageContent holds the raw lines (including line terminators) that
	// make up the package section: everything before //gorn:main, minus
	// directive lines and the optional line-1 shebang. Directives are
	// only permitted before any package-section content, so PackageContent
	// is a contiguous run of the original source lines (no gaps) — this
	// makes PackageStart sufficient as a single //line anchor when
	// generating Go code.
	PackageContent string

	// MainContent holds the raw lines (including line terminators) that make
	// up the main section: every line after //gorn:main, except leading
	// blank lines immediately following the directive, which are dropped
	// (Gorn never needs to reverse-generate the original source, so this
	// trivia isn't preserved). Its first line corresponds to the source
	// line recorded in MainStart.
	MainContent string

	// PackageStart is the 1-based line number of the first non-blank
	// package-section line. Leading blank lines (including any that
	// appear between or after directives) are not recorded and do not
	// set PackageStart, so directives may appear after blank-only
	// spacing without being rejected. PackageStart is nil when the
	// script has no non-blank package-section content before
	// //gorn:main.
	PackageStart *int

	// MainStart is the 1-based line number of the first non-blank line
	// after //gorn:main. A script whose main section has no non-blank
	// content (e.g. //gorn:main is the last line, or only blank lines
	// follow it) is rejected with ErrEmptyMain, so MainStart is always
	// a valid line (never 0) on a successful parse.
	MainStart int
}

// Dump writes a developer-oriented summary of the parsed script, including
// parser internals, to w on a best-effort basis. It backs `gorn run
// --verbose` diagnostics.
func (s *Script) Dump(w io.Writer) {
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
	p("--- parsed script ---\n")
	p("Path:         %s\n", s.SourcePath)
	p("GoVersion:    %q\n", s.GoVersion)
	p("Module:       %q\n", s.Module)
	p("UsePreamble:  %t\n", s.UsePreamble)
	p("Requires:     %+v\n", s.Requires)
	p("PackageStart: %s\n", intPtrString(s.PackageStart))
	p("MainStart:    %d\n", s.MainStart)
	p("--- PackageContent ---\n")
	p("%s", s.PackageContent)
	p("--- MainContent ---\n")
	p("%s", s.MainContent)
	p("--- end ---\n")
}

func intPtrString(p *int) string {
	if p == nil {
		return "<nil>"
	}
	return strconv.Itoa(*p)
}

type Require struct {
	Path    string
	Version string
}

type state struct {
	seenMain          bool
	mainDirectiveLine int
	currLine          int
	mainBuilder       strings.Builder
	packageBuilder    strings.Builder
	script            *Script
}

func ParseSource(s *so.Source) (*Script, error) {
	data := s.Data()

	state := &state{
		script: &Script{
			SourcePath: s.Path(),
		},
	}

	for line := range bytes.Lines(data) {
		state.currLine++
		// Shebang handling is intentionally permissive: any line 1 prefixed
		// with "#!" is treated as a shebang and discarded verbatim. Gorn
		// does not validate or require a specific shebang target (e.g.
		// "#!/usr/bin/env gorn"); this is a deliberate choice, not an
		// oversight, so scripts remain runnable regardless of how they are
		// invoked.
		if state.currLine == 1 && bytes.HasPrefix(line, []byte("#!")) {
			continue
		}

		trimmed := bytes.TrimSpace(line)

		// handle directives
		if after, ok := bytes.CutPrefix(trimmed, []byte("//gorn:")); ok {
			directive := after
			if strings.TrimSpace(string(directive)) == "main" {
				if state.seenMain {
					return nil, lineError(state.currLine, ErrMultipleMain)
				}
				state.seenMain = true
				state.mainDirectiveLine = state.currLine
				continue
			}

			if state.seenMain {
				return nil, lineError(state.currLine, ErrDirectiveAfterMain)
			}
			// Directives must precede all real (non-blank) package-section
			// content, so that PackageLines stays a contiguous run of
			// source lines once it starts. A single //line marker at
			// PackageStart would otherwise misalign once a directive is
			// filtered out from the middle of an otherwise-contiguous
			// block. Leading blank lines don't count as "started" (see the
			// package-section branch below), so directives may still
			// appear after blank-only spacing.
			if state.script.PackageStart != nil {
				return nil, lineError(state.currLine, ErrDirectiveAfterPackage)
			}

			if err := state.applyDirective(directive); err != nil {
				return nil, lineError(state.currLine, err)
			}

			continue
		}

		if state.seenMain {
			// Leading blank lines immediately after //gorn:main carry no
			// content worth generating or anchoring a //line marker to.
			// Gorn never needs to reverse-generate the original source, so
			// they are dropped entirely rather than preserved, keeping
			// MainStart aligned with the first main line.
			if state.script.MainStart == 0 && len(trimmed) == 0 {
				continue
			}
			if state.script.MainStart == 0 {
				state.script.MainStart = state.currLine
			}
			state.mainBuilder.Write(line)
		} else {
			// Leading blank lines carry no content worth anchoring a //line
			// marker to, and including them would let a following directive
			// falsely appear to come "after package content". Drop them
			// entirely until the first non-blank line is seen.
			if state.script.PackageStart == nil && len(trimmed) == 0 {
				continue
			}
			if state.script.PackageStart == nil {
				state.script.PackageStart = new(state.currLine)
			}
			state.packageBuilder.Write(line)
		}

		continue
	}

	if !state.seenMain {
		return nil, lineError(state.currLine, ErrMissingMain)
	}
	if state.script.MainStart == 0 {
		return nil, lineError(state.mainDirectiveLine, ErrEmptyMain)
	}

	state.script.MainContent = state.mainBuilder.String()
	state.script.PackageContent = state.packageBuilder.String()
	return state.script, nil
}

func (s *state) applyDirective(directive []byte) error {
	parts := bytes.Fields(directive)
	if len(parts) == 0 {
		return ErrInvalidDirective
	}

	switch string(parts[0]) {
	case "go":
		if len(parts) != 2 {
			return ErrInvalidDirective
		}
		if s.script.GoVersion != "" {
			return ErrDuplicateGo
		}
		s.script.GoVersion = string(parts[1])
		return nil
	case "module":
		if len(parts) != 2 {
			return ErrInvalidDirective
		}
		if s.script.Module != "" {
			return ErrDuplicateModule
		}
		s.script.Module = string(parts[1])
		return nil
	case "require":
		if len(parts) != 3 {
			return ErrInvalidRequire
		}
		if string(parts[2]) == "latest" {
			return ErrInvalidRequire
		}
		s.script.Requires = append(s.script.Requires, Require{
			Path:    string(parts[1]),
			Version: string(parts[2]),
		})
		return nil
	case "preamble":
		if len(parts) != 1 {
			return ErrInvalidDirective
		}
		s.script.UsePreamble = true
		return nil
	}

	return ErrInvalidDirective
}
