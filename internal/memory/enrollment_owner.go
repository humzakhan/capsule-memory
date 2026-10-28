package memory

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/humzakhan/capsule-memory/internal/enrollment"
)

var ErrEnrollmentPending = errors.New("enrollment outcome pending; use device resume, or device deny for that request before other writes")

type enrollmentIntent struct {
	Invitation Signed `json:"invitation" cm:"required"`
	Request    Signed `json:"request" cm:"required"`
	Decision   Signed `json:"decision" cm:"required"`
}

func (s *Service) enrollmentClient() enrollment.Client {
	c := s.configuration()
	return enrollment.Client{Invitation: enrollment.Invitation{Vault: c.Vault, Relay: c.Relay, Owner: c.OwnerPublic}}
}

func (s *Service) EnrollmentRequests(ctx context.Context) (enrollment.Inbox, error) {
	var out enrollment.Inbox
	key, err := s.loadOwnerKey()
	if err != nil {
		return out, err
	}
	err = s.enrollmentClient().Call(ctx, "GET", "/requests", "owner", key, nil, &out)
	if err != nil {
		return out, err
	}
	if len(out.Entries) > 32 {
		return out, ErrInvalid
	}
	for n := range out.Entries {
		entry := &out.Entries[n]
		i, err := enrollment.ParseInvitation(entry.Invitation)
		if err != nil || i.Vault != s.configuration().Vault || i.Owner != s.configuration().OwnerPublic || i.Relay != s.configuration().Relay {
			return enrollment.Inbox{}, ErrInvalid
		}
		if entry.Request != nil {
			if _, err = enrollment.ParseRequest(*entry.Request, i); err != nil {
				return enrollment.Inbox{}, err
			}
		}
		var cancelled int
		if err = s.reader.QueryRowContext(ctx, "SELECT count(*) FROM meta WHERE key=?", "enrollment_cancelled/"+i.ID).Scan(&cancelled); err != nil {
			return enrollment.Inbox{}, ErrStorage
		}
		if cancelled != 0 {
			entry.State = "cancelled"
		}
	}
	return out, nil
}

func (s *Service) enrollmentEntry(ctx context.Context, id string) (enrollment.Entry, error) {
	if !idPattern.MatchString(id) {
		return enrollment.Entry{}, ErrInvalid
	}
	box, err := s.EnrollmentRequests(ctx)
	if err != nil {
		return enrollment.Entry{}, err
	}
	for _, entry := range box.Entries {
		i, _ := enrollment.ParseInvitation(entry.Invitation)
		if i.ID == id {
			return entry, nil
		}
	}
	return enrollment.Entry{}, errors.New("enrollment request is absent; no action taken")
}

func (s *Service) refreshEnrollmentControl(ctx context.Context) (Control, Signed, error) {
	if err := s.publishControl(ctx); err != nil {
		return Control{}, Signed{}, err
	}
	var signed Signed
	if err := s.request(ctx, "GET", s.basePath()+"/manifest", "", nil, &signed); err != nil {
		return Control{}, signed, err
	}
	if err := s.rememberControl(ctx, signed); err != nil {
		return Control{}, signed, err
	}
	c, err := s.control(signed)
	return c, signed, err
}

func (s *Service) InviteDevice(ctx context.Context, scopes []string, ttl time.Duration, out string) (map[string]any, error) {
	if s.lockMode != LockExclusive || s.RequireV2() != nil {
		return nil, ErrProfileBusy
	}
	if !enrollment.ValidScopes(scopes) || ttl < time.Minute || ttl > time.Hour || out == "" {
		return nil, errors.New("invitation requires explicit --scopes, a new --out file and expiry between 1m and 1h")
	}
	key, err := s.ownerKey()
	if err != nil {
		return nil, err
	}
	c, _, err := s.refreshEnrollmentControl(ctx)
	if err != nil {
		return nil, err
	}
	for _, scope := range scopes {
		_, epoch, err := c.grant(s.configuration().Device, scope)
		if err != nil || s.configuration().Keys[scope].Epoch != epoch {
			return nil, ErrInvalid
		}
	}
	id, err := enrollment.RandomID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	i := enrollment.Invitation{Version: 1, Vault: c.Vault, ID: id, Relay: s.configuration().Relay, Owner: s.configuration().OwnerPublic, Scopes: scopes, Issued: strconv.FormatInt(now.Unix(), 10), Expires: strconv.FormatInt(now.Add(ttl).Unix(), 10), Loopback: s.configuration().Loopback}
	signed, err := enrollment.Sign(enrollment.InviteDomain, i, key)
	if err != nil {
		return nil, err
	}
	// The private invitation survives a lost registration reply and can be
	// registered by the joining device with these exact owner-signed bytes.
	if err = enrollment.WritePrivate(out, signed); err != nil {
		return nil, err
	}
	var receipt enrollment.Receipt
	if err = s.enrollmentClient().Call(ctx, "POST", "/invites", "", nil, signed, &receipt); err != nil {
		return nil, err
	}
	return map[string]any{"invite_id": id, "expires_at": i.Expires, "scope_ids": scopes, "invitation_file": out, "grants_access": false}, nil
}

// completeOwnerReplay refuses a grant unless every current scope is available
// and fully reconciled at one archive head. This is incremental replay, not a
// reread of old plaintext, and preserves private-history reader checks.
func (s *Service) completeOwnerReplay(ctx context.Context) (Control, Signed, string, error) {
	c, _, err := s.refreshEnrollmentControl(ctx)
	if err != nil {
		return c, Signed{}, "", err
	}
	head := ""
	for _, scope := range c.Scopes {
		k, ok := s.configuration().Keys[scope.ID]
		if !ok || k.Epoch != scope.Epoch {
			return c, Signed{}, "", errors.New("owner requires current keys and complete replay for every collection before enrollment")
		}
		result, err := s.SyncScope(ctx, scope.ID, SyncOptions{ReplayOnly: true})
		if err != nil {
			return c, Signed{}, "", err
		}
		if !result.CaughtUp || head != "" && head != result.Coverage.Head {
			return c, Signed{}, "", errors.New("archive changed or replay is incomplete; reconcile all collections and review again")
		}
		head = result.Coverage.Head
	}
	latest, signed, err := s.current(ctx)
	if err != nil || latest.Generation != c.Generation {
		return c, signed, "", ErrHeadsChanged
	}
	return latest, signed, head, nil
}

func (s *Service) enrollmentHistory(ctx context.Context, last Signed) ([]Signed, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT signed FROM controls ORDER BY length(generation),generation LIMIT 256")
	if err != nil {
		return nil, ErrStorage
	}
	defer rows.Close()
	history := []Signed{}
	generation := "1"
	for rows.Next() {
		var b []byte
		var signed Signed
		if rows.Scan(&b) != nil || decode(b, &signed) != nil {
			return nil, ErrStorage
		}
		c, err := s.control(signed)
		if err != nil || c.Generation != generation {
			return nil, errors.New("owner control history is incomplete; preserve profile and recover before enrollment")
		}
		history = append(history, signed)
		generation = next(generation)
	}
	if rows.Err() != nil {
		return nil, ErrStorage
	}
	control, err := s.control(last)
	if err != nil || control.Generation != generation || len(history) >= 256 {
		return nil, errors.New("owner control history is incomplete or at its enrollment bound")
	}
	return append(history, last), nil
}

func (s *Service) ApproveDevice(ctx context.Context, id, fingerprint string, scopes []string) (enrollment.Receipt, error) {
	if s.lockMode != LockExclusive || s.RequireV2() != nil {
		return enrollment.Receipt{}, ErrProfileBusy
	}
	var cancelled int
	if err := s.reader.QueryRowContext(ctx, "SELECT count(*) FROM meta WHERE key=?", "enrollment_cancelled/"+id).Scan(&cancelled); err != nil {
		return enrollment.Receipt{}, ErrStorage
	}
	if cancelled != 0 {
		return enrollment.Receipt{}, errors.New("request was cancelled by this owner; use a fresh invitation")
	}
	entry, err := s.enrollmentEntry(ctx, id)
	if err != nil {
		return enrollment.Receipt{}, err
	}
	i, err := enrollment.ParseInvitation(entry.Invitation)
	if err != nil || entry.Request == nil || entry.State != "pending" {
		return enrollment.Receipt{}, errors.New("only an exact pending request can be approved")
	}
	request, err := enrollment.ParseRequest(*entry.Request, i)
	if err != nil {
		return enrollment.Receipt{}, err
	}
	if strings.ToUpper(strings.TrimSpace(fingerprint)) != enrollment.Fingerprint(*entry.Request) {
		return enrollment.Receipt{}, errors.New("full joining-device fingerprint does not match; no approval prepared")
	}
	if !enrollment.ValidScopes(scopes) {
		return enrollment.Receipt{}, ErrInvalid
	}
	for _, scope := range scopes {
		if !contains(i.Scopes, scope) {
			return enrollment.Receipt{}, ErrInvalid
		}
	}
	key, err := s.ownerKey()
	if err != nil {
		return enrollment.Receipt{}, err
	}
	c, parent, head, err := s.completeOwnerReplay(ctx)
	if err != nil {
		return enrollment.Receipt{}, err
	}
	if err = enrollment.CheckLive(i, time.Now()); err != nil {
		return enrollment.Receipt{}, err
	}
	for _, d := range c.Devices {
		if d.ID == request.Device || d.Public == request.Public {
			return enrollment.Receipt{}, errors.New("device identity already exists; use a fresh invitation and identity")
		}
	}
	base := c.Generation
	c.Generation = next(c.Generation)
	c.Devices = append(c.Devices, Device{request.Device, request.Public, append([]string{}, scopes...), false})
	if err = validateControl(c, c.Vault); err != nil {
		return enrollment.Receipt{}, err
	}
	controlBody, err := json.Marshal(c)
	if err != nil || len(controlBody) > 128<<10 {
		return enrollment.Receipt{}, errors.New("membership exceeds relay control bound; no approval prepared")
	}
	control, err := sign("agent-memory/control/v1", c, key)
	if err != nil {
		return enrollment.Receipt{}, err
	}
	history, err := s.enrollmentHistory(ctx, control)
	if err != nil {
		return enrollment.Receipt{}, err
	}
	challenge := make([]byte, 32)
	if _, err = rand.Read(challenge); err != nil {
		return enrollment.Receipt{}, err
	}
	keys := enrollmentKeys{Activation: base64.StdEncoding.EncodeToString(challenge), Version: 1, Vault: c.Vault, Device: request.Device, Invite: i.ID, RequestDigest: enrollment.Digest(*entry.Request), Keys: map[string]packageKey{}, History: history}
	for _, scope := range scopes {
		k := s.configuration().Keys[scope]
		var label, role string
		if err = s.reader.QueryRowContext(ctx, "SELECT label,role FROM scope_labels WHERE scope=?", scope).Scan(&label, &role); err != nil {
			return enrollment.Receipt{}, ErrStorage
		}
		keys.Keys[scope] = packageKey{Epoch: k.Epoch, Key: k.Key, Label: label, Role: role}
	}
	plain, err := json.Marshal(keys)
	if err != nil {
		return enrollment.Receipt{}, err
	}
	ciphertext, err := enrollment.Seal(i, *entry.Request, plain)
	if err != nil {
		return enrollment.Receipt{}, err
	}
	decision := enrollment.Decision{ActivationDigest: enrollment.Hash(challenge), Version: 1, Vault: c.Vault, Invite: i.ID, RequestDigest: enrollment.Digest(*entry.Request), Action: "approve", Scopes: scopes, Parent: enrollment.Digest(parent), ArchiveCursor: head, Control: &control, Package: ciphertext}
	signed, err := enrollment.Sign(enrollment.DecisionDomain, decision, key)
	if err != nil {
		return enrollment.Receipt{}, err
	}
	intent := enrollmentIntent{entry.Invitation, *entry.Request, signed}
	b, err := json.Marshal(intent)
	if err != nil || len(b) > enrollment.MaxBody {
		return enrollment.Receipt{}, enrollment.ErrInvalid
	}
	s.wmu.Lock()
	err = s.withGrantAuthority(ctx, base, c, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, "INSERT INTO meta(key,value) VALUES('enrollment_intent',?)", b)
		return e
	})
	if err == nil {
		s.enrollmentPending.Store(true)
	} else {
		// Keep the in-process fence after an ambiguous local commit unless a
		// read conclusively shows that no durable reservation exists.
		var key string
		e := s.db.QueryRowContext(ctx, "SELECT key FROM meta WHERE key='enrollment_intent'").Scan(&key)
		s.enrollmentPending.Store(e != sql.ErrNoRows)
	}
	s.wmu.Unlock()
	if err != nil {
		return enrollment.Receipt{}, err
	}
	return s.ResumeEnrollment(ctx)
}

func (s *Service) DenyDevice(ctx context.Context, id string) (enrollment.Receipt, error) {
	if s.lockMode != LockExclusive {
		return enrollment.Receipt{}, ErrProfileBusy
	}
	entry, err := s.enrollmentEntry(ctx, id)
	if err != nil || entry.Request == nil {
		return enrollment.Receipt{}, errors.New("exact enrollment request is unavailable")
	}
	i, err := enrollment.ParseInvitation(entry.Invitation)
	if err != nil {
		return enrollment.Receipt{}, err
	}
	key, err := s.loadOwnerKey()
	if err != nil {
		return enrollment.Receipt{}, err
	}
	d := enrollment.Decision{Version: 1, Vault: i.Vault, Invite: i.ID, RequestDigest: enrollment.Digest(*entry.Request), Action: "deny", Scopes: []string{}}
	signed, err := enrollment.Sign(enrollment.DecisionDomain, d, key)
	if err != nil {
		return enrollment.Receipt{}, err
	}
	var receipt enrollment.Receipt
	err = s.enrollmentClient().Call(ctx, "POST", "/requests/"+id+"/decision", "owner", key, signed, &receipt)
	if err != nil {
		return receipt, err
	}
	if receipt.State != "denied" || receipt.Decision == nil || *receipt.Decision != signed {
		return receipt, ErrInvalid
	}
	var pending enrollmentIntent
	if err = s.getMeta(ctx, "enrollment_intent", &pending); err == sql.ErrNoRows {
		return receipt, nil
	} else if err != nil {
		return receipt, err
	}
	pi, e := enrollment.ParseInvitation(pending.Invitation)
	if e != nil || pi.ID != id {
		return receipt, ErrEnrollmentPending
	}
	return receipt, s.finishEnrollment(ctx, pending, receipt)
}

func (s *Service) ResumeEnrollment(ctx context.Context) (enrollment.Receipt, error) {
	if s.lockMode != LockExclusive {
		return enrollment.Receipt{}, ErrProfileBusy
	}
	var cancellation enrollmentCancellation
	if err := s.getMeta(ctx, "enrollment_cancel", &cancellation); err == nil {
		return s.CancelEnrollment(ctx)
	} else if err != sql.ErrNoRows {
		return enrollment.Receipt{}, err
	}
	var intent enrollmentIntent
	if err := s.getMeta(ctx, "enrollment_intent", &intent); err != nil {
		return enrollment.Receipt{}, errors.New("no prepared enrollment to resume")
	}
	i, err := enrollment.ParseInvitation(intent.Invitation)
	if err != nil {
		return enrollment.Receipt{}, err
	}
	if i.Vault != s.configuration().Vault || i.Owner != s.configuration().OwnerPublic || i.Relay != s.configuration().Relay {
		return enrollment.Receipt{}, ErrInvalid
	}
	key, err := s.loadOwnerKey()
	if err != nil {
		return enrollment.Receipt{}, err
	}
	var receipt enrollment.Receipt
	err = s.enrollmentClient().Call(ctx, "POST", "/requests/"+i.ID+"/decision", "owner", key, intent.Decision, &receipt)
	if err != nil {
		// An owner-signed terminal denial can resolve an ambiguous cancellation.
		var current enrollment.Receipt
		if e := s.enrollmentClient().Call(ctx, "GET", "/requests/"+i.ID, "owner", key, nil, &current); e != nil || current.State != "denied" {
			return receipt, err
		}
		receipt = current
	}
	if err = s.finishEnrollment(ctx, intent, receipt); err != nil {
		return receipt, err
	}
	if receipt.State == "conflicted" {
		return receipt, ErrHeadsChanged
	}
	return receipt, nil
}

func (s *Service) finishEnrollment(ctx context.Context, intent enrollmentIntent, receipt enrollment.Receipt) error {
	i, err := enrollment.ParseInvitation(intent.Invitation)
	if err != nil || receipt.Decision == nil {
		return ErrInvalid
	}
	d, err := enrollment.ParseDecision(*receipt.Decision, i, intent.Request)
	if err != nil {
		return err
	}
	if receipt.State != "denied" && receipt.State != "approved" && receipt.State != "active" && receipt.State != "revoked" && receipt.State != "conflicted" {
		return ErrEnrollmentPending
	}
	if receipt.State == "denied" && d.Action != "deny" {
		return ErrInvalid
	}
	if d.Action == "approve" && *receipt.Decision != intent.Decision {
		return ErrInvalid
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	err = s.enrollmentTransaction(ctx, true, func(tx *sql.Tx) error {
		var cancelCount int
		if tx.QueryRowContext(ctx, "SELECT count(*) FROM meta WHERE key='enrollment_cancel'").Scan(&cancelCount) != nil {
			return ErrStorage
		}
		if cancelCount != 0 {
			return ErrEnrollmentPending
		}
		var saved []byte
		var current enrollmentIntent
		if tx.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='enrollment_intent'").Scan(&saved) != nil || decode(saved, &current) != nil || current.Decision != intent.Decision {
			return ErrEnrollmentPending
		}
		if d.Action == "approve" {
			if d.Control == nil || receipt.Manifest == nil {
				return ErrInvalid
			}
			c, err := s.control(*d.Control)
			if err != nil {
				return err
			}
			latest, err := s.control(*receipt.Manifest)
			cmp, e := compare(latest.Generation, c.Generation)
			if err != nil || e != nil || cmp < 0 || cmp == 0 && *receipt.Manifest != *d.Control && receipt.State != "conflicted" {
				return ErrEnrollmentPending
			}
			if receipt.State == "conflicted" {
				if *receipt.Manifest == *d.Control {
					return ErrInvalid
				}
				b, e := json.Marshal(receipt.Manifest)
				if e != nil {
					return e
				}
				if _, e = tx.ExecContext(ctx, "UPDATE meta SET value=? WHERE key='current_control'", b); e != nil {
					return ErrStorage
				}
				_, e = tx.ExecContext(ctx, "DELETE FROM meta WHERE key IN ('enrollment_intent','enrollment_cancel')")
				return e
			}
			if cmp, e := compare(receipt.Cursor, "0"); e != nil || cmp <= 0 {
				return ErrInvalid
			}
			if err = s.checkGrantsQuery(ctx, tx, c); err != nil {
				return err
			}
			if err = s.saveControl(ctx, tx, c, *d.Control); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, "UPDATE controls SET receipt=? WHERE generation=?", receipt.Cursor, c.Generation); err != nil {
				return ErrStorage
			}
			if cmp > 0 {
				b, e := json.Marshal(receipt.Manifest)
				if e != nil {
					return e
				}
				if _, e = tx.ExecContext(ctx, "UPDATE meta SET value=? WHERE key='current_control'", b); e != nil {
					return ErrStorage
				}
			}
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM meta WHERE key IN ('enrollment_intent','enrollment_cancel')")
		return err
	})
	if err == nil {
		s.enrollmentPending.Store(false)
	}
	return err
}

func (s *Service) RevokeDevice(ctx context.Context, id string) (map[string]any, error) {
	if s.lockMode != LockExclusive || !idPattern.MatchString(id) {
		return nil, ErrInvalid
	}
	key, err := s.ownerKey()
	if err != nil {
		return nil, err
	}
	c, _, err := s.refreshEnrollmentControl(ctx)
	if err != nil {
		return nil, err
	}
	if id == s.configuration().Device {
		return nil, errors.New("owner console cannot revoke its own device")
	}
	base := c.Generation
	found := false
	for n, d := range c.Devices {
		if d.ID == id {
			if d.Revoked {
				return map[string]any{"device_id": id, "published": true, "revoked": true, "rotation": "manual"}, nil
			}
			c.Devices[n].Revoked = true
			found = true
		}
	}
	if !found {
		return nil, ErrInvalid
	}
	c.Generation = next(c.Generation)
	signed, err := sign("agent-memory/control/v1", c, key)
	if err != nil {
		return nil, err
	}
	s.wmu.Lock()
	err = s.withGrantAuthority(ctx, base, c, func(tx *sql.Tx) error { return s.saveControl(ctx, tx, c, signed) })
	s.wmu.Unlock()
	if err != nil {
		return nil, err
	}
	if err = s.publishControl(ctx); err != nil {
		return nil, errors.Join(err, errors.New("revocation retained locally; sync owner to confirm publication"))
	}
	return map[string]any{"device_id": id, "published": true, "revoked": true, "old_copies_retained": true, "rotation": "manual"}, nil
}

func (s *Service) EnrollmentDevices(ctx context.Context) (map[string]any, error) {
	box, err := s.EnrollmentRequests(ctx)
	if err != nil {
		return nil, err
	}
	var signed Signed
	if err = s.request(ctx, "GET", s.basePath()+"/manifest", "", nil, &signed); err != nil {
		return nil, err
	}
	c, err := s.control(signed)
	if err != nil {
		return nil, err
	}
	var pending int
	if err = s.reader.QueryRowContext(ctx, "SELECT count(*) FROM meta WHERE key='enrollment_intent'").Scan(&pending); err != nil {
		return nil, ErrStorage
	}
	scopes := append([]Scope{}, c.Scopes...)
	sort.Slice(scopes, func(a, b int) bool { return scopes[a].ID < scopes[b].ID })
	labels := map[string]string{}
	for _, scope := range scopes {
		var label string
		if err = s.reader.QueryRowContext(ctx, "SELECT label FROM scope_labels WHERE scope=?", scope.ID).Scan(&label); err != nil && err != sql.ErrNoRows {
			return nil, ErrStorage
		}
		labels[scope.ID] = label
	}
	return map[string]any{"vault_id": c.Vault, "device_id": s.configuration().Device, "devices": c.Devices, "scopes": scopes, "scope_labels": labels, "requests": box.Entries, "pending_operation": pending != 0}, nil
}
