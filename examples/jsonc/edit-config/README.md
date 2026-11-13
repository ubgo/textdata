# jsonc: edit and save a config

Edits a commented JSONC config with `Set`, `SetAfter`, `SetBefore`, `Delete`, `Disable` and `Restore`, and shows that every byte it did not touch (comments, the blank line, trailing commas, key order) survives. Then it saves: in place keeping the file's `0640` mode, a first-run file created `0600`, through a symlink that stays a symlink, `SaveAs` and `SetPath`, and a `SaveGuard` plugin (`guard.go`) that refuses to write a plaintext password. Everything happens in a temp folder.

## Run it

```sh
cd examples/jsonc/edit-config && go run .
```

Expected output (`main_test.go` checks it on every test run):

```text
--- after edits ---
{
  // Where to sync
  "sync": {
    "remote": "origin", // git remote name
    "branch": "main",
    // "interval": 60,
  },

  /* feature switches */
  "features": {
    "alpha": true,
  },
  "log": {
    "level": "info"
  },
}
restored interval: 60 | second Restore is ErrExists: true
saved app.jsonc, mode -rw-r-----
first run: existed=false, created new.jsonc mode -rw-------
{
  "port": 8080
}
link still a symlink: true | Path after SaveAs: link.jsonc
no path: jsonc: no path set; use Open, or SetPath before Save
after SetPath, saved from-bytes.jsonc
refused: jsonc: plugin "no-plaintext-secret": guard save guarded.jsonc: plaintext secret; write a ${scheme:ref} reference instead
is errPlaintextSecret: true | file written: false
```

## Learn more

- Guides: [edit](../../../docs/guides/jsonc/edit.md), [disable and restore](../../../docs/guides/jsonc/disable-restore.md), [save](../../../docs/guides/jsonc/save.md)
- Module README: [jsonc § Editing](../../../jsonc/README.md#editing), [§ Saving](../../../jsonc/README.md#saving)
