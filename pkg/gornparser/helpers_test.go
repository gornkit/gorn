package gornparser_test

import (
	gp "github.com/gornkit/gorn/pkg/gornparser"
	so "github.com/gornkit/gorn/pkg/source"
)

// parseSource wraps bytes in a source.Source (as the CLI does) before parsing,
// so tests keep passing a name + bytes after ParseSource moved to *source.Source.
func parseSource(name string, src []byte) (*gp.Script, error) {
	s, err := so.New(name, src)
	if err != nil {
		return nil, err
	}
	return gp.ParseSource(s)
}
