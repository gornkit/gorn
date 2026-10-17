package buildprofile

import (
	"fmt"
	"go/build/constraint"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/mod/module"
)

// splitGoWords follows cmd/go's flag quoting: only a quote at the start of a
// field delimits it, and quoted contents are literal, with no shell escaping.
func splitGoWords(text string) ([]string, error) {
	var words []string
	for text = strings.TrimLeft(text, " \t\r\n"); text != ""; text = strings.TrimLeft(text, " \t\r\n") {
		if text[0] == '\'' || text[0] == '"' {
			end := strings.IndexByte(text[1:], text[0])
			if end < 0 {
				return nil, fmt.Errorf("unterminated Go flag quote")
			}
			words = append(words, text[1:end+1])
			text = text[end+2:]
			continue
		}
		end := strings.IndexAny(text, " \t\r\n")
		if end < 0 {
			end = len(text)
		}
		words = append(words, text[:end])
		text = text[end:]
	}
	return words, nil
}

func joinGoWords(words []string) (string, error) {
	quoted := make([]string, 0, len(words))
	for _, word := range words {
		switch {
		case word != "" && !strings.ContainsAny(word, " \t\r\n") && word[0] != '\'' && word[0] != '"':
			quoted = append(quoted, word)
		case !strings.ContainsRune(word, '\''):
			quoted = append(quoted, "'"+word+"'")
		case !strings.ContainsRune(word, '"'):
			quoted = append(quoted, `"`+word+`"`)
		default:
			return "", fmt.Errorf("linker value cannot be represented with Go flag quoting")
		}
	}
	return strings.Join(quoted, " "), nil
}

type buildFlags struct {
	tags     string
	trimpath bool
	linker   string
}

func parseFlags(text string) (buildFlags, error) {
	var result buildFlags
	words, err := splitGoWords(text)
	if err != nil {
		return result, err
	}
	seen := make(map[string]bool)
	for _, word := range words {
		name, value, hasValue := strings.Cut(word, "=")
		if seen[name] {
			return result, fmt.Errorf("duplicate GOFLAGS option %s; supply one value", name)
		}
		seen[name] = true
		switch name {
		case "-tags":
			if !hasValue {
				return result, fmt.Errorf("GOFLAGS -tags requires =<tag-list>")
			}
			tags := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
			for _, tag := range tags {
				expr, err := constraint.Parse("//go:build " + tag)
				if parsed, ok := expr.(*constraint.TagExpr); err != nil || !ok || parsed.Tag != tag {
					return result, fmt.Errorf("invalid build tag")
				}
			}
			slices.Sort(tags)
			result.tags = strings.Join(slices.Compact(tags), ",")
		case "-trimpath":
			result.trimpath = true
			if hasValue {
				result.trimpath, err = strconv.ParseBool(value)
				if err != nil {
					return result, fmt.Errorf("invalid -trimpath boolean")
				}
			}
		case "-ldflags":
			if !hasValue {
				return result, fmt.Errorf("GOFLAGS -ldflags requires =<linker-flags>")
			}
			result.linker, err = parseLinker(value)
			if err != nil {
				return result, err
			}
		default:
			// Values may contain secrets, so report only the option name.
			return result, fmt.Errorf("unsupported GOFLAGS option; supported: -tags, -trimpath, -ldflags (-s, -w, -X)")
		}
	}
	return result, nil
}

func parseLinker(text string) (string, error) {
	words, err := splitGoWords(text)
	if err != nil {
		return "", err
	}
	assignments := make(map[string]string)
	var strip, dwarf bool
	for i := 0; i < len(words); i++ {
		word := words[i]
		switch {
		case word == "-s":
			strip = true
		case word == "-w":
			dwarf = true
		case word == "-X" || strings.HasPrefix(word, "-X="):
			value := strings.TrimPrefix(word, "-X=")
			if word == "-X" {
				i++
				if i >= len(words) {
					return "", fmt.Errorf("linker -X needs importpath.name=value")
				}
				value = words[i]
			}
			name, _, found := strings.Cut(value, "=")
			dot := strings.LastIndexByte(name, '.')
			if !found || dot < 1 || !token.IsIdentifier(name[dot+1:]) || module.CheckImportPath(name[:dot]) != nil {
				return "", fmt.Errorf("linker -X needs importpath.name=value")
			}
			if previous, exists := assignments[name]; exists && previous != value {
				return "", fmt.Errorf("conflicting linker -X assignments")
			}
			assignments[name] = value
		default:
			return "", fmt.Errorf("unsupported linker flag; supported: -s, -w, -X importpath.name=value")
		}
	}
	var normalized []string
	if strip {
		normalized = append(normalized, "-s")
	}
	if dwarf {
		normalized = append(normalized, "-w")
	}
	names := make([]string, 0, len(assignments))
	for name := range assignments {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		normalized = append(normalized, "-X", assignments[name])
	}
	return joinGoWords(normalized)
}

func (f buildFlags) args() []string {
	var args []string
	if f.tags != "" {
		args = append(args, "-tags="+f.tags)
	}
	if f.trimpath {
		args = append(args, "-trimpath")
	}
	if f.linker != "" {
		args = append(args, "-ldflags="+f.linker)
	}
	return args
}
