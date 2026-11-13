# jsonc: arrays and comments

Edits the elements of a commented array with `Append`, `Insert` and `Remove`, appends a `jsonc.Raw` value whose own comment survives, and changes the comments attached to values with `SetComment` and `RemoveComment`. It prints the file after each step.

## Run it

```sh
cd examples/jsonc/arrays-comments && go run .
```

Expected output (`main_test.go` checks it on every test run):

```text
--- append and insert ---
{
  // Hosts tried in order
  "hosts": [
    "local",
    "a.internal", // primary
    "b.internal",
    "c.internal",
  ],
  "retries": 3,
}
--- remove ---
{
  // Hosts tried in order
  "hosts": [
    "local",
    "a.internal", // primary
    "c.internal",
  ],
  "retries": 3,
}
--- raw value ---
{
  // Hosts tried in order
  "hosts": [
    "local",
    "a.internal", // primary
    "c.internal",
    {"host": "d.internal" /* canary */},
  ],
  "retries": 3,
}
--- comments ---
{
  // Hosts tried in order, local first
  "hosts": [
    "local",
    "a.internal",
    "c.internal",
    {"host": "d.internal" /* canary */},
  ],
  // How often to retry
  // before giving up
  "retries": 3,
}
comment of retries: "How often to retry\nbefore giving up"
```

## Learn more

- Guide: [edit](../../../docs/guides/jsonc/edit.md)
- Module README: [jsonc § Editing](../../../jsonc/README.md#editing)
