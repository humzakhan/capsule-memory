//go:build !linux && !darwin

package filetail

import "os"

func Identity(os.FileInfo) string { return "-" }
