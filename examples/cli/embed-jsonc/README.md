# embed-jsonc

Runs the jsonc verbs inside another Go program three ways: `jsonccmd.Execute` in-process under the host's own name (`WithProgramName`), `jsonccmd.Command` grafted into a host cobra tree as `myapp jsonc`, and the whole `textdatactl` umbrella through `ctl.Execute` and `ctl.Formats`.

Guide: [Embed the CLIs in your own program](../../../docs/guides/cli/embedding.md).

## Run it

```sh
cd examples/cli/embed-jsonc && go run .
```

Expected output:

```text
exit: 0
{
  // where the API listens
  "server": {"host": "localhost", "port": 9090}
}
9090
exit: 0
myapp jsonc: missing.jsonc: no such file
exit: 1
{"ok":true,"data":{"path":"server","kind":"object","value":{"host":"localhost","port":9090}}}
exit: 0
myapp jsonc: missing.jsonc: no such file
exit: 1
myapp: unknown command "nosuchverb" for "myapp jsonc"
exit: 2
formats: [dotenv jsonc]
localhost
exit: 0
textdatactl dotenv: key "PORT" not found in missing.env
exit: 1
```

`main_test.go` runs the program and checks this output.
