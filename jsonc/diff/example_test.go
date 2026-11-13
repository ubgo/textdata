package diff_test

import (
	"fmt"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/diff"
)

// snippet:start readme_Diff

// Example compares two configs by meaning: comments and layout do not count.
func Example() {
	a, _ := jsonc.Parse([]byte("{\n  // dev port\n  \"port\": 8080,\n  \"debug\": true,\n}"))
	b, _ := jsonc.Parse([]byte(`{"port": 9090, "hosts": ["a"]}`))
	changes, err := diff.Diff(a, b)
	if err != nil {
		panic(err)
	}
	for _, c := range changes {
		fmt.Printf("%s %s %v -> %v (a %s, b %s)\n", c.Kind, c.Path, c.Old, c.New, c.OldPos, c.NewPos)
	}
	// Output:
	// removed debug true -> <nil> (a 4:12, b -)
	// added hosts <nil> -> [a] (a -, b 1:25)
	// changed port 8080 -> 9090 (a 3:11, b 1:10)
}

// snippet:end readme_Diff
