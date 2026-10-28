//go:build !linux && !darwin

package daemon

import "errors"

func singleton(string) (func(), error) {
	return nil, errors.New("unsupported_platform: daemon requires Linux or macOS")
}
