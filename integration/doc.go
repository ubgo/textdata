// Package integration holds the cross-format tests: one value plugin
// installed in a dotenv file and a jsonc file at once.
//
// It is its own module so that no library module requires a sibling only for
// its tests: each plugin under plugins/ requires the base alone, and the
// formats never require a plugin. Nothing imports this module and it is never
// tagged or released; it exists only in the workspace (go.work).
package integration
