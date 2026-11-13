# embed-dotenvctl

Runs the dotenvctl verbs inside another Go program with `dotenvcmd.Execute`: a verb in-process with its exit code returned, the host's own name in failure lines (`WithProgramName`), and the verbs mounted as `myapp env` in a host cobra tree.

Guide: [Embed the CLIs in your own program](../../../docs/guides/cli/embedding.md).

## Run it

```sh
cd examples/cli/embed-dotenvctl && go run .
```

Expected output:

```text
http://localhost:8080
exit: 0
dotenvctl: key "PORT" not found in missing.env
exit: 1
myapp env: unknown command "nosuchverb" for "myapp env"
Run 'myapp env --help' for usage.
exit: 2
{"ok":true,"data":{"results":[{"key":"PORT","created":true}],"changed":true,"dry_run":false}}
exit: 0
myapp env: key "PORT" not found in missing.env
exit: 1
```

`main_test.go` runs the program and checks this output.
