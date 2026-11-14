package schema

import (
	"fmt"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugin"
)

// PluginName is the name the plugin reports.
const PluginName = "schema"

// Checker validates a JSONC document against a schema. Installed with
// jsonc.WithPlugin it is both a plugin.Validator (doc.Validate() runs it on
// demand) and a plugin.SaveGuard (Save refuses a document with schema
// errors). Warnings never fail either; pass StrictKeys to make unknown keys
// errors, so both refuse a file with a typo'd key.
type Checker struct {
	schema *Schema
	opts   []Option
}

// Plugin returns a Validator and SaveGuard plugin for s. Warnings cannot
// travel through the returned error; pass OnWarning to receive them.
func Plugin(s *Schema, opts ...Option) *Checker {
	return &Checker{schema: s, opts: opts}
}

// Name implements plugin.Plugin.
func (c *Checker) Name() string { return PluginName }

// Validate implements plugin.Validator. A document that is not a *jsonc.Doc
// is parsed from its bytes (named by its Location). The error is nil, or a
// pos.ErrorList with every schema error.
func (c *Checker) Validate(doc plugin.Document) error {
	d, ok := doc.(*jsonc.Doc)
	if !ok {
		var err error
		d, err = jsonc.Parse(doc.Bytes(), jsonc.WithFileName(doc.Location()))
		if err != nil {
			return fmt.Errorf("schema: %w", err)
		}
	}
	res := Validate(d, c.schema, c.opts...)
	o := validateOptions{}
	for _, fn := range c.opts {
		fn(&o)
	}
	if o.onWarning != nil {
		for _, w := range res.Warnings {
			o.onWarning(w)
		}
	}
	return res.Err()
}

// GuardSave implements plugin.SaveGuard: Save refuses to write a document
// with schema errors and leaves the file untouched. Warnings (unknown keys,
// unless StrictKeys is passed) do not refuse; OnWarning still receives them.
func (c *Checker) GuardSave(doc plugin.Document) error { return c.Validate(doc) }

var (
	_ plugin.Validator = (*Checker)(nil)
	_ plugin.SaveGuard = (*Checker)(nil)
)
