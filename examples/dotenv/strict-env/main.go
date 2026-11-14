// Command strict-expand reads a .env with dotenv.WithStrictExpand: references
// resolve as usual, and one that would otherwise read as "" (a typo'd
// variable, a scheme no plugin claims, a $(command) with commands off) fails
// with the file:line:col of the value and a typed expand error. The values
// are made up.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/expand"
)

// src is the .env being read: two resolvable values and three that strict
// mode refuses.
const src = `HOST=db.internal
DSN=postgres://${HOST}/${DB_NAME:-app}
# a typo: DB_HOTS is defined nowhere
REPLICA=postgres://${DB_HOTS}/app
TOKEN=${vault:secret/app#token}
STAMP=$(date +%s)
`

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run reads each key and reports what strict mode made of it.
//
//ds:def id=dotenv-strict-run-m2v8k5qa
func run(w io.Writer) error {
	f := dotenv.Parse(src, dotenv.WithStrictExpand())
	f.Path = ".env" // the name errors report
	for _, key := range []string{"DSN", "REPLICA", "TOKEN", "STAMP"} {
		v, _, err := f.GetExpanded(key)
		if err == nil {
			fmt.Fprintf(w, "%-8s %s\n", key, v)
			continue
		}
		if !errors.Is(err, expand.ErrStrict) {
			return err
		}
		fmt.Fprintf(w, "%-8s %v\n", key, err)
	}
	return nil
}
