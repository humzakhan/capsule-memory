//go:build !darwin || !cgo

package ownerkey

import "errors"

var errUnavailable = errors.New("native owner Keychain requires macOS with cgo; existing private-file owner profiles remain supported")

func Get(string) ([]byte, error) { return nil, errUnavailable }
func Put(string, []byte) error   { return errUnavailable }
