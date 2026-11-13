// Package textdata is the shared base of the textdata formats: comment- and
// byte-preserving parsers and editors for human-edited config files.
//
// The root package holds no code; the base lives in its sub-packages:
//
//   - plugin: plugin interfaces, capability detection, named plugin errors
//   - expand: the ${VAR} / $(cmd) / ${scheme:ref} expansion engine
//   - atomicfile: temp file + fsync + rename saves that keep mode and symlinks
//   - pos: positions and file:line:col errors
//   - path: a."b.c"[2] paths and JSON Pointer for tree formats
//   - texttest: round-trip and untouched-bytes property harness
//
// The formats are separate modules (github.com/ubgo/textdata/dotenv,
// github.com/ubgo/textdata/jsonc) that import this one; the base never imports
// a format. The base is public API with a backward-compatibility promise:
// minimal version selection can pair any format version with a newer base, so
// changes here are additive only.
package textdata
