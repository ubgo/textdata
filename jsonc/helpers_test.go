package jsonc

import (
	"errors"
	"testing"
)

func errorsIs(err, target error) bool { return errors.Is(err, target) }

func mustParse(t testing.TB, src string, opts ...Option) *Doc {
	t.Helper()
	d, err := Parse([]byte(src), opts...)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return d
}
