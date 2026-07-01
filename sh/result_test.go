package sh

import (
	"errors"
	"testing"
)

func TestResultOKAndError(t *testing.T) {
	err := errors.New("boom")

	if !((Result{}).OK()) {
		t.Fatal("zero result should be OK")
	}
	if (Result{Code: 1}).OK() {
		t.Fatal("non-zero code should not be OK")
	}
	if (Result{Err: err}).OK() {
		t.Fatal("error should not be OK")
	}
	if got := (Result{Err: err}).Error(); !errors.Is(got, err) {
		t.Fatalf("Error() = %v, want %v", got, err)
	}
}

func TestFormatFailure(t *testing.T) {
	err := errors.New("boom")

	tests := []struct {
		name string
		in   Result
		want string
	}{
		{
			name: "operation",
			in:   Result{Op: "copy", Err: err},
			want: `operation: copy
cause: boom
`,
		},
		{
			name: "command",
			in: Result{
				Cmd: struct {
					Name string
					Args []string
				}{Name: "echo", Args: []string{"hello", "world"}},
				Code: 2,
			},
			want: `command: echo hello world
exit status: 2
`,
		},
	}

	for _, tt := range tests {
		if got := formatFailure(tt.in); got != tt.want {
			t.Fatalf("%s: formatFailure() = %q, want %q", tt.name, got, tt.want)
		}
	}
}
