package buildprofile

import (
	"reflect"
	"strings"
	"testing"
)

func TestGoWordQuotingIsLiteral(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []string
	}{
		{` a 'two words' "literal\backslash" `, []string{"a", "two words", `literal\backslash`}},
		{`'a'b`, []string{"a", "b"}},
		{`main.X=both'"`, []string{`main.X=both'"`}},
		{`''`, []string{""}},
	} {
		got, err := splitGoWords(tc.text)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("split %q: %q %v", tc.text, got, err)
		}
		encoded, err := joinGoWords(got)
		if err != nil {
			t.Fatal(err)
		}
		roundtrip, err := splitGoWords(encoded)
		if err != nil || !reflect.DeepEqual(roundtrip, got) {
			t.Fatalf("roundtrip %q: %q %v", encoded, roundtrip, err)
		}
	}
	if _, err := splitGoWords(`"unfinished`); err == nil {
		t.Fatal("accepted malformed quotes")
	}
}

func TestSupportedFlagsNormalizeAndPreserveLinkerValues(t *testing.T) {
	flags, err := parseFlags(`-tags=z,a,z -trimpath "-ldflags=-w -X 'main.Message=hello \ world' -s"`)
	if err != nil {
		t.Fatal(err)
	}
	if flags.tags != "a,z" || !flags.trimpath {
		t.Fatalf("normalization: %+v", flags)
	}
	words, err := splitGoWords(flags.linker)
	if err != nil || !reflect.DeepEqual(words, []string{"-s", "-w", "-X", `main.Message=hello \ world`}) {
		t.Fatalf("linker value changed: %q %v", words, err)
	}
	other, err := parseFlags(`-tags=a,z -trimpath=true "-ldflags=-s -w -X 'main.Message=hello \ world'"`)
	if err != nil || other != flags {
		t.Fatalf("equivalent flags differ: %+v %v", other, err)
	}
}

func TestUnsupportedFlagsNeverEchoValues(t *testing.T) {
	for _, text := range []string{
		"-race", "-asan", "-msan", "-overlay=private-value", "-modfile=private-value", "-pkgdir=private-value", "-pgo=private-value",
		"-toolexec=private-value", "-gcflags=private-value", "-asmflags=private-value", "-compiler=gccgo", "-buildmode=shared",
		`"-ldflags=-extldflags=private-value"`, `"-ldflags=-X main.X=private-value -X main.X=conflicting-value"`,
		`-tags=a -tags=b`, `-trimpath=private-value`, `-ldflags`, `"-ldflags=-X"`, `-tags=a+b`,
	} {
		_, err := parseFlags(text)
		if err == nil || strings.Contains(err.Error(), "private-value") || strings.Contains(err.Error(), "conflicting-value") {
			t.Fatalf("invalid input accepted or echoed: %q: %v", text, err)
		}
	}
}
