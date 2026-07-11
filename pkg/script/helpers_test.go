package script_test

import (
	"github.com/gornkit/gorn/pkg/app"
	"github.com/gornkit/gorn/pkg/script"
)

// parseSource wraps bytes in a source.Source (as the CLI does) before parsing,
// so tests keep passing a name + bytes after Parse moved to *source.Source.
func parseSource(name string, src []byte) (*script.File, error) {
	s, err := app.New(name, src)
	if err != nil {
		return nil, err
	}
	return script.Parse(s)
}
