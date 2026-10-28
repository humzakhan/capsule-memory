package codex

import (
	"context"
	"github.com/humzakhan/capsule-memory/internal/capture/filetail"
	"sort"
)

func previewVersions(ctx context.Context, c Config) []string {
	ss, _, e := filetail.Discover(ctx, tail(c), "", 64, accept)
	if e != nil {
		return nil
	}
	versions := map[string]bool{}
	for _, st := range ss {
		b, e := filetail.Header(c.Root, st.Key)
		if e != nil {
			continue
		}
		r, p, e := decode(b)
		if e != nil || r.Type != "session_meta" {
			continue
		}
		v := p.Version
		if v == "" {
			v = "unknown"
		}
		versions[v] = true
	}
	out := make([]string, 0, len(versions))
	for v := range versions {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
