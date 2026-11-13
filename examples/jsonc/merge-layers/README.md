# jsonc: merge a base and an override

Deep-merges a local override file onto a commented base file with `Clone` and `Merge`: nested objects merge by key, arrays replace whole, a new key arrives with its comment, the base's comments stay. A second merge is a no-op, and a broken or non-object override is refused with the target unchanged.

## Run it

```sh
cd examples/jsonc/merge-layers && go run .
```

Expected output (`main_test.go` checks it on every test run):

```text
--- merged ---
{
  // HTTP listener
  "server": {
    "host": "0.0.0.0",
    "port": 9090,
  },
  "features": [
    "auth"
  ],
  // Local tracing collector
  "tracing": "http://localhost:4318",
}
base unchanged: true
second merge is a no-op: true
broken is ErrInvalid: true
array root is ErrType: true
target unchanged: true
```

## Learn more

- Guide: [merge and clone](../../../docs/guides/jsonc/merge-clone.md)
- Module README: [jsonc § Merge and Clone](../../../jsonc/README.md#merge-and-clone)
