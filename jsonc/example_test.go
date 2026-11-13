package jsonc_test

import (
	"fmt"
	"os"

	"github.com/ubgo/textdata/jsonc"
)

// Example edits a commented config: only the touched bytes change.
func Example() {
	src := `{
  // Where to sync
  "sync": {
    "remote": "origin", // git remote name
    "interval": 30,
  },
}
`
	doc, err := jsonc.Parse([]byte(src))
	if err != nil {
		fmt.Println(err)
		return
	}
	_ = doc.Set("sync.interval", 60)
	_ = doc.SetAfter("remote", "sync.branch", "main")
	_ = doc.Disable("sync.interval")
	os.Stdout.Write(doc.Render())

	remote, _ := doc.GetString("sync.remote")
	note, _ := doc.Comment("sync.remote")
	fmt.Println(remote, "/", note)
	// Output:
	// {
	//   // Where to sync
	//   "sync": {
	//     "remote": "origin", // git remote name
	//     "branch": "main",
	//     // "interval": 60,
	//   },
	// }
	// origin / git remote name
}

// ExampleDoc_Expanded reads ${VAR} references through a lookup the caller
// chooses; the document itself keeps the references.
func ExampleDoc_Expanded() {
	doc, _ := jsonc.Parse([]byte(`{"url": "postgres://${HOST:-localhost}/app"}`),
		jsonc.WithLookup(func(name string) (string, bool) {
			return map[string]string{"HOST": "db.internal"}[name], name == "HOST"
		}))
	x, _ := doc.Expanded()
	url, _ := x.GetString("url")
	raw, _ := doc.GetString("url")
	fmt.Println(url)
	fmt.Println(raw)
	// Output:
	// postgres://db.internal/app
	// postgres://${HOST:-localhost}/app
}

// ExampleParse_errors shows the file:line:col error for a JSON5-ism.
func ExampleParse_errors() {
	_, err := jsonc.Parse([]byte("{\n  port: 8080\n}"), jsonc.WithFileName("config.jsonc"))
	fmt.Println(err)
	// Output:
	// config.jsonc:2:3: object keys must be double-quoted (unquoted keys are JSON5)
}
