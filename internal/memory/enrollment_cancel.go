package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/humzakhan/capsule-memory/internal/enrollment"
)

type enrollmentCancellation struct {
	RequestDigest string `json:"request_sha256" cm:"required"`
	Parent        string `json:"parent_sha256" cm:"required"`
	Control       Signed `json:"control" cm:"required"`
}

// CancelEnrollment fences even a membership CAS whose response was lost. It
// does not depend on an expiring inbox receipt. A no-op successor occupies the
// old grant's generation when absent; an ordered revocation removes a granted
// target. No keys leave the owner. Exact cancellation bytes survive retries.
func (s *Service) CancelEnrollment(ctx context.Context) (enrollment.Receipt, error) {
	if s.lockMode != LockExclusive {
		return enrollment.Receipt{}, ErrProfileBusy
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var intent enrollmentIntent
	if err := s.getMeta(ctx, "enrollment_intent", &intent); err != nil {
		return enrollment.Receipt{}, errors.New("no prepared enrollment to cancel")
	}
	i, err := enrollment.ParseInvitation(intent.Invitation)
	if err != nil {
		return enrollment.Receipt{}, err
	}
	if i.Vault != s.configuration().Vault || i.Owner != s.configuration().OwnerPublic || i.Relay != s.configuration().Relay {
		return enrollment.Receipt{}, ErrInvalid
	}
	r, err := enrollment.ParseRequest(intent.Request, i)
	if err != nil {
		return enrollment.Receipt{}, err
	}
	d, err := enrollment.ParseDecision(intent.Decision, i, intent.Request)
	if err != nil || d.Control == nil || d.Action != "approve" {
		return enrollment.Receipt{}, ErrInvalid
	}
	wanted, err := s.control(*d.Control)
	if err != nil {
		return enrollment.Receipt{}, err
	}
	key, err := s.loadOwnerKey()
	if err != nil {
		return enrollment.Receipt{}, err
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		var remote Signed
		if err = s.request(ctx, "GET", s.basePath()+"/manifest", "", nil, &remote); err != nil {
			return enrollment.Receipt{}, err
		}
		current, err := s.control(remote)
		if err != nil {
			return enrollment.Receipt{}, err
		}
		local, localSigned, err := s.current(ctx)
		if err != nil {
			return enrollment.Receipt{}, err
		}
		if cmp, e := compare(current.Generation, local.Generation); e != nil || cmp < 0 || cmp == 0 && remote != localSigned {
			return enrollment.Receipt{}, ErrHeadsChanged
		}
		active := false
		for _, device := range current.Devices {
			if device.ID == r.Device && device.Public == r.Public && !device.Revoked {
				active = true
			}
		}
		cmp, err := compare(current.Generation, wanted.Generation)
		if err != nil {
			return enrollment.Receipt{}, err
		}
		if cmp >= 0 && !active {
			return s.finishEnrollmentCancellation(ctx, intent, remote, "")
		}
		var prepared enrollmentCancellation
		err = s.getMeta(ctx, "enrollment_cancel", &prepared)
		if err != nil && err != sql.ErrNoRows {
			return enrollment.Receipt{}, err
		}
		makeNew := err == sql.ErrNoRows
		if !makeNew {
			old, err := s.control(prepared.Control)
			if err != nil || prepared.RequestDigest != d.RequestDigest {
				return enrollment.Receipt{}, ErrInvalid
			}
			cmp, err := compare(current.Generation, old.Generation)
			if err != nil {
				return enrollment.Receipt{}, err
			}
			// A newer generation fences the prior attempt. Revoking an exact
			// target still active there is a new minimal control operation,
			// never a re-sign against an ambiguous unchanged parent.
			makeNew = cmp >= 0
			if !makeNew && (next(current.Generation) != old.Generation || enrollment.Digest(remote) != prepared.Parent) {
				return enrollment.Receipt{}, ErrHeadsChanged
			}
		}
		if makeNew {
			proposal := current
			proposal.Devices = append([]Device{}, current.Devices...)
			for n, device := range proposal.Devices {
				if device.ID == r.Device && device.Public == r.Public {
					proposal.Devices[n].Revoked = true
				}
			}
			proposal.Generation = next(current.Generation)
			signed, err := sign("agent-memory/control/v1", proposal, key)
			if err != nil {
				return enrollment.Receipt{}, err
			}
			prepared = enrollmentCancellation{d.RequestDigest, enrollment.Digest(remote), signed}
			b, err := json.Marshal(prepared)
			if err != nil {
				return enrollment.Receipt{}, err
			}
			s.wmu.Lock()
			err = s.enrollmentTransaction(ctx, true, func(tx *sql.Tx) error {
				if err := s.checkEnrollmentIntent(ctx, tx, intent); err != nil {
					return err
				}
				if err := s.checkGrantsQuery(ctx, tx, proposal); err != nil {
					return err
				}
				_, err := tx.ExecContext(ctx, "INSERT INTO meta(key,value) VALUES('enrollment_cancel',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", b)
				return err
			})
			s.wmu.Unlock()
			if err != nil {
				return enrollment.Receipt{}, err
			}
		}
		body, err := json.Marshal(prepared.Control)
		if err != nil {
			return enrollment.Receipt{}, err
		}
		var receipt struct {
			Cursor     string `json:"cursor"`
			Generation string `json:"generation"`
		}
		if err = s.request(ctx, "PUT", s.basePath()+"/manifest", "", body, &receipt); err != nil {
			lastErr = err
			continue
		}
		published, err := s.control(prepared.Control)
		if err != nil || receipt.Generation != published.Generation {
			return enrollment.Receipt{}, ErrInvalid
		}
		if cmp, e := compare(receipt.Cursor, "0"); e != nil || cmp <= 0 {
			return enrollment.Receipt{}, ErrInvalid
		}
		return s.finishEnrollmentCancellation(ctx, intent, prepared.Control, receipt.Cursor)
	}
	return enrollment.Receipt{}, errors.Join(lastErr, errors.New("cancellation outcome retained; use device resume before any new owner operation"))
}

func (s *Service) checkEnrollmentIntent(ctx context.Context, q Queryer, intent enrollmentIntent) error {
	var b []byte
	var saved enrollmentIntent
	if q.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='enrollment_intent'").Scan(&b) != nil || decode(b, &saved) != nil || saved.Decision != intent.Decision || saved.Request != intent.Request || saved.Invitation != intent.Invitation {
		return ErrEnrollmentPending
	}
	return nil
}

func (s *Service) finishEnrollmentCancellation(ctx context.Context, intent enrollmentIntent, control Signed, cursor string) (enrollment.Receipt, error) {
	if _, err := s.control(control); err != nil {
		return enrollment.Receipt{}, err
	}
	b, err := json.Marshal(control)
	if err != nil {
		return enrollment.Receipt{}, err
	}
	i, err := enrollment.ParseInvitation(intent.Invitation)
	if err != nil {
		return enrollment.Receipt{}, err
	}
	closed := s.closeCancelledRequest(ctx, intent, i)
	s.wmu.Lock()
	defer s.wmu.Unlock()
	err = s.enrollmentTransaction(ctx, true, func(tx *sql.Tx) error {
		if err := s.checkEnrollmentIntent(ctx, tx, intent); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE meta SET value=? WHERE key='current_control'", b); err != nil {
			return ErrStorage
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO meta(key,value) VALUES(?, 'true') ON CONFLICT(key) DO UPDATE SET value='true'", "enrollment_cancelled/"+i.ID); err != nil {
			return ErrStorage
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM meta WHERE key IN ('enrollment_intent','enrollment_cancel')")
		return err
	})
	if err != nil {
		return enrollment.Receipt{}, err
	}
	s.enrollmentPending.Store(false)
	return enrollment.Receipt{State: "cancelled", Cursor: cursor, Manifest: &control, RequestClosed: closed}, nil
}

// Notification is separate from the archive generation fence. An expired or
// unavailable inbox must never leave the owner permanently write-blocked after
// its old grant is provably fenced. Report whether a terminal denial was acked.
func (s *Service) closeCancelledRequest(ctx context.Context, intent enrollmentIntent, i enrollment.Invitation) bool {
	key, err := s.loadOwnerKey()
	if err != nil {
		return false
	}
	d := enrollment.Decision{Version: 1, Vault: i.Vault, Invite: i.ID, RequestDigest: enrollment.Digest(intent.Request), Action: "deny", Scopes: []string{}}
	signed, err := enrollment.Sign(enrollment.DecisionDomain, d, key)
	if err != nil {
		return false
	}
	var receipt enrollment.Receipt
	if s.enrollmentClient().Call(ctx, "POST", "/requests/"+i.ID+"/decision", "owner", key, signed, &receipt) != nil {
		return false
	}
	return receipt.State == "denied" && receipt.Decision != nil && *receipt.Decision == signed
}
