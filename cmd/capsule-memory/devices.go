package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/humzakhan/capsule-memory/internal/app"
	"github.com/humzakhan/capsule-memory/internal/enrollment"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/mattn/go-isatty"
)

func runEnrollment(ctx context.Context, home, command string, args []string) (handled bool, resultErr error) {
	sub := ""
	if command == "device" || command == "owner" {
		if command == "device" && len(args) == 1 && (args[0] == "--help" || args[0] == "help" || args[0] == "-h") {
			fmt.Fprintln(os.Stdout, "Device management: invite, join, requests, approve, deny, resume, cancel, revoke.\nUse devices for the interactive owner console; device COMMAND --help for flags.\nLegacy create/grant/revoke-scope/import-keys remain available.")
			return true, nil
		}
		if len(args) == 0 {
			return false, nil
		}
		sub = args[0]
		args = args[1:]
		if command == "owner" && sub != "protect" {
			return false, nil
		}
		if command == "device" && sub != "invite" && sub != "join" && sub != "requests" && sub != "approve" && sub != "deny" && sub != "resume" && sub != "cancel" && sub != "revoke" {
			return false, nil
		}
	} else if command != "devices" {
		return false, nil
	}
	fs := flag.NewFlagSet(command+" "+sub, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&home, "home", home, "private profile directory")
	var scopes, id, fingerprint, out, invite string
	var asJSON, once, confirm bool
	var expires, wait time.Duration
	switch {
	case command == "devices":
		fs.BoolVar(&asJSON, "json", false, "print device and request status without secrets")
	case command == "owner":
	case sub == "invite":
		fs.StringVar(&scopes, "scopes", "", "explicit maximum collection IDs")
		fs.StringVar(&out, "out", "", "new private invitation file")
		fs.DurationVar(&expires, "expires", 10*time.Minute, "invitation lifetime (1m to 1h)")
	case sub == "join":
		fs.StringVar(&invite, "invite-file", "", "private owner invitation; omitted on resume")
		fs.BoolVar(&once, "once", false, "perform one exchange without waiting")
		fs.DurationVar(&wait, "wait", 10*time.Minute, "maximum interactive wait")
	case sub == "approve":
		fs.StringVar(&id, "id", "", "exact request ID")
		fs.StringVar(&fingerprint, "fingerprint", "", "full fingerprint verified on joining device")
		fs.StringVar(&scopes, "scopes", "", "explicit collection IDs to grant")
	case sub == "cancel":
		fs.BoolVar(&confirm, "confirm", false, "cancel pending enrollment; revoke it if already published")
	case sub == "deny" || sub == "revoke":
		fs.StringVar(&id, "id", "", "exact request/device ID")
		if sub == "revoke" {
			fs.BoolVar(&confirm, "confirm", false, "confirm revocation; old copies and keys remain")
		}
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintf(os.Stdout, "Usage: capsule-memory [--home PRIVATE_HOME] %s %s [flags]\n", command, sub)
		fs.SetOutput(os.Stdout)
		fs.PrintDefaults()
		return true, nil
	}
	if err := fs.Parse(args); err != nil || len(fs.Args()) != 0 {
		return true, errors.New("invalid device-management flags; see docs/device-enrollment.md")
	}
	print := func(v any) error { return json.NewEncoder(os.Stdout).Encode(v) }
	if sub == "join" {
		interactive := isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd()) && !once
		if !interactive {
			wait = 0
		}
		if wait < 0 || wait > time.Hour {
			return true, errors.New("join wait must be between 0 and 1h")
		}
		deadline := time.Now().Add(wait)
		shown := false
		for {
			result, err := memory.JoinDevice(ctx, home, invite)
			if err != nil {
				return true, err
			}
			if !interactive || result["ready"] == true {
				return true, print(result)
			}
			if !shown {
				fmt.Fprintf(os.Stdout, "Waiting for approval. Compare this full fingerprint on the owner laptop:\n%s\n", result["fingerprint"])
				shown = true
			}
			if !time.Now().Before(deadline) {
				return true, errors.New("still pending; run device join with the same home to resume")
			}
			timer := time.NewTimer(min(5*time.Second, time.Until(deadline)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return true, ctx.Err()
			case <-timer.C:
			}
		}
	}
	s, err := memory.OpenWith(ctx, home, memory.LockExclusive)
	if err != nil {
		return true, errors.Join(err, errors.New("use the private owner/admin profile, separate from daemon-owned profiles"))
	}
	defer func() { resultErr = errors.Join(resultErr, s.Close()) }()
	app.InstallModules(s)
	if command == "owner" {
		if err = s.ProtectOwner(ctx); err != nil {
			return true, err
		}
		return true, print(map[string]any{"owner_storage": "macos-keychain", "private_owner_file_removed": true})
	}
	if command == "devices" {
		if asJSON {
			v, err := deviceConsoleStatus(ctx, s)
			if err != nil {
				return true, err
			}
			return true, print(v)
		}
		if !isatty.IsTerminal(os.Stdin.Fd()) || !isatty.IsTerminal(os.Stdout.Fd()) {
			return true, errors.New("devices requires a terminal; use devices --json for noninteractive status")
		}
		return true, deviceConsole(ctx, s, home)
	}
	switch sub {
	case "invite":
		v, err := s.InviteDevice(ctx, csv(scopes), expires, out)
		if err != nil {
			return true, err
		}
		return true, print(v)
	case "requests":
		v, err := deviceConsoleStatus(ctx, s)
		if err != nil {
			return true, err
		}
		return true, print(v["requests"])
	case "approve", "deny", "resume", "cancel":
		var receipt enrollment.Receipt
		if sub == "approve" {
			receipt, err = s.ApproveDevice(ctx, id, fingerprint, csv(scopes))
		} else if sub == "deny" {
			receipt, err = s.DenyDevice(ctx, id)
		} else if sub == "cancel" {
			if !confirm {
				return true, errors.New("cancel requires --confirm; it may revoke the exact pending device")
			}
			receipt, err = s.CancelEnrollment(ctx)
		} else {
			receipt, err = s.ResumeEnrollment(ctx)
		}
		if err != nil {
			return true, err
		}
		result := map[string]any{"state": receipt.State, "cursor": receipt.Cursor, "device_ready": receipt.State == "active"}
		if receipt.State == "cancelled" {
			result["request_closed"] = receipt.RequestClosed
		}
		return true, print(result)
	case "revoke":
		if !confirm {
			return true, errors.New("revocation requires --confirm; downloaded copies remain and key rotation is manual")
		}
		v, err := s.RevokeDevice(ctx, id)
		if err != nil {
			return true, err
		}
		return true, print(v)
	}
	return true, errors.New("unknown enrollment operation")
}

// The relay's full signed invitations are bearer request capabilities. Console
// and JSON output deliberately project only public IDs, scope bounds and proof
// fingerprints, never invitation signatures, key packages or private profiles.
func deviceConsoleStatus(ctx context.Context, s *memory.Service) (map[string]any, error) {
	v, err := s.EnrollmentDevices(ctx)
	if err != nil {
		return nil, err
	}
	rows := []map[string]any{}
	for _, entry := range v["requests"].([]enrollment.Entry) {
		i, err := enrollment.ParseInvitation(entry.Invitation)
		if err != nil {
			return nil, err
		}
		row := map[string]any{"request_id": i.ID, "state": entry.State, "scope_ids": i.Scopes, "expires_at": i.Expires}
		if entry.Request != nil {
			r, err := enrollment.ParseRequest(*entry.Request, i)
			if err != nil {
				return nil, err
			}
			row["device_id"] = r.Device
			row["fingerprint"] = enrollment.Fingerprint(*entry.Request)
		}
		rows = append(rows, row)
	}
	v["requests"] = rows
	return v, nil
}

func deviceConsole(ctx context.Context, s *memory.Service, home string) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 4096)
	stop := context.AfterFunc(ctx, func() { _ = os.Stdin.Close() })
	defer stop()
	read := func(prompt string) (string, error) {
		fmt.Fprint(os.Stdout, prompt)
		if !scanner.Scan() {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			return "", errors.New("device console input ended or exceeded its limit")
		}
		return strings.TrimSpace(scanner.Text()), nil
	}
	for ctx.Err() == nil {
		status, err := deviceConsoleStatus(ctx, s)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, "\n+-- CAPSULE MEMORY / DEVICES ----------------------------------+")
		fmt.Fprintf(os.Stdout, "Vault: %s\n", status["vault_id"])
		deviceIDs := []string{}
		for n, d := range status["devices"].([]memory.Device) {
			deviceIDs = append(deviceIDs, d.ID)
			state := "enrolled"
			if d.Revoked {
				state = "revoked"
			}
			fmt.Fprintf(os.Stdout, "  [%d] %s  %s  collections=%s\n", n+1, d.ID, state, strings.Join(d.Scopes, ","))
		}
		collectionIDs := []string{}
		fmt.Fprintln(os.Stdout, "Collections:")
		for n, sc := range status["scopes"].([]memory.Scope) {
			collectionIDs = append(collectionIDs, sc.ID)
			fmt.Fprintf(os.Stdout, "  [%d] %s  %q\n", n+1, sc.ID, status["scope_labels"].(map[string]string)[sc.ID])
		}
		fmt.Fprintln(os.Stdout, "Requests:")
		requestIDs := []string{}
		for n, r := range status["requests"].([]map[string]any) {
			requestIDs = append(requestIDs, r["request_id"].(string))
			fmt.Fprintf(os.Stdout, "  [%d] %s  %s\n", n+1, r["request_id"], r["state"])
			if fp, ok := r["fingerprint"]; ok {
				fmt.Fprintf(os.Stdout, "    Verify on new device: %s\n", fp)
			}
		}
		if status["pending_operation"] == true {
			fmt.Fprintln(os.Stdout, "An approval needs recovery. Resume it, or deny that same request.")
		}
		fmt.Fprintln(os.Stdout, "[i] Invite  [a] Approve  [d] Deny  [v] Revoke  [u] Resume  [c] Cancel pending  [r] Refresh  [q] Quit")
		action, err := read("> ")
		if err != nil {
			return err
		}
		switch action {
		case "q":
			return nil
		case "r":
			continue
		case "i":
			selected, e := read("Maximum collections (numbers/IDs, comma-separated): ")
			if e != nil {
				return e
			}
			id, e := enrollment.RandomID()
			if e != nil {
				return e
			}
			out := filepath.Join(home, "invitation-"+id+".json")
			_, err = s.InviteDevice(ctx, consoleSelections(selected, collectionIDs), 10*time.Minute, out)
			if err == nil {
				fmt.Fprintf(os.Stdout, "Private invitation: %s\nTransfer only this invitation securely; it grants no archive access.\n", out)
			}
		case "a":
			id, e := read("Request number/ID: ")
			if e != nil {
				return e
			}
			id = consoleSelection(id, requestIDs)
			fp, e := read("Enter the FULL fingerprint from the joining device: ")
			if e != nil {
				return e
			}
			selected, e := read("Grant collections (numbers/IDs; read + write, existing data): ")
			if e != nil {
				return e
			}
			confirm, e := read("Type approve to authorize exactly this access: ")
			if e != nil {
				return e
			}
			if confirm != "approve" {
				continue
			}
			receipt, e := s.ApproveDevice(ctx, id, fp, consoleSelections(selected, collectionIDs))
			err = e
			if err == nil {
				fmt.Fprintf(os.Stdout, "Membership %s; device activation is separate.\n", receipt.State)
			}
		case "d":
			id, e := read("Request number/ID to deny permanently: ")
			if e != nil {
				return e
			}
			_, err = s.DenyDevice(ctx, consoleSelection(id, requestIDs))
		case "v":
			id, e := read("Device number/ID to revoke: ")
			if e != nil {
				return e
			}
			id = consoleSelection(id, deviceIDs)
			fmt.Fprintln(os.Stdout, "Future sync will stop. Old copies/keys remain; rotation is manual.")
			confirmation, e := read("Retype the full device ID to confirm: ")
			if e != nil {
				return e
			}
			if confirmation != id {
				continue
			}
			_, err = s.RevokeDevice(ctx, id)
		case "u":
			_, err = s.ResumeEnrollment(ctx)
		case "c":
			confirmation, e := read("Type cancel to block/revoke the exact pending enrollment: ")
			if e != nil {
				return e
			}
			if confirmation == "cancel" {
				_, err = s.CancelEnrollment(ctx)
			}
		default:
			fmt.Fprintln(os.Stdout, "Choose one of the listed actions.")
		}
		if err != nil {
			fmt.Fprintln(os.Stdout, "Action stopped:", err)
		}
	}
	return ctx.Err()
}

func consoleSelection(value string, ids []string) string {
	for _, id := range ids {
		if value == id {
			return value
		}
	}
	if n, err := strconv.Atoi(value); err == nil && n > 0 && n <= len(ids) {
		return ids[n-1]
	}
	return value
}
func consoleSelections(value string, ids []string) []string {
	selected := csv(value)
	for n, v := range selected {
		selected[n] = consoleSelection(v, ids)
	}
	return selected
}
