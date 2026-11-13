package main

import (
	"bytes"
	"testing"
)

// want is the program's exact output; the guide's "Try it" section shows it.
const want = `round trip exact: true
name: billing port: 8080 debug: false ratio: 0.75
GetInt(ratio) is ErrType: true
Get(servers[1]): map[host:b.internal]
keys: [name port debug ratio servers a/b]
servers: array of 2 | has timeout: false
GetString(timeout) is ErrNotFound: true
comment of name: "Public name of the service"
comment of servers: "upstream servers, tried in order"
comment of port: ""
paths: a.internal a.internal true slash key
port value at app.jsonc:4:11, key at 4:3
JSON() parses under Strict(): 2 servers
decoded: billing on 8080 via b.internal
strict: true
strict error: data.json:1:12: trailing comma is not allowed in strict JSON
ParseReader, Bytes equals Render: true
syntax error: app.jsonc:2:3: object keys must be double-quoted (unquoted keys are JSON5)
still renders input: true
Set is ErrInvalid: true
Read: billing 8080
`

func TestRun(t *testing.T) {
	var buf bytes.Buffer
	if err := run(&buf, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != want {
		t.Errorf("output changed:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
