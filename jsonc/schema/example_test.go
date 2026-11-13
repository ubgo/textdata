package schema_test

import (
	"fmt"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/schema"
)

func ExampleValidate() {
	type Server struct {
		Port uint16 `json:"port" default:"8080" doc:"Listen port."`
	}
	type Config struct {
		Name   string `json:"name" env:"NAME,required"`
		Server Server `json:"server"`
	}
	s, err := schema.FromStruct(Config{})
	if err != nil {
		panic(err)
	}
	doc, _ := jsonc.Parse([]byte("{\n  \"server\": {\"port\": 70000},\n  \"tpyo\": 1\n}\n"), jsonc.WithFileName("config.jsonc"))
	res := schema.Validate(doc, s)
	for _, e := range res.Errors {
		fmt.Println("error:", e)
	}
	for _, w := range res.Warnings {
		fmt.Println("warning:", w)
	}
	// Output:
	// error: config.jsonc:1:1: (root): missing required key "name"
	// error: config.jsonc:2:22: server.port: must be <= 65535
	// warning: config.jsonc:3:3: tpyo: unknown key "tpyo"
}
