package selection

import (
	"reflect"
	"strings"
	"testing"
)

func TestSelectionProjections(t *testing.T) {
	t.Parallel()
	s := &Selection{
		Pairs:   []Pair{{"A", "1"}, {"B", "2"}, {"A", "3"}},
		Skipped: []Skip{{Name: "P", Reason: SkipPlaceholder}, {Name: "E", Reason: SkipEmpty}},
	}
	if got := s.Names(); !reflect.DeepEqual(got, []string{"A", "B", "A"}) {
		t.Errorf("Names = %v", got)
	}
	if got := s.SkippedNames(); !reflect.DeepEqual(got, []string{"P", "E"}) {
		t.Errorf("SkippedNames = %v", got)
	}
	if got := s.Map(); !reflect.DeepEqual(got, map[string]string{"A": "3", "B": "2"}) {
		t.Errorf("Map = %v (last wins)", got)
	}
	empty := &Selection{}
	if len(empty.Names()) != 0 || len(empty.SkippedNames()) != 0 || len(empty.Map()) != 0 {
		t.Error("empty selection projections")
	}
}

func TestPlaceholderMatcher(t *testing.T) {
	t.Parallel()
	re, err := PlaceholderMatcher("")
	if err != nil || !re.MatchString("__YOU__") || re.MatchString("_YOU_") || re.MatchString("x__YOU__") {
		t.Errorf("default matcher wrong: %v", err)
	}
	if again, _ := PlaceholderMatcher(DefaultPlaceholderPattern); again != re {
		t.Error("default pattern compiled twice")
	}
	custom, err := PlaceholderMatcher(`^CHANGEME$`)
	if err != nil || !custom.MatchString("CHANGEME") || custom.MatchString("__YOU__") {
		t.Errorf("custom matcher wrong: %v", err)
	}
	if _, err := PlaceholderMatcher("("); err == nil || !strings.Contains(err.Error(), `placeholder pattern "("`) {
		t.Errorf("bad pattern = %v", err)
	}
}
