package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)

type ingestGroup struct {
	scope, project, session, manifestRecord, branch string
	manifest records.SessionManifest
	events []records.Event
	redactions []records.Redaction
	omissions []records.Omission
}

type identity struct {
	position, id, digest string
}

// ingestNow is called with the module mutex held. Preparation performs local
// reads and encryption; commitCapture owns the single durable acknowledgement.
func (m *Module) ingestNow(ctx context.Context, src capture.Source, st capture.Stream, ch capture.Changes) (IngestResult, error) {
	result := IngestResult{}
	policy, err := m.captureSnapshot(ctx, src, ch.ObservedPolicyVersion)
	if err != nil {
		result.Blocked = "capture_snapshot_changed"
		return result, err
	}
	if ch.ObservedConsent == "" || ch.ObservedConsent != src.Consent {
		return result, memory.ErrLeaseLost
	}
	if ch.Quarantine != "" {
		result.Blocked = ch.Quarantine
		return result, m.quarantineCapture(ctx, src, st, ch, ch.Quarantine, 1)
	}
	blocked, err := m.captureDiskPressure(ctx)
	if err != nil {
		return result, err
	}
	if blocked {
		result.Blocked = "disk_floor"
		return result, m.MarkCaptureSource(ctx, src, "blocked", "disk_floor")
	}
	info, err := m.svc.Info(ctx)
	if err != nil {
		return result, err
	}
	var since string
	var remaining int64
	var historical int
	err = m.svc.Read(ctx, func(q memory.Queryer) error {
		err := q.QueryRowContext(ctx, `SELECT b.since,b.remaining,s.active FROM backfills b
			JOIN backfill_streams s ON s.source_id=b.source_id WHERE b.source_id=? AND s.stream=?`,
			src.ID, st.Key).Scan(&since, &remaining, &historical)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return result, err
	}
	if historical != 0 && ch.ScannedBytes > remaining {
		return result, errors.New("backfill_budget_exhausted")
	}
	gaps := append([]records.Omission{}, ch.Omissions...)
	observations := make(map[string]capture.Observation)
	for _, observation := range ch.Observations {
		observations[SessionKey(observation.Origin)] = observation
	}
	groups := make(map[string]*ingestGroup)
	var ids []identity
	pendingPositions := map[string]string{}
	pendingEvents := map[string]bool{}

	err = m.svc.Read(ctx, func(q memory.Queryer) error {
		expanded, err := expandT3Corrections(ctx, q, src, ch.Events)
		if err != nil {
			return err
		}
		for _, captured := range expanded {
			archived := false
			if ch.Reconcile {
				native := nativeIdentity(captured, ch.Next.Generation)
				if native != "" {
					var count int
					err := q.QueryRowContext(ctx, `SELECT count(*) FROM event_lookup ev JOIN sessions s
						ON s.session=ev.session WHERE s.session_key=? AND (ev.native_id=? OR ev.event_id=?)`,
						SessionKey(captured.Origin), native, native).Scan(&count)
					if err != nil {
						return err
					}
					archived = count > 0
				} else {
					old, err := archivedPosition(ctx, q, src, st.Key, captured.PositionKey, ch.Next.Generation)
					if err != nil {
						return err
					}
					archived = old != ""
				}
			}
			if !archived && captured.Event.Type != "correction" {
				boundary, reason := src.Consent, "future_baseline_excluded"
				if historical != 0 {
					boundary, reason = since, "backfill_time_excluded"
				}
				from, err := time.Parse(time.RFC3339Nano, boundary)
				if err != nil {
					return memory.ErrInvalid
				}
				when, err := time.Parse(time.RFC3339Nano, captured.Event.Time)
				if err != nil || when.Before(from) {
					gaps = append(gaps, records.Omission{Event: captured.PositionKey, Reason: reason})
					result.Omitted++
					continue
				}
			}
			key := SessionKey(captured.Origin)
			observation := observations[key]
			if observation.Origin.NativeSessionID == "" {
				observation = capture.Observation{Origin: captured.Origin, State: "unknown"}
			}
			if captured.CWD != "" {
				observation.CWD = captured.CWD
				observations[key] = observation
			}
			suppressed, secondary, err := suppression(ctx, q, key, captured.Origin)
			if err != nil {
				return err
			}
			if secondary {
				result.Blocked = "suppressed_match"
				return nil
			}
			excluded := suppressed
			for _, pattern := range policy.SessionExclusions {
				if pattern == captured.Origin.NativeSessionID || regexp.MustCompile(pattern).MatchString(observation.Title) {
					excluded = true
					break
				}
			}
			if excluded {
				gaps = append(gaps, records.Omission{Event: captured.PositionKey, Reason: "session_excluded"})
				result.Omitted++
				continue
			}
			cwd := captured.CWD
			if cwd == "" {
				cwd = observation.CWD
			}
			project, scope, err := projectFor(ctx, q, cwd)
			if captured.CorrectionScope != "" {
				if captured.Event.Type != "correction" || captured.Event.Text != "" || captured.ToolBody != "" {
					return memory.ErrInvalid
				}
				scope = captured.CorrectionScope
				err = q.QueryRowContext(ctx, "SELECT project_id FROM projects WHERE scope=? LIMIT 1", scope).Scan(&project)
			}
			if err != nil {
				return err
			}
			// A supplied but unmapped child path is not permission to inherit
			// the parent's project. Only missing child paths may inherit.
			if project == "" && cwd == "" && observation.Parent != nil {
				parent := observations[SessionKey(observation.Parent.Origin)]
				parent.Origin = observation.Parent.Origin
				project, scope, err = inheritedProject(ctx, q, parent, policy)
				if err != nil {
					return err
				}
			}
			if project == "excluded" || project == "" {
				reason := "ambiguous_project"
				if project == "excluded" {
					reason = "excluded_project"
				}
				gaps = append(gaps, records.Omission{Event: captured.PositionKey, Reason: reason})
				result.Omitted++
				continue
			}
			event := captured.Event
			event.Text = strings.ToValidUTF8(event.Text, "�")
			if event.ID == "" {
				event.ID = event.NativeID
			}
			if event.ID == "" {
				event.ID = "p:" + ch.Next.Generation + ":" + captured.PositionKey
			}
			if event.Type == "reasoning" || event.Type == "tool_call" || event.Type == "tool_result" {
				if policy.ToolDetail == "full" {
					var attachment bool
					event.Text, attachment = textToolBody(captured.ToolBody)
					if attachment {
						gaps = append(gaps, records.Omission{Event: event.ID, Reason: "attachment_excluded", Bytes: int64(len(captured.ToolBody))})
					}
				} else {
					event.Text = ""
					result.Omitted++
				}
			}
			recall, err := recallResult(ctx, q, captured, ch.Events)
			if err != nil {
				return err
			}
			if recall {
				event.Origin = "capsule_recall"
			}
			redactedText, redactions := Redact(event.ID, event.Text, policy.RedactionExtra)
			event.Text = redactedText
			event.ToolName, _ = Redact(event.ID, event.ToolName, policy.RedactionExtra)
			event.ToolStatus, _ = Redact(event.ID, event.ToolStatus, policy.RedactionExtra)
			native := nativeIdentity(captured, ch.Next.Generation)
			var correction *records.Event
			if event.ViewOf != "" {
				event.ViewOf, err = positionalView(ctx, q, src, st.Key, ch.Next.Generation, event.ViewOf, ids)
				if err != nil {
					return err
				}
			}
			if ch.Reconcile && native == "" {
				event, err = reconcilePosition(ctx, q, src, st.Key, captured.PositionKey, ch.Next.Generation, scope, event)
				if err != nil {
					return err
				}
			}
			if native != "" {
				event, correction, err = reconcileEvent(ctx, q, key, scope, native, event)
				if err != nil {
					return err
				}
			}
			for i := range redactions {
				redactions[i].Event = event.ID
			}
			content, err := json.Marshal(event)
			if err != nil {
				return memory.ErrInvalid
			}
			digest := hash(content)
			if prior, ok := pendingPositions[captured.PositionKey]; ok {
				if prior != digest {
					return ErrRewriteUnresolved
				}
				continue
			}
			pendingPositions[captured.PositionKey] = digest
			var old string
			err = q.QueryRowContext(ctx, `SELECT content_digest FROM event_ids
				WHERE source_id=? AND stream=? AND generation=? AND position_key=?`,
				src.ID, st.Key, ch.Next.Generation, captured.PositionKey).Scan(&old)
			if err == nil {
				if old == digest {
					continue
				}
				if native == "" {
					return ErrRewriteUnresolved
				}
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			ids = append(ids, identity{captured.PositionKey, event.ID, digest})
			var exists int
			err = q.QueryRowContext(ctx, `SELECT count(*) FROM seen_events e JOIN sessions s
				ON s.session=e.session WHERE s.session_key=? AND s.scope=? AND e.event_id=?`, key, scope, event.ID).Scan(&exists)
			if err != nil {
				return err
			}
			eventKey := key + ":" + scope + ":" + event.ID
			if exists > 0 || pendingEvents[eventKey] {
				continue
			}
			pendingEvents[eventKey] = true
			groupKey := key + ":" + scope
			group := groups[groupKey]
			if group == nil {
				group, err = m.captureGroup(ctx, q, src, st, info, policy, project, scope, observation)
				if err != nil {
					return err
				}
				groups[groupKey] = group
			}
			parts, err := splitEvent(event)
			if err != nil {
				group.omissions = append(group.omissions, records.Omission{Event: event.ID, Reason: "too_large", Bytes: int64(len(event.Text))})
				result.Omitted++
				continue
			}
			if correction != nil {
				group.events = append(group.events, *correction)
			}
			group.events = append(group.events, parts...)
			group.redactions = append(group.redactions, redactions...)
			if captured.ToolBody != "" && policy.ToolDetail != "full" {
				group.omissions = append(group.omissions, records.Omission{Event: event.ID, Reason: "tool_body_excluded", Bytes: int64(len(captured.ToolBody))})
			}
		}
		if ch.Reconcile && ch.CaughtUp && historical == 0 {
			if err := m.reconcileMissing(ctx, q, src, st, ch, ids, groups); err != nil {
				return err
			}
			ch.Next.Reconciling = false
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	if result.Blocked != "" {
		return result, m.quarantineCapture(ctx, src, st, ch, result.Blocked, 1)
	}
	prepared, err := m.prepareCapture(ctx, src, st, ch, info, groups, gaps, &result)
	if err != nil || result.Blocked != "" {
		return result, err
	}
	if err := m.commitCapture(ctx, src, st, ch, policy, historical != 0, prepared, observations, groups, ids, gaps); err != nil {
		return result, err
	}
	result.Checkpoint = ch.Next.Position
	if result.Events > 0 {
		m.Hint(src.ID, st.Key)
	}
	return result, nil
}

func suppression(ctx context.Context, q memory.Queryer, key string, origin records.Origin) (bool, bool, error) {
	var count int
	if err := q.QueryRowContext(ctx, "SELECT count(*) FROM suppressions WHERE session_key=?", key).Scan(&count); err != nil {
		return false, false, err
	}
	if count > 0 {
		return true, false, nil
	}
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM suppressions s JOIN revisions r ON r.revision=s.revision
		WHERE json_extract(r.body,'$.body.origin.harness')=? AND json_extract(r.body,'$.body.origin.native_session_id')=?`,
		origin.Harness, origin.NativeSessionID).Scan(&count)
	return false, count > 0, err
}
