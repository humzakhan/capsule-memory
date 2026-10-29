package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

type statsView struct {
	Memories      int             `json:"memories"`
	Skills        int             `json:"skills"`
	Pending       int             `json:"pending_uploads"`
	CaptureStatus json.RawMessage `json:"capture_status"`
	Devices       []struct {
		ID         string `json:"device_id"`
		Revoked    bool   `json:"revoked"`
		ThisDevice bool   `json:"this_device"`
	} `json:"devices"`
	Scopes map[string]struct {
		Memories int `json:"memories"`
		Skills   int `json:"skills"`
		Pending  int `json:"pending_uploads"`
		Coverage struct {
			Cursor   string `json:"cursor"`
			Head     string `json:"head"`
			Complete bool   `json:"complete"`
		} `json:"coverage"`
	} `json:"scopes"`
}

func printStats(w io.Writer, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var view statsView
	if err := json.Unmarshal(raw, &view); err != nil {
		return err
	}
	var b strings.Builder
	const width = 68
	line := func(s string) {
		chars := []rune(s)
		if len(chars) == 0 {
			fmt.Fprintf(&b, "│%*s│\n", width, "")
			return
		}
		for len(chars) > 0 {
			n := len(chars)
			if n > width-4 {
				n = width - 4
			}
			part := string(chars[:n])
			fmt.Fprintf(&b, "│  %s%s│\n", part, strings.Repeat(" ", width-2-n))
			chars = chars[n:]
		}
	}
	section := func(title string) {
		fmt.Fprintf(&b, "├%s┤\n", strings.Repeat("─", width))
		line("")
		line(title)
		line("")
	}
	fmt.Fprintf(&b, "╭─ CAPSULE MEMORY %s╮\n", strings.Repeat("─", width-17))
	line("")
	line("THIS PROFILE")
	line("")
	line("SAVED MEMORIES                   PENDING UPLOADS")
	line(fmt.Sprintf("%-33d%d", view.Memories, view.Pending))
	line("Locally visible                  On this device")
	line("")
	section("CLOUDFLARE · ENCRYPTED ARCHIVE")
	line("Connection        Unknown (no network check)")
	line("Archive replay    See collection coverage below")
	line(fmt.Sprintf("Memories          %d locally visible", view.Memories))
	line(fmt.Sprintf("Skills            %d locally visible", view.Skills))
	line("")
	line("Local snapshot. Newer relay data may exist.")
	line("Sync each collection to check for updates.")
	line("")
	section("DEVICES · LAST KNOWN VAULT MEMBERSHIP")
	sort.Slice(view.Devices, func(i, j int) bool { return view.Devices[i].ID < view.Devices[j].ID })
	if len(view.Devices) == 0 {
		line("No devices available in local control state.")
	}
	for _, d := range view.Devices {
		state := "Enrolled · sync unknown"
		if d.Revoked {
			state = "Revoked"
		} else if d.ThisDevice {
			state = "This device"
		}
		line(d.ID)
		line("  " + state)
	}
	line("")
	line("Pending approvals  Unknown (no network check)")
	line("")
	section("PROFILE DETAILS · COLLECTIONS")
	ids := make([]string, 0, len(view.Scopes))
	for id := range view.Scopes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		line("No collections in this profile.")
	}
	for _, id := range ids {
		s := view.Scopes[id]
		state := "Incomplete or not yet scanned"
		if s.Coverage.Complete {
			state = "Complete at last scan"
		}
		line("Collection  " + id)
		line(fmt.Sprintf("Memories %d · Skills %d · Pending %d", s.Memories, s.Skills, s.Pending))
		line("Replay      " + state)
		line(fmt.Sprintf("Cursor %s / head %s", s.Coverage.Cursor, s.Coverage.Head))
		line("")
	}
	var capture struct {
		SourceStates map[string]int `json:"source_states"`
		Saved        int            `json:"saved_chunks"`
		Pending      int            `json:"pending_streams"`
	}
	if json.Unmarshal(view.CaptureStatus, &capture) == nil && capture.SourceStates != nil {
		line(fmt.Sprintf("Capture     %d saved chunks · %d pending streams", capture.Saved, capture.Pending))
		states := make([]string, 0, len(capture.SourceStates))
		for state := range capture.SourceStates {
			states = append(states, state)
		}
		sort.Strings(states)
		if len(states) == 0 {
			line("Sources     none configured")
		} else {
			for _, state := range states {
				line(fmt.Sprintf("Sources     %s: %d", state, capture.SourceStates[state]))
			}
		}
	} else {
		line("Capture     Not reported for this profile")
	}
	line("")
	line("Memory counts exclude skills and captured session chunks.")
	line("Offline writes on other devices may not be included.")
	line("")
	fmt.Fprintf(&b, "╰%s╯\n", strings.Repeat("─", width))
	_, err = io.WriteString(w, b.String())
	return err
}
