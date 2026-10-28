package derive

import (
	"errors"
	"strings"
	"testing"

	"github.com/humzakhan/capsule-memory/internal/records"
)

// Prompt-only schema delivery must retain local authority checks even when the
// endpoint returns syntactically valid JSON without constraining its contents.
func TestModelJSONStillRequiresExactEvidenceAndKnownFields(t *testing.T) {
	known := map[string]citation{"event-1": {Ref: records.SourceRef{Record: "synthetic-chunk", Revision: "synthetic-revision"}, Role: "user"}}
	valid := `{"findings":[{"kind":"work","text":"Synthetic project uses amber badges.","evidence":["event-1"]}],"candidates":[],"gaps":[]}`
	if _, err := validate(valid, known); err != nil {
		t.Fatal("valid evidence rejected:", err)
	}
	for _, invalid := range []string{
		strings.ReplaceAll(valid, "event-1", "invented-event"),
		strings.Replace(valid, `"gaps":[]`, `"gaps":[],"execute":"untrusted command"`, 1),
		`{"findings":[],"candidates":[{"type":"memory","category":"choice","assertion":"Use amber badges.","basis":"explicit","decision_state":"proposed","verification":"none","evidence":["event-1"],"contradictions":[]}],"gaps":[]}`,
	} {
		if _, err := validate(invalid, known); !errors.Is(err, ErrOutput) {
			t.Fatal("unconstrained model JSON bypassed local validation:", err)
		}
	}
}
