# jsonc: lint and validate

Lints a file that parses but has mistakes: a duplicate key, a tab-indented member, a comment between a key and its value, and (with `ForbidTrailingCommas`) a trailing comma. Shows `LintOnly`, the `Issue` fields and `AllLintRules`, then runs a `Validator` plugin of its own through `Doc.Validate`.

## Run it

```sh
cd examples/jsonc/lint && go run .
```

Expected output (`main_test.go` checks it on every test run):

```text
--- all rules, trailing commas forbidden ---
app.jsonc:3:3: duplicate key "port" (first at 2:3); the last one wins (duplicate-key)
app.jsonc:4:1: indented 1 tab, expected 2 spaces (the file indents by 2 spaces) (mixed-indent)
app.jsonc:5:11: comment between key "name" and its value; formatters and other tools may move or drop it (odd-comment)
app.jsonc:5:34: trailing comma after the last member (trailing-comma)
--- duplicate keys only ---
rule=duplicate-key line=3 col=3 msg="duplicate key \"port\" (first at 2:3); the last one wins"
rules available: [duplicate-key trailing-comma mixed-indent odd-comment]
validate: jsonc: plugin "require-owner": validate: missing required key owner
is errMissingOwner: true
after Set, validate: <nil>
```

## Learn more

- Guide: [lint and validate](../../../docs/guides/jsonc/lint-validate.md)
- Module README: [jsonc § Lint](../../../jsonc/README.md#lint)
