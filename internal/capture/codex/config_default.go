package codex

import (
	"os"
	"path/filepath"
)

func defaultRoot() string {
	root := os.Getenv("CODEX_HOME")
	if root == "" {
		home, e := os.UserHomeDir()
		if e != nil {
			return ""
		}
		root = filepath.Join(home, ".codex")
	}
	return filepath.Join(root, "sessions")
}
