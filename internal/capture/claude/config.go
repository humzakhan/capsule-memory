package claude

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/capture/filetail"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)

func config(raw json.RawMessage) (filetail.Config, error) {
	c := filetail.Config{IncludeSubagents: true}
	if strictjson.Decode(raw, 1<<20, &c) != nil {
		return c, capture.ErrInvalid
	}
	if c.Root == "" {
		root := os.Getenv("CLAUDE_CONFIG_DIR")
		if root == "" {
			home, e := os.UserHomeDir()
			if e != nil {
				return c, capture.ErrUnavailable
			}
			root = filepath.Join(home, ".claude")
		}
		c.Root = filepath.Join(root, "projects")
	}
	b, e := json.Marshal(c)
	if e != nil {
		return c, capture.ErrInvalid
	}
	return filetail.Parse(b)
}
