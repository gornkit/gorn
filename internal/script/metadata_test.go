package script_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gornkit/gorn/internal/script"
	"golang.org/x/mod/module"
)

func TestMetadataAcceptsConcreteGoModuleVersions(t *testing.T) {
	for _, header := range []string{
		"//gorn:go 1.26.4\n//gorn:module task",
		"//gorn:go 1.26rc1",
		"//gorn:go 1.26beta1",
		"//gorn:require example.com/tool/v2 v2.1.0",
		"//gorn:require example.com/tool v2.1.0+incompatible",
		"//gorn:require gopkg.in/yaml.v3 v3.0.1",
		"//gorn:require github.com/Acme/tool v1.2.3-rc.1",
		"//gorn:require example.com/tool v0.0.0-20260101000000-abcdefabcdef",
		"//gorn:require example.com/tool v1.2.4-0.20260101000000-abcdefabcdef",
	} {
		if _, err := parseSource("metadata.gorn", []byte(header+"\npackage main;func main(){}")); err != nil {
			t.Fatalf("%s: %v", header, err)
		}
	}
}

func TestMetadataRejectsInvalidTokensBeforeToolchain(t *testing.T) {
	t.Setenv("PATH", "")
	for _, tc := range []struct {
		header string
		want   error
	}{
		{"//gorn:go go1.26", script.ErrInvalidGoVersion},
		{"//gorn:go 1", script.ErrInvalidGoVersion},
		{"//gorn:go 1.026", script.ErrInvalidGoVersion},
		{"//gorn:go 1.26.00", script.ErrInvalidGoVersion},
		{"//gorn:go 1.26rc", script.ErrInvalidGoVersion},
		{"//gorn:go 1.26abc1", script.ErrInvalidGoVersion},
		{"//gorn:go 1.26.1rc1", script.ErrInvalidGoVersion},
		{"//gorn:module example.com/x@v1.0.0", script.ErrInvalidModule},
		{"//gorn:module example.com//x", script.ErrInvalidModule},
		{"//gorn:module ../local", script.ErrInvalidModule},
		{"//gorn:module example.com/x#bad", script.ErrInvalidModule},
		{"//gorn:require example.com/tool master", script.ErrInvalidRequire},
		{"//gorn:require example.com/tool latest", script.ErrInvalidRequire},
		{"//gorn:require example.com/tool v1.2", script.ErrInvalidRequire},
		{"//gorn:require example.com/tool abcdefabcdef", script.ErrInvalidRequire},
		{"//gorn:require example.com/tool v2.0.0", script.ErrInvalidRequire},
		{"//gorn:require example.com/tool/v2 v1.2.3", script.ErrInvalidRequire},
		{"//gorn:require example.com/tool v1.2.3+untracked", script.ErrInvalidRequire},
		{"//gorn:require example.com/tool v0.0.0-20261301000000-abcdefabcdef", script.ErrInvalidRequire},
		{"//gorn:require example.com/tool v1.2.0-0.20260101000000-abcdefabcdef", script.ErrInvalidRequire},
		{"//gorn:require example.com/tool v2.0.0-20260101000000-abcdefabcdef+incompatible", script.ErrInvalidRequire},
		{"//gorn:require ../local v1.2.3", script.ErrInvalidRequire},
	} {
		_, err := parseSource("metadata.gorn", []byte("// header\n"+tc.header+"\npackage main;func main(){}"))
		var line *script.LineError
		if !errors.Is(err, tc.want) || !errors.As(err, &line) || line.Line != 2 {
			t.Fatalf("%s: %v, want %v on line 2", tc.header, err, tc.want)
		}
	}
}

func TestDuplicateRequirementsIdentifyBothLines(t *testing.T) {
	for _, version := range []string{"v1.0.0", "v1.1.0"} {
		source := "//gorn:require example.com/tool v1.0.0\n// header\n//gorn:require example.com/tool " + version + "\npackage main;func main(){}"
		_, err := parseSource("duplicate.gorn", []byte(source))
		var line *script.LineError
		if !errors.Is(err, script.ErrDuplicateRequire) || !errors.As(err, &line) || line.Line != 3 || !strings.Contains(err.Error(), "first declared at line 1") {
			t.Fatalf("duplicate version %s: %v", version, err)
		}
	}
}

func TestMetadataPreservesGoModuleValidationCause(t *testing.T) {
	_, err := parseSource("bad.gorn", []byte("//gorn:require example.com//tool v1.0.0\npackage main;func main(){}"))
	var cause *module.InvalidPathError
	if !errors.Is(err, script.ErrInvalidRequire) || !errors.As(err, &cause) {
		t.Fatalf("lost module validation cause: %v", err)
	}
}
