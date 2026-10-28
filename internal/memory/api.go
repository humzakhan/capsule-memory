package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/humzakhan/capsule-memory/internal/frame"
	"github.com/humzakhan/capsule-memory/internal/records"
)

var (
	ErrNeedsMigration    = errors.New("profile uses local schema v1; run `capsule-memory profile migrate`")
	ErrUnsupportedRecord = errors.New("unsupported record schema or kind; replay stopped for this scope")
	ErrHeadsChanged      = errors.New("record heads changed during preparation; retry")
	ErrProfileBusy       = errors.New("profile is owned by the capsule-memory daemon or another process; use the daemon IPC client")
	ErrSyncBusy          = errors.New("scope sync busy")
	ErrMissingRevision   = errors.New("input revision not available locally")
)

type LockMode int

const (
	LockShared LockMode = iota
	LockExclusive
)

type ProfileInfo struct {
	Home, Vault, Device, Role string
	Scopes                    []ScopeInfo
	Schema                    int
	Owner                     bool
}
type ScopeInfo struct{ ID, Label, Role, Epoch string }

func NewID() (string, error) { return newID() }
func (s *Service) RequireV2() error {
	if s.schemaVersion != 2 {
		return ErrNeedsMigration
	}
	return nil
}
func (s *Service) Info(ctx context.Context) (ProfileInfo, error) {
	out := ProfileInfo{Home: s.home, Vault: s.configuration().Vault, Device: s.configuration().Device, Role: "memory", Schema: s.schemaVersion, Scopes: []ScopeInfo{}}
	_, e := s.ownerKey()
	out.Owner = e == nil
	for id, k := range s.configuration().Keys {
		sc := ScopeInfo{ID: id, Role: "default", Epoch: k.Epoch}
		if s.schemaVersion == 2 {
			e := s.reader.QueryRowContext(ctx, "SELECT label,role FROM scope_labels WHERE scope=?", id).Scan(&sc.Label, &sc.Role)
			if e != nil && e != sql.ErrNoRows {
				return out, ErrStorage
			}
			if sc.Role == "project" || sc.Role == "personal" || sc.Role == "ledger" {
				out.Role = "session"
			}
		}
		out.Scopes = append(out.Scopes, sc)
	}
	sort.Slice(out.Scopes, func(i, j int) bool { return out.Scopes[i].ID < out.Scopes[j].ID })
	return out, nil
}
func Migrate(ctx context.Context, home string) (map[string]any, error) {
	s, e := OpenWith(ctx, home, LockExclusive)
	if e != nil {
		return nil, e
	}
	defer s.Close()
	if s.schemaVersion == 2 {
		return map[string]any{"schema": 2, "migrated": false}, nil
	}
	path := filepath.Join(home, "memory.db.pre-v2."+strconv.FormatInt(time.Now().UnixNano(), 10))
	out := map[string]any{"snapshot": path, "migrated": false}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		return out, e
	}
	if e = f.Close(); e != nil {
		return out, e
	}
	if _, e = s.db.ExecContext(ctx, "VACUUM INTO '"+strings.ReplaceAll(path, "'", "''")+"'"); e != nil {
		return out, ErrStorage
	}
	f, e = os.OpenFile(path, os.O_RDWR, 0600)
	if e != nil {
		return out, e
	}
	e = f.Sync()
	f.Close()
	if e != nil {
		return out, e
	}
	if e = syncDir(home); e != nil {
		return out, e
	}
	e = s.Update(ctx, func(tx *sql.Tx) error { _, e := tx.ExecContext(ctx, schemaV2); return e })
	if e != nil {
		return out, e
	}
	s.schemaVersion = 2
	out["schema"] = 2
	out["migrated"] = true
	return out, nil
}
func (s *Service) Read(ctx context.Context, fn func(Queryer) error) error {
	tx, e := s.reader.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return ErrStorage
	}
	defer tx.Rollback()
	if e = fn(tx); e != nil {
		return e
	}
	if tx.Commit() != nil {
		return ErrStorage
	}
	return nil
}
func (s *Service) Update(ctx context.Context, fn func(*sql.Tx) error) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return s.updateLocked(ctx, fn)
}
func (s *Service) updateLocked(ctx context.Context, fn func(*sql.Tx) error) error {
	return s.enrollmentTransaction(ctx, false, fn)
}
func (s *Service) enrollmentTransaction(ctx context.Context, allowPending bool, fn func(*sql.Tx) error) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return ErrStorage
	}
	defer tx.Rollback()
	if !allowPending {
		// Reserve the writer before the guard read, avoiding a deferred
		// read-snapshot upgrade racing another legacy direct-mode writer.
		if _, e = tx.ExecContext(ctx, "UPDATE meta SET value=value WHERE key='current_control'"); e != nil {
			return ErrStorage
		}
		var n int
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM meta WHERE key='enrollment_intent'").Scan(&n); e != nil {
			return ErrStorage
		}
		if n != 0 {
			return ErrEnrollmentPending
		}
	}
	if e = fn(tx); e != nil {
		return e
	}
	if tx.Commit() != nil {
		return ErrStorage
	}
	return nil
}
func (s *Service) Heads(ctx context.Context, q Queryer, scope, record string) ([]Revision, bool, error) {
	if e := s.scope(scope); e != nil {
		return nil, false, e
	}
	a, e := recordRevisions(ctx, q, scope, record)
	if e != nil {
		return nil, false, e
	}
	h, d := state(a)
	return h, d, nil
}
func (s *Service) Revision(ctx context.Context, q Queryer, id string) (Revision, error) {
	var b []byte
	var r Revision
	if e := q.QueryRowContext(ctx, "SELECT body FROM revisions WHERE revision=?", id).Scan(&b); e != nil {
		if e == sql.ErrNoRows {
			return r, ErrMissingRevision
		}
		return r, ErrStorage
	}
	if decode(b, &r) != nil {
		return r, ErrStorage
	}
	if s.scope(r.Scope) != nil {
		return Revision{}, ErrInvalid
	}
	return r, nil
}
func (s *Service) Coverage(ctx context.Context, scope string) (Coverage, error) {
	return s.coverage(ctx, scope)
}

type PutInput struct {
	Scope, Kind, Record string
	Operation           string
	Parents             []string
	Restores            []string
	Text                string
	Body                any
	Provenance          Provenance
	Tags                []string
}
type Prepared struct {
	Revision        Revision
	Envelope        []byte
	headsAtPrepare  []string
	implicitParents bool
}

func headIDs(h []Revision) []string {
	a := make([]string, 0, len(h))
	for _, r := range h {
		a = append(a, r.ID)
	}
	sort.Strings(a)
	return a
}
func (s *Service) Prepare(ctx context.Context, in PutInput) (Prepared, error) {
	return s.prepare(ctx, in, nil)
}
func (s *Service) prepare(ctx context.Context, in PutInput, seal func(*Revision) error) (Prepared, error) {
	var out Prepared
	if e := s.scope(in.Scope); e != nil {
		return out, e
	}
	c, e := s.Control(ctx)
	if e != nil {
		return out, e
	}
	_, epoch, e := c.grant(s.configuration().Device, in.Scope)
	if e != nil || s.configuration().Keys[in.Scope].Epoch != epoch {
		return out, ErrInvalid
	}
	v := 1
	if records.V2Kinds[in.Kind] {
		v = 2
		if e = s.RequireV2(); e != nil {
			return out, e
		}
	} else if in.Kind != "memory" && in.Kind != "skill" {
		return out, ErrInvalid
	}
	if in.Record == "" {
		in.Record, e = newID()
		if e != nil {
			return out, e
		}
	}
	if !idPattern.MatchString(in.Record) {
		return out, ErrInvalid
	}
	var heads []Revision
	e = s.Read(ctx, func(q Queryer) error { var e error; heads, _, e = s.Heads(ctx, q, in.Scope, in.Record); return e })
	if e != nil {
		return out, e
	}
	for _, h := range heads {
		if h.Kind != in.Kind {
			return out, ErrInvalid
		}
	}
	out.implicitParents = in.Parents == nil
	if out.implicitParents {
		out.headsAtPrepare = headIDs(heads)
		in.Parents = append([]string{}, out.headsAtPrepare...)
	}
	if in.Operation == "" {
		in.Operation = "put"
	}
	if in.Operation == "put" && len(in.Parents) > 1 {
		in.Operation = "resolve"
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}
	id, e := newID()
	if e != nil {
		return out, e
	}
	op, e := newID()
	if e != nil {
		return out, e
	}
	r := Revision{Version: v, Vault: s.configuration().Vault, Scope: in.Scope, Record: in.Record, Kind: in.Kind, ID: id, Parents: in.Parents, Restores: in.Restores, OperationID: op, Operation: in.Operation, Author: s.configuration().Device, Created: time.Now().UTC().Format(time.RFC3339Nano), Provenance: in.Provenance, Tags: in.Tags}
	if r.Operation != "delete" {
		if v == 1 {
			r.Payload = &Payload{in.Text}
		} else {
			if seal != nil {
				e = seal(&r)
			} else {
				r.Body, e = json.Marshal(in.Body)
			}
			if e != nil {
				return out, e
			}
		}
	}
	if e = validateRevision(r); e != nil {
		return out, e
	}
	if e = s.verifyOwnerInsight(r); e != nil {
		return out, e
	}
	b, e := json.Marshal(r)
	if e != nil {
		return out, e
	}
	if v == 2 && len(b) > frame.ChunkHardMax {
		return out, frame.ErrTooLarge
	}
	out.Envelope, e = s.encryptPlain(r, b)
	if e != nil {
		return Prepared{}, e
	}
	out.Revision = r
	return out, nil
}
func (s *Service) Commit(ctx context.Context, set []Prepared, extra func(context.Context, *sql.Tx) error) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return s.commitLocked(ctx, set, extra)
}

// commitLocked owns one durable revision/outbox transaction. The caller owns wmu.
func (s *Service) commitLocked(ctx context.Context, set []Prepared, extra func(context.Context, *sql.Tx) error) error {
	return s.updateLocked(ctx, func(tx *sql.Tx) error {
		control, _, err := s.currentTx(ctx, tx)
		if err != nil {
			return err
		}
		for _, prepared := range set {
			revision := prepared.Revision
			_, epoch, err := control.grant(s.configuration().Device, revision.Scope)
			if err != nil || s.configuration().Keys[revision.Scope].Epoch != epoch || revision.Author != s.configuration().Device {
				return ErrInvalid
			}
			decoded, err := s.decrypt(prepared.Envelope, revision.Scope, control)
			if err != nil {
				return err
			}
			actual, err := json.Marshal(decoded)
			if err != nil {
				return ErrInvalid
			}
			expected, err := json.Marshal(revision)
			if err != nil || !equalBytes(actual, expected) {
				return ErrInvalid
			}
			var exists int
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM revisions WHERE revision=?", revision.ID).Scan(&exists); err != nil {
				return ErrStorage
			}
			if exists == 0 && prepared.implicitParents {
				heads, _, err := s.Heads(ctx, tx, revision.Scope, revision.Record)
				if err != nil {
					return err
				}
				if !equalStrings(headIDs(heads), prepared.headsAtPrepare) {
					return ErrHeadsChanged
				}
			}
			if err = insertRevision(ctx, tx, revision); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO outbox(revision,scope,envelope) VALUES(?,?,?) ON CONFLICT(revision) DO NOTHING", revision.ID, revision.Scope, prepared.Envelope); err != nil {
				return ErrStorage
			}
			if exists != 0 {
				continue
			}
			if err = s.apply(ctx, tx, revision, ApplyLocal); err != nil {
				return err
			}
			if revision.Version != 2 || revision.Operation == "delete" || (revision.Provenance.SourceKind != "derived" && revision.Provenance.SourceKind != "projection") {
				continue
			}
			if revision.Kind == records.KindProjection {
				var projection records.Projection
				if json.Unmarshal(revision.Body, &projection) != nil {
					return ErrInvalid
				}
				if projection.State == "withdrawn" || projection.State == "hidden" {
					continue
				}
			}
			refs := []records.SourceRef{{Record: revision.Record, Revision: revision.ID, Kind: revision.Kind}}
			if err = CheckConsent(ctx, tx, refs); err != nil {
				return err
			}
		}
		if extra != nil {
			return extra(ctx, tx)
		}
		return nil
	})
}

func equalBytes(a, b []byte) bool { return string(a) == string(b) }
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type ApplyOrigin int

const (
	ApplyLocal ApplyOrigin = iota
	ApplyReplay
)

type Applier interface {
	Apply(context.Context, *sql.Tx, Revision, ApplyOrigin) error
	Rebuild(context.Context, *sql.Tx) error
}
type RevisionHook func(context.Context, *sql.Tx, Revision) error

func (s *Service) OnRevision(h RevisionHook) { s.revisionHooks = append(s.revisionHooks, h) }
func (s *Service) RegisterApplier(kind string, a Applier) {
	if !records.V2Kinds[kind] || a == nil {
		panic("invalid applier")
	}
	if s.appliers[kind] != nil {
		panic("duplicate applier")
	}
	s.appliers[kind] = a
}
func (s *Service) apply(ctx context.Context, tx *sql.Tx, r Revision, origin ApplyOrigin) error {
	if r.Version == 2 {
		a := s.appliers[r.Kind]
		if a == nil {
			return ErrUnsupportedRecord
		}
		if e := a.Apply(ctx, tx, r, origin); e != nil {
			return e
		}
	}
	if s.schemaVersion == 2 {
		if e := s.RefreshStale(ctx, tx, []string{r.Record}); e != nil {
			return e
		}
		for _, h := range s.revisionHooks {
			if e := h(ctx, tx, r); e != nil {
				return e
			}
		}
	}
	return nil
}
func (s *Service) currentTx(ctx context.Context, q Queryer) (Control, Signed, error) {
	var b []byte
	var signed Signed
	if e := q.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='current_control'").Scan(&b); e != nil {
		return Control{}, signed, e
	}
	if e := decode(b, &signed); e != nil {
		return Control{}, signed, e
	}
	c, e := s.control(signed)
	return c, signed, e
}
func (s *Service) Control(ctx context.Context) (Control, error) {
	c, _, e := s.currentTx(ctx, s.reader)
	return c, e
}
func (s *Service) rebuildV2(ctx context.Context, tx *sql.Tx) error {
	for _, table := range []string{"deps", "retracted_events", "projects", "manifest_origins", "manifest_bindings", "sessions", "chunks", "bindings", "suppressions", "seen_events", "event_lookup", "candidate_keys", "projection_records", "ledger", "bridge_grants", "knowledge", "aggregation_grants", "session_search"} {
		if _, e := tx.ExecContext(ctx, "DELETE FROM "+table); e != nil {
			return ErrStorage
		}
	}
	for _, kind := range []string{records.KindProject, records.KindSessionManifest, records.KindSessionChunk, records.KindSuppression, records.KindSummary, records.KindInsight, records.KindKnowledge, records.KindProjection, records.KindBridgeGrant, records.KindAggregationGrant, records.KindJob} {
		if a := s.appliers[kind]; a != nil {
			if e := a.Rebuild(ctx, tx); e != nil {
				return e
			}
		} else {
			var n int
			if tx.QueryRowContext(ctx, "SELECT count(*) FROM revisions WHERE kind=?", kind).Scan(&n) != nil {
				return ErrStorage
			}
			if n > 0 {
				return ErrUnsupportedRecord
			}
		}
	}
	return s.RecomputeStale(ctx, tx)
}

// Config maps are immutable after publication. Administration clones the map
// before replacing it; a reader's snapshot remains valid without holding wmu.
func (s *Service) configuration() Config {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.config
}
func (s *Service) setConfig(c Config) { s.configMu.Lock(); defer s.configMu.Unlock(); s.config = c }
func coverageQuery(ctx context.Context, q Queryer, scope string) (Coverage, error) {
	c := Coverage{Cursor: "0", Head: "0", Basis: "last successful relay scan; offline freshness unknown"}
	e := q.QueryRowContext(ctx, "SELECT cursor,head FROM replay WHERE scope=?", scope).Scan(&c.Cursor, &c.Head)
	if e != nil && e != sql.ErrNoRows {
		return c, ErrStorage
	}
	c.Complete = e == nil && c.Cursor == c.Head
	return c, nil
}

// CheckLocalScope rechecks the device's grant and key epoch in this snapshot.
// Possession of a previously copied scope key is not live authorization.
func (s *Service) CheckLocalScope(ctx context.Context, q Queryer, scope string) error {
	c, _, e := s.currentTx(ctx, q)
	if e != nil {
		return e
	}
	cfg := s.configuration()
	_, epoch, e := c.grant(cfg.Device, scope)
	if e != nil {
		return ErrInvalid
	}
	key, ok := cfg.Keys[scope]
	if !ok || key.Epoch != epoch {
		return ErrInvalid
	}
	return nil
}

func (s *Service) UnrestoredDeletions(ctx context.Context, q Queryer, record string) ([]string, error) {
	rows, e := q.QueryContext(ctx, "SELECT revision FROM revisions d WHERE record=? AND json_extract(body,'$.operation')='delete' AND NOT EXISTS(SELECT 1 FROM restores x WHERE x.deletion=d.revision) ORDER BY revision LIMIT 4097", record)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			return nil, ErrStorage
		}
		out = append(out, id)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out) > 4096 {
		return nil, ErrInvalid
	}
	return out, nil
}

// SnapshotFence returns a request-local guard for a proof made in q. Call it
// after releasing other read snapshots. Any source revision, signed control or
// key-map replacement invalidates the proof; it never performs network I/O.
func (s *Service) SnapshotFence(ctx context.Context, q Queryer) (func(context.Context) bool, error) {
	var high, consentHigh int64
	var control, policy []byte
	if e := q.QueryRowContext(ctx, "SELECT COALESCE(max(rowid),0),(SELECT value FROM meta WHERE key='current_control'),COALESCE((SELECT max(rowid) FROM consent_roots),0),COALESCE((SELECT body FROM policy WHERE name='session'),X'') FROM revisions").Scan(&high, &control, &consentHigh, &policy); e != nil {
		return nil, e
	}
	cfg := s.configuration()
	return func(ctx context.Context) bool {
		current := s.configuration()
		if current.Device != cfg.Device || current.Vault != cfg.Vault || len(current.Keys) != len(cfg.Keys) {
			return false
		}
		for id, key := range cfg.Keys {
			if current.Keys[id] != key {
				return false
			}
		}
		same := false
		e := s.Read(ctx, func(q Queryer) error {
			var now, consentNow int64
			var signed, currentPolicy []byte
			if e := q.QueryRowContext(ctx, "SELECT COALESCE(max(rowid),0),(SELECT value FROM meta WHERE key='current_control'),COALESCE((SELECT max(rowid) FROM consent_roots),0),COALESCE((SELECT body FROM policy WHERE name='session'),X'') FROM revisions").Scan(&now, &signed, &consentNow, &currentPolicy); e != nil {
				return e
			}
			same = now == high && consentNow == consentHigh && equalBytes(control, signed) && equalBytes(policy, currentPolicy)
			return nil
		})
		return e == nil && same
	}, nil
}
