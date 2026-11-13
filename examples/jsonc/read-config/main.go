// Command read-config parses a commented JSONC config and reads it every way
// the jsonc package offers: typed getters, structure, comments, dotted paths
// and JSON Pointer, positions, plain JSON, a struct, and a one-call Read.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/textdata/jsonc"
)

// configName is the name errors and positions report for the parsed bytes.
const configName = "app.jsonc"

// configSrc is the file the example reads. It uses every JSONC extension:
// line and block comments and trailing commas.
//
//ds:def id=configsrc-38thfgz2
const configSrc = `{
  // Public name of the service
  "name": "billing",
  "port": 8080,
  "debug": false,
  "ratio": 0.75,
  /* upstream servers, tried in order */
  "servers": [
    {"host": "a.internal", "tags": {"x.y": true}},
    {"host": "b.internal"},
  ],
  "a/b": "slash key",
}
`

// plainJSON is the same kind of data with no JSONC extensions, for Strict.
const plainJSON = `{"name": "billing", "port": 8080}`

func main() {
	dir, err := os.MkdirTemp("", "read-config-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := run(os.Stdout, dir); err != nil {
		log.Fatal(err)
	}
}

// run executes every step in guide order; dir is a scratch folder for the
// one step that reads a real file.
func run(w io.Writer, dir string) error {
	doc, err := parseConfig(w)
	if err != nil {
		return err
	}
	steps := []func(io.Writer, *jsonc.Doc) error{
		readValues, inspectStructure, readComments, readPaths, readPositions, toPlainJSON, decodeStruct,
	}
	for _, step := range steps {
		if err := step(w, doc); err != nil {
			return err
		}
	}
	if err := parseStrict(w); err != nil {
		return err
	}
	if err := parseFromReader(w); err != nil {
		return err
	}
	if err := showSyntaxError(w); err != nil {
		return err
	}
	return readFile(w, dir)
}

// parseConfig parses the bytes and proves the round trip: Render returns the
// input byte for byte, comments and trailing commas included.
//
//ds:def id=parseconfig-673uqvd2
func parseConfig(w io.Writer) (*jsonc.Doc, error) {
	doc, err := jsonc.Parse([]byte(configSrc), jsonc.WithFileName(configName))
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(w, "round trip exact:", bytes.Equal(doc.Render(), []byte(configSrc)))
	return doc, nil
}

// readValues uses the typed getters. GetInt is exact: 0.75 would be ErrType,
// never a rounded 0 or 1.
//
//ds:def id=readvalues-cx6g68s8
func readValues(w io.Writer, doc *jsonc.Doc) error {
	name, err := doc.GetString("name")
	if err != nil {
		return err
	}
	port, err := doc.GetInt("port")
	if err != nil {
		return err
	}
	debug, err := doc.GetBool("debug")
	if err != nil {
		return err
	}
	ratio, err := doc.GetFloat("ratio")
	if err != nil {
		return err
	}
	_, err = doc.GetInt("ratio")
	fmt.Fprintln(w, "name:", name, "port:", port, "debug:", debug, "ratio:", ratio)
	fmt.Fprintln(w, "GetInt(ratio) is ErrType:", errors.Is(err, jsonc.ErrType))
	val, err := doc.Get("servers[1]")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Get(servers[1]): %v\n", val)
	return nil
}

// inspectStructure asks what is there before reading it. A missing path is
// ErrNotFound from every getter; Has is the check that never errors.
//
//ds:def id=inspectstructure-73xc6dju
func inspectStructure(w io.Writer, doc *jsonc.Doc) error {
	keys, err := doc.Keys("")
	if err != nil {
		return err
	}
	n, err := doc.Len("servers")
	if err != nil {
		return err
	}
	kind, err := doc.Kind("servers")
	if err != nil {
		return err
	}
	_, missing := doc.GetString("timeout")
	fmt.Fprintln(w, "keys:", keys)
	fmt.Fprintln(w, "servers:", kind, "of", n, "| has timeout:", doc.Has("timeout"))
	fmt.Fprintln(w, "GetString(timeout) is ErrNotFound:", errors.Is(missing, jsonc.ErrNotFound))
	return nil
}

// readComments reads the comment attached to a value: the lines directly
// above a member (either comment style), or one after it on the same line.
//
//ds:def id=readcomments-66jq6c8d
func readComments(w io.Writer, doc *jsonc.Doc) error {
	for _, p := range []string{"name", "servers", "port"} {
		c, err := doc.Comment(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "comment of %s: %q\n", p, c)
	}
	return nil
}

// readPaths shows both path spellings: dotted (quotes around a key holding a
// dot) and JSON Pointer, picked when the path starts with "/".
//
//ds:def id=readpaths-55tx8e6t
func readPaths(w io.Writer, doc *jsonc.Doc) error {
	dotted, err := doc.GetString("servers[0].host")
	if err != nil {
		return err
	}
	pointer, err := doc.GetString("/servers/0/host")
	if err != nil {
		return err
	}
	quoted, err := doc.GetBool(`servers[0].tags."x.y"`)
	if err != nil {
		return err
	}
	slash, err := doc.GetString("/a~1b")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "paths:", dotted, pointer, quoted, slash)
	return nil
}

// readPositions reports where a value and its key start, as file:line:col
// material for your own error messages.
//
//ds:def id=readpositions-q9u8htda
func readPositions(w io.Writer, doc *jsonc.Doc) error {
	val, err := doc.Pos("port")
	if err != nil {
		return err
	}
	key, err := doc.KeyPos("port")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "port value at %s:%s, key at %s\n", doc.FileName(), val, key)
	return nil
}

// toPlainJSON strips comments, trailing commas and a BOM for a consumer that
// only takes JSON; every other byte stays as written.
//
//ds:def id=toplainjson-fgubdyv8
func toPlainJSON(w io.Writer, doc *jsonc.Doc) error {
	out, err := doc.JSON()
	if err != nil {
		return err
	}
	sub, err := jsonc.Parse(out, jsonc.Strict())
	if err != nil {
		return err
	}
	servers, err := sub.Len("servers")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "JSON() parses under Strict():", servers, "servers")
	return nil
}

// Config is the typed shape the service wants; Decode fills it with
// encoding/json rules (json tags, unknown keys ignored).
//
//ds:def id=config-e43h99u8
type Config struct {
	Name    string `json:"name"`
	Port    int    `json:"port"`
	Servers []struct {
		Host string `json:"host"`
	} `json:"servers"`
}

// decodeStruct decodes the whole document into Config.
//
//ds:def id=decodestruct-xzsp4vpj
func decodeStruct(w io.Writer, doc *jsonc.Doc) error {
	var cfg Config
	if err := doc.Decode(&cfg); err != nil {
		return err
	}
	fmt.Fprintf(w, "decoded: %s on %d via %s\n", cfg.Name, cfg.Port, cfg.Servers[1].Host)
	return nil
}

// parseStrict parses plain JSON. Under Strict() a comment or trailing comma
// is an error, exactly as encoding/json.Valid would reject it.
//
//ds:def id=parsestrict-p6sb4csj
func parseStrict(w io.Writer) error {
	doc, err := jsonc.Parse([]byte(plainJSON), jsonc.Strict())
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "strict:", doc.IsStrict())
	_, err = jsonc.Parse([]byte(`{"a": [1, 2,]}`), jsonc.Strict(), jsonc.WithFileName("data.json"))
	fmt.Fprintln(w, "strict error:", err)
	return nil
}

// parseFromReader parses from any io.Reader (a request body, an embedded
// file). Bytes is what Save would write: the same bytes Render returns.
//
//ds:def id=parsefromreader-2uj8g78s
func parseFromReader(w io.Writer) error {
	doc, err := jsonc.ParseReader(strings.NewReader(configSrc), jsonc.WithFileName(configName))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "ParseReader, Bytes equals Render:", bytes.Equal(doc.Bytes(), doc.Render()))
	return nil
}

// showSyntaxError shows tolerant parsing: the Doc still comes back, renders
// its input unchanged, and refuses edits with ErrInvalid.
//
//ds:def id=showsyntaxerror-gsq5n4wh
func showSyntaxError(w io.Writer) error {
	src := []byte("{\n  port: 8080\n}")
	doc, err := jsonc.Parse(src, jsonc.WithFileName(configName))
	fmt.Fprintln(w, "syntax error:", err)
	fmt.Fprintln(w, "still renders input:", bytes.Equal(doc.Render(), src))
	fmt.Fprintln(w, "Set is ErrInvalid:", errors.Is(doc.Set("port", 1), jsonc.ErrInvalid))
	return nil
}

// readFile is the one-call shortcut: open a file and decode it into a map.
//
//ds:def id=readfile-gm56cbjv
func readFile(w io.Writer, dir string) error {
	path := filepath.Join(dir, configName)
	if err := os.WriteFile(path, []byte(configSrc), 0o600); err != nil {
		return err
	}
	m, err := jsonc.Read(path)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "Read:", m["name"], m["port"])
	return nil
}
