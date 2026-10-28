package memory

import (
	"context"
	"fmt"
	"testing"
)

// Historical tombstones must not consume the public skill-list result limit.
func TestListSkillsAfterDeletedHistory(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	initialized, err := Init(ctx, home, "http://127.0.0.1:1", true)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	scope := initialized["scope_id"].(string)
	for i := 0; i < 100; i++ {
		record := fmt.Sprintf("a_skill_%03d", i)
		if _, err := svc.Write(ctx, WriteInput{Scope: scope, Kind: "skill", Record: record, Text: "synthetic", Source: "manual"}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Write(ctx, WriteInput{Scope: scope, Kind: "skill", Record: record, Operation: "delete", Source: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	const visible = "z_skill_visible"
	if _, err := svc.Write(ctx, WriteInput{Scope: scope, Kind: "skill", Record: visible, Text: "synthetic", Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	skills, err := svc.ListSkills(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].Record != visible {
		t.Fatalf("visible skill hidden by historical deletions: got %d results", len(skills))
	}
	if _, err := svc.Write(ctx, WriteInput{Scope: scope, Record: "a_skill_000", Operation: "restore", Text: "synthetic restored", TextSet: true, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	skills, err = svc.ListSkills(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 || skills[0].Record != "a_skill_000" || skills[1].Record != visible {
		t.Fatalf("explicitly restored skill remains hidden: got %d results", len(skills))
	}
}
