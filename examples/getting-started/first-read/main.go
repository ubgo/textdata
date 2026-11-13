// Command first-read reads a dotenv file (app.env) and a JSONC file that sit next to it,
// first the one-call way (to a map), then through the document types that the
// rest of the library builds on. Run it from this folder: the paths are
// relative.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/jsonc"
)

// The sample files shipped with this example.
const (
	envPath    = "app.env"
	configPath = "config.jsonc"
)

// Config is the typed shape of config.jsonc; Decode fills it through
// encoding/json, so ordinary json tags apply.
//
//ds:def id=config-apr2nsth
type Config struct {
	Server struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"server"`
	Features []string `json:"features"`
}

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run executes every step of the guide in order.
func run(w io.Writer) error {
	for _, step := range []func(io.Writer) error{readEnvMap, openEnv, readJSONCMap, openJSONC} {
		if err := step(w); err != nil {
			return err
		}
	}
	return nil
}

// readEnvMap is the shortest read: every active key to its value, with
// references such as ${DB_HOST} already expanded. A missing file is an error.
//
//ds:def id=readenvmap-mv7e44qp
func readEnvMap(w io.Writer) error {
	values, err := dotenv.Read(envPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "dotenv.Read: DB_PORT=%s LOG_LEVEL=%s DATABASE_URL=%s\n", values["DB_PORT"], values["LOG_LEVEL"], values["DATABASE_URL"])
	return nil
}

// openEnv keeps the whole file, so you can ask what it holds, resolve
// references, and later edit and save it with its comments intact.
//
//ds:def id=openenv-mf8nqgu4
func openEnv(w io.Writer) error {
	f, err := dotenv.Open(envPath)
	if err != nil {
		return err
	}
	raw, _ := f.Get("DATABASE_URL") // as written: what Save would write back
	url, _, err := f.GetExpanded("DATABASE_URL")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "dotenv.Open: keys=%v\n  Get=%s\n  GetExpanded=%s\n  has API_SECRET=%v\n", f.Keys(), raw, url, f.Has("API_SECRET"))
	return nil
}

// readJSONCMap decodes a JSONC file into a map in one call; comments and
// trailing commas are allowed. Unlike dotenv.Read, jsonc.Read expands
// ${...} only when given jsonc.WithExpand().
//
//ds:def id=readjsoncmap-ap2395qz
func readJSONCMap(w io.Writer) error {
	m, err := jsonc.Read(configPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "jsonc.Read: server=%v\n", m["server"])
	return nil
}

// openJSONC reads single values by path, then decodes the whole document into
// a struct.
//
//ds:def id=openjsonc-vp5aznqj
func openJSONC(w io.Writer) error {
	doc, err := jsonc.Open(configPath)
	if err != nil {
		return err
	}
	port, err := doc.GetInt("server.port")
	if err != nil {
		return err
	}
	first, err := doc.GetString("features[0]")
	if err != nil {
		return err
	}
	var cfg Config
	if err := doc.Decode(&cfg); err != nil {
		return err
	}
	fmt.Fprintf(w, "jsonc.Open: server.port=%d features[0]=%s decoded=%+v\n", port, first, cfg)
	return nil
}
