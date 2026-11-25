//go:build !unix

package microvm

import "errors"

func accessExec(string) error { return errors.New("microVMs run on Linux") }
