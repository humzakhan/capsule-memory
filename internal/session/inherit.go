package session

import (
	"context"
	"database/sql"
	"errors"
	"regexp"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/memory"
)

// Parent admission uses its observed path under the current mapping/exclusion
// policy. An old project id alone cannot override a later path exclusion.
func inheritedProject(ctx context.Context, q memory.Queryer, parent capture.Observation, policy Policy) (string, string, error) {
	key := SessionKey(parent.Origin)
	suppressed, secondary, e := suppression(ctx, q, key, parent.Origin)
	if e != nil {
		return "", "", e
	}
	if suppressed || secondary {
		return "excluded", "", nil
	}
	title := parent.Title
	if title == "" {
		_ = q.QueryRowContext(ctx, "SELECT json_extract(r.body,'$.body.title') FROM revisions r JOIN sessions s ON s.session=r.record WHERE s.session_key=? AND r.kind='session_manifest' AND json_extract(r.body,'$.operation')!='delete' ORDER BY r.rowid DESC LIMIT 1", key).Scan(&title)
	}
	for _, pattern := range policy.SessionExclusions {
		re := regexp.MustCompile(pattern)
		if re.MatchString(parent.Origin.NativeSessionID) || re.MatchString(title) {
			return "excluded", "", nil
		}
	}
	path := parent.CWD
	if path == "" {
		e = q.QueryRowContext(ctx, "SELECT path FROM origin_paths WHERE session_key=?", key).Scan(&path)
		if errors.Is(e, sql.ErrNoRows) {
			return "", "", nil
		}
		if e != nil {
			return "", "", e
		}
	}
	return projectFor(ctx, q, path)
}
