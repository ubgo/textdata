package jsonc

import (
	"sync"
	"testing"
)

// TestPosConcurrentReads pins that position lookups are reads: the lazily
// built offset index must not race when several goroutines ask for positions
// on one unedited Doc (found by cfgkit's contrib/format-jsonc under -race).
func TestPosConcurrentReads(t *testing.T) {
	d, err := Parse([]byte("{\n  // c\n  \"a\": 1,\n  \"b\": [true, {\"c\": \"x\"}]\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := d.Clone().Pos("b[1].c")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				if p, err := d.Pos("b[1].c"); err != nil || p != want {
					t.Errorf("Pos = %v, %v; want %v", p, err, want)
					return
				}
				if _, err := d.KeyPos("a"); err != nil {
					t.Errorf("KeyPos: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
