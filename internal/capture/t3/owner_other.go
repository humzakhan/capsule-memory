//go:build !linux && !darwin

package t3

import "os"

func owned(os.FileInfo) bool { return false }
