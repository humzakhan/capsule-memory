package session

import (
	"regexp"
	"sort"
	"strings"

	"github.com/humzakhan/capsule-memory/internal/records"
)

var redactRules = []struct{ class, pattern string }{
	{"private_key", `-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`},
	{"aws_key", `\b(AKIA|ASIA)[0-9A-Z]{16}\b`},
	{"github_token", `\b(gh[pousr]_[A-Za-z0-9]{36,255}|github_pat_[A-Za-z0-9_]{22,255})\b`},
	{"api_key", `\b(sk-(ant-|proj-)?[A-Za-z0-9_-]{20,}|xox[abposr]-[A-Za-z0-9-]{10,})\b`},
	{"jwt", `\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`},
	{"bearer", `(?i)\b(authorization:\s*bearer|bearer)\s+[A-Za-z0-9._~+/=-]{16,}`},
	{"assignment", `(?i)\b(password|passwd|secret|token|api[_-]?key)\s*[:=]\s*["']?[^\s"']{8,}`},
}

// Redact resolves overlaps in rule priority, then writes once so offsets refer to final text.
func Redact(event, text string, extra []string) (string, []records.Redaction) {
	text = strings.ToValidUTF8(text, "�")
	rules := append([]struct{ class, pattern string }{}, redactRules...)
	for _, p := range extra {
		rules = append(rules, struct{ class, pattern string }{"owner", p})
	}
	type match struct {
		start, end int
		class      string
	}
	var matches []match
	for _, rule := range rules {
		re := regexp.MustCompile(rule.pattern)
		for _, span := range re.FindAllStringIndex(text, -1) {
			if span[0] == span[1] {
				continue
			}
			overlap := false
			for _, m := range matches {
				if span[0] < m.end && span[1] > m.start {
					overlap = true
					break
				}
			}
			if !overlap {
				matches = append(matches, match{span[0], span[1], rule.class})
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].start < matches[j].start })
	var out strings.Builder
	var audit []records.Redaction
	last := 0
	for _, m := range matches {
		out.WriteString(text[last:m.start])
		replacement := "[REDACTED:" + m.class + "]"
		audit = append(audit, records.Redaction{Event: event, Offset: out.Len(), Length: len(replacement), Class: m.class})
		out.WriteString(replacement)
		last = m.end
	}
	out.WriteString(text[last:])
	return out.String(), audit
}
