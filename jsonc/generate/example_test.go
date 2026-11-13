package generate_test

import (
	"fmt"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/generate"
)

func ExampleNew() {
	f := generate.New(generate.SchemaURL("./app.schema.json"))
	f.Header("Created by `app init`. Edit freely.")
	srv := f.Object("server", generate.Doc("HTTP server"), generate.BlankBefore())
	srv.Set("port", 8080, generate.Trailing("default"))
	srv.Set("host", "0.0.0.0")
	f.Array("hosts").Add("a.example").Add("b.example")
	f.Set("raw", jsonc.Raw(`[1, /* two */ 2]`))
	b, err := f.Bytes()
	if err != nil {
		panic(err)
	}
	fmt.Print(string(b))
	// Output:
	// // Created by `app init`. Edit freely.
	// {
	//   "$schema": "./app.schema.json",
	//
	//   // HTTP server
	//   "server": {
	//     "port": 8080, // default
	//     "host": "0.0.0.0"
	//   },
	//   "hosts": [
	//     "a.example",
	//     "b.example"
	//   ],
	//   "raw": [1, /* two */ 2]
	// }
}
