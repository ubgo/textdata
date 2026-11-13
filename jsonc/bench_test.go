package jsonc

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// benchDoc is a plain-JSON config of about 40 KB.
var benchDoc = func() []byte {
	var b strings.Builder
	b.WriteString("{\n")
	for i := 0; i < 400; i++ {
		fmt.Fprintf(&b, "  \"key%d\": {\"name\": \"value %d\", \"n\": %d, \"on\": true, \"tags\": [\"a\", \"b\", \"c\"]},\n", i, i, i)
	}
	b.WriteString("  \"last\": null\n}\n")
	return []byte(b.String())
}()

// BenchmarkParse vs BenchmarkEncodingJSONValid: parsing must stay within a
// small factor of encoding/json's own scan.
func BenchmarkParse(b *testing.B) {
	b.SetBytes(int64(len(benchDoc)))
	for i := 0; i < b.N; i++ {
		if _, err := Parse(benchDoc); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodingJSONValid(b *testing.B) {
	b.SetBytes(int64(len(benchDoc)))
	for i := 0; i < b.N; i++ {
		if !json.Valid(benchDoc) {
			b.Fatal("invalid")
		}
	}
}

// BenchmarkDecode vs BenchmarkEncodingJSONUnmarshal: a full read into a map.
func BenchmarkDecode(b *testing.B) {
	b.SetBytes(int64(len(benchDoc)))
	for i := 0; i < b.N; i++ {
		d, _ := Parse(benchDoc)
		var m map[string]any
		if err := d.Decode(&m); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodingJSONUnmarshal(b *testing.B) {
	b.SetBytes(int64(len(benchDoc)))
	for i := 0; i < b.N; i++ {
		var m map[string]any
		if err := json.Unmarshal(benchDoc, &m); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSet(b *testing.B) {
	d, _ := Parse(benchDoc)
	for i := 0; i < b.N; i++ {
		if err := d.Set("key200.n", i); err != nil {
			b.Fatal(err)
		}
	}
}
