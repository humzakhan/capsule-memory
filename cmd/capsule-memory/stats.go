package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

type statsView struct {
	CaptureStatus json.RawMessage `json:"capture_status"`
	Scopes        map[string]struct {
		Memories int `json:"memories"`
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
	fmt.Fprintln(&b, "+------------------------------------------------------------+")
	fmt.Fprintln(&b, "|                    CAPSULE MEMORY STATS                    |")
	fmt.Fprintln(&b, "+------------------------------------------------------------+")
	ids := make([]string, 0, len(view.Scopes))
	for id := range view.Scopes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		fmt.Fprintln(&b, "\n  No collections in this profile.")
	}
	for _, id := range ids {
		s := view.Scopes[id]
		state := "incomplete"
		if s.Coverage.Complete {
			state = "complete at last scan"
		}
		fmt.Fprintf(&b, "\n  COLLECTION  %s\n  Memories    %d\n  Pending     %d uploads\n  Replay      %s (cursor %s / head %s)\n", id, s.Memories, s.Pending, state, s.Coverage.Cursor, s.Coverage.Head)
	}
	var capture struct {
		SourceStates map[string]int `json:"source_states"`
		Saved        int            `json:"saved_chunks"`
		Pending      int            `json:"pending_streams"`
	}
	if json.Unmarshal(view.CaptureStatus, &capture) == nil && capture.SourceStates != nil {
		fmt.Fprintf(&b, "\n  CAPTURE     %d saved chunks, %d pending streams\n", capture.Saved, capture.Pending)
		states := make([]string, 0, len(capture.SourceStates))
		for state := range capture.SourceStates {
			states = append(states, state)
		}
		sort.Strings(states)
		if len(states) == 0 {
			fmt.Fprintln(&b, "  Sources     none configured")
		} else {
			for _, state := range states {
				fmt.Fprintf(&b, "  Sources     %s: %d\n", state, capture.SourceStates[state])
			}
		}
	} else {
		fmt.Fprintln(&b, "\n  CAPTURE     Not reported for this profile")
	}
	fmt.Fprintln(&b, "\n  Local snapshot only. Sync to check for newer relay data.")
	fmt.Fprintln(&b, "  Counts exclude skills and captured session chunks.")
	fmt.Fprintln(&b, "+------------------------------------------------------------+")
	_, err = io.WriteString(w, b.String())
	return err
}
