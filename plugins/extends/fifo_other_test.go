//go:build !unix

package extends

import "errors"

func mkfifo(string) error { return errors.New("unsupported") }
