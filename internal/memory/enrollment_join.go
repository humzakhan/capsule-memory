package memory

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/humzakhan/capsule-memory/internal/enrollment"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)

type joinState struct {
	Version    int                 `json:"version" cm:"required"`
	Invitation Signed              `json:"invitation" cm:"required"`
	Request    Signed              `json:"request" cm:"required"`
	Identity   enrollment.Identity `json:"identity" cm:"required"`
}

type enrollmentKeys struct {
	Activation    string                `json:"activation_secret_b64" cm:"required"`
	Version       int                   `json:"version" cm:"required"`
	Vault         string                `json:"vault_id" cm:"required"`
	Device        string                `json:"device_id" cm:"required"`
	Invite        string                `json:"invite_id" cm:"required"`
	RequestDigest string                `json:"request_sha256" cm:"required"`
	Keys          map[string]packageKey `json:"scope_keys" cm:"required"`
	History       []Signed              `json:"control_history" cm:"required"`
}

// JoinDevice performs one resumable exchange. A pending result has ready:false;
// it is not enrollment success. The CLI may poll under its own bounded context.
func JoinDevice(ctx context.Context, home, inviteFile string) (result map[string]any, resultErr error) {
	if err := privateDir(home, true); err != nil {
		return nil, err
	}
	lock, err := lockProfile(home, LockExclusive)
	if err != nil {
		return nil, err
	}
	defer func() {
		if lock != nil {
			resultErr = errors.Join(resultErr, unlockProfile(lock))
		}
	}()
	statePath := filepath.Join(home, "enrollment.json")
	var state joinState
	if _, err = os.Lstat(statePath); os.IsNotExist(err) {
		entries, err := os.ReadDir(home)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.Name() != "profile.lock" {
				return nil, errors.New("joining requires an unused private home; do not clone another active profile")
			}
		}
		if inviteFile == "" {
			return nil, errors.New("first join requires --invite-file; later joins resume the saved request")
		}
		var signed Signed
		if err = enrollment.ReadPrivate(inviteFile, 16<<10, &signed); err != nil {
			return nil, err
		}
		i, err := enrollment.ParseInvitation(signed)
		if err != nil {
			return nil, err
		}
		if err = enrollment.CheckLive(i, time.Now()); err != nil {
			return nil, err
		}
		identity, request, err := enrollment.NewIdentity(i)
		if err != nil {
			return nil, err
		}
		state = joinState{1, signed, request, identity}
		if err = enrollment.WritePrivate(statePath, state); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if err = enrollment.ReadPrivate(statePath, 32<<10, &state); err != nil || state.Version != 1 {
		return nil, enrollment.ErrInvalid
	}
	i, err := enrollment.ParseInvitation(state.Invitation)
	if err != nil {
		return nil, err
	}
	if inviteFile != "" {
		var supplied Signed
		if err = enrollment.ReadPrivate(inviteFile, 16<<10, &supplied); err != nil || supplied != state.Invitation {
			return nil, errors.New("saved join request belongs to a different invitation; preserve this home")
		}
	}
	key, err := state.Identity.Validate(i, state.Request)
	if err != nil {
		return nil, err
	}
	r, err := enrollment.ParseRequest(state.Request, i)
	if err != nil {
		return nil, err
	}
	client := enrollment.Client{Invitation: i}
	var receipt enrollment.Receipt
	if err = client.Call(ctx, "POST", "/requests", "", nil, enrollment.Submission{Invitation: state.Invitation, Request: state.Request}, &receipt); err != nil {
		return nil, err
	}
	if err = client.Call(ctx, "GET", "/requests/"+i.ID, r.Device, key, nil, &receipt); err != nil {
		return nil, err
	}
	result = map[string]any{"state": receipt.State, "ready": false, "device_id": r.Device, "request_id": i.ID, "fingerprint": enrollment.Fingerprint(state.Request)}
	if receipt.State == "pending" {
		return result, nil
	}
	if receipt.State != "approved" && receipt.State != "active" {
		return result, errors.New("enrollment is " + receipt.State + "; no new access was activated")
	}
	if receipt.Decision == nil || receipt.Manifest == nil {
		return nil, enrollment.ErrInvalid
	}
	d, err := enrollment.ParseDecision(*receipt.Decision, i, state.Request)
	if err != nil || d.Action != "approve" {
		return nil, enrollment.ErrInvalid
	}
	plain, err := enrollment.Open(i, state.Request, state.Identity, d.Package)
	if err != nil {
		return nil, err
	}
	var keys enrollmentKeys
	if strictjson.Decode(plain, enrollment.MaxPlaintext, &keys) != nil || keys.Version != 1 || keys.Vault != i.Vault || keys.Device != r.Device || keys.Invite != i.ID || keys.RequestDigest != enrollment.Digest(state.Request) || len(keys.Keys) != len(d.Scopes) {
		return nil, enrollment.ErrInvalid
	}
	challenge, err := enrollment.DecodeKey(keys.Activation, 32)
	if err != nil || enrollment.Hash(challenge) != d.ActivationDigest {
		return nil, enrollment.ErrInvalid
	}
	config := Config{Version: 1, Relay: i.Relay, Vault: i.Vault, Device: r.Device, OwnerPublic: i.Owner, DevicePrivate: state.Identity.Signing, Keys: map[string]ScopeKey{}, Loopback: i.Loopback, Bootstrap: keys.History}
	verifier := &Service{config: config}
	if len(keys.History) == 0 || len(keys.History) > 256 || d.Control == nil || keys.History[len(keys.History)-1] != *d.Control {
		return nil, enrollment.ErrInvalid
	}
	generation := "1"
	for _, signed := range keys.History {
		control, err := verifier.control(signed)
		if err != nil || control.Generation != generation {
			return nil, enrollment.ErrInvalid
		}
		generation = next(generation)
	}
	control, err := verifier.control(*receipt.Manifest)
	if err != nil {
		return nil, err
	}
	approved, err := verifier.control(*d.Control)
	if err != nil {
		return nil, err
	}
	if cmp, err := compare(control.Generation, approved.Generation); err != nil || cmp < 0 {
		return nil, enrollment.ErrInvalid
	}
	for _, scope := range d.Scopes {
		k, ok := keys.Keys[scope]
		device, epoch, err := control.grant(r.Device, scope)
		approvedDevice, approvedEpoch, approvedErr := approved.grant(r.Device, scope)
		if !ok || err != nil || approvedErr != nil || epoch != k.Epoch || approvedEpoch != k.Epoch || device.Public != r.Public || approvedDevice.Public != r.Public || len(device.Scopes) != len(d.Scopes) || len(approvedDevice.Scopes) != len(d.Scopes) || !validRole(k.Role) || len(k.Label) > 256 {
			return nil, enrollment.ErrInvalid
		}
		if _, err = enrollment.DecodeKey(k.Key, 32); err != nil {
			return nil, err
		}
		config.Keys[scope] = ScopeKey{k.Epoch, k.Key}
	}
	if _, err = os.Lstat(filepath.Join(home, "owner.json")); !os.IsNotExist(err) {
		return nil, errors.New("joining home contains owner material or is inaccessible")
	}
	if _, err = os.Lstat(filepath.Join(home, "owner-key.json")); !os.IsNotExist(err) {
		return nil, errors.New("joining home contains an owner credential reference or is inaccessible")
	}
	profile := filepath.Join(home, "device.json")
	if _, err = os.Lstat(profile); os.IsNotExist(err) {
		if err = enrollment.WritePrivate(profile, config); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else {
		existing, err := Load(home)
		if err != nil || existing.Vault != config.Vault || existing.Device != config.Device || existing.DevicePrivate != config.DevicePrivate || existing.OwnerPublic != config.OwnerPublic || existing.Relay != config.Relay || existing.Loopback != config.Loopback || len(existing.Keys) != len(config.Keys) {
			return nil, enrollment.ErrInvalid
		}
		for scope, k := range config.Keys {
			if existing.Keys[scope] != k {
				return nil, enrollment.ErrInvalid
			}
		}
	}
	if err = unlockProfile(lock); err != nil {
		return nil, err
	}
	lock = nil
	s, err := OpenWith(ctx, home, LockExclusive)
	if err != nil {
		return nil, err
	}
	err = s.Update(ctx, func(tx *sql.Tx) error {
		for scope, k := range keys.Keys {
			if _, err := tx.ExecContext(ctx, "INSERT INTO scope_labels VALUES(?,?,?) ON CONFLICT(scope) DO NOTHING", scope, k.Label, k.Role); err != nil {
				return ErrStorage
			}
		}
		return nil
	})
	err = errors.Join(err, s.Close())
	if err != nil {
		return nil, err
	}
	if err = client.Call(ctx, "POST", "/requests/"+i.ID+"/ack", r.Device, key, map[string]string{"decision_sha256": enrollment.Digest(*receipt.Decision), "activation_secret_b64": keys.Activation}, &receipt); err != nil {
		return result, err
	}
	if receipt.State != "active" {
		return nil, enrollment.ErrInvalid
	}
	result["state"] = "active"
	result["ready"] = true
	result["scope_ids"] = d.Scopes
	result["history_replayed"] = false
	return result, nil
}
