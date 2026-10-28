package memory

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// A one-record refresh must not scan unrelated archive revisions. Check the
// actual query plan rather than imposing a machine-dependent timing threshold.
func TestRefreshStaleUsesAffectedRecordLookups(t *testing.T) {
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "owner")
	if _, err := Init(ctx, home, "http://127.0.0.1:1", true); err != nil {
		t.Fatal(err)
	}
	svc, err := Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Update(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DROP INDEX revision_record")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	// Existing schema-v2 profiles receive the additive index on open.
	svc, err = Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	err = svc.Update(ctx, func(tx *sql.Tx) error {
		if err := svc.RefreshStale(ctx, tx, []string{"synthetic_missing_record"}); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, "EXPLAIN QUERY PLAN "+refreshInvalidSQL)
		if err != nil {
			return err
		}
		defer rows.Close()
		lookups := 0
		for rows.Next() {
			var node, parent, unused int
			var detail string
			if err := rows.Scan(&node, &parent, &unused, &detail); err != nil {
				return err
			}
			if detail == "SCAN r" || strings.HasPrefix(detail, "SCAN r ") {
				t.Errorf("refresh scans archive revisions: %s", detail)
			}
			if strings.HasPrefix(detail, "SEARCH r ") && strings.Contains(detail, "INDEX revision_record ") {
				lookups++
			}
		}
		if lookups < 2 {
			t.Errorf("affected record lookups missing: %d", lookups)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
}
