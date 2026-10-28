package daemon_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/humzakhan/capsule-memory/internal/app"
	"github.com/humzakhan/capsule-memory/internal/daemon"
	"github.com/humzakhan/capsule-memory/internal/ops"
)

func TestInitializedAdminAuthenticatesWithoutScopeGrants(t *testing.T) {
	home := filepath.Join(t.TempDir(), "daemon")
	result, err := daemon.Init(context.Background(), home, []app.ProfileRef{
		{Name: "qa", Home: filepath.Join(t.TempDir(), "profile")},
	})
	if err != nil {
		t.Fatal(err)
	}
	token, err := os.ReadFile(result["admin_token_file"].(string))
	if err != nil {
		t.Fatal(err)
	}
	grant, err := daemon.Authenticate(home, "admin", strings.TrimSpace(string(token)))
	if err != nil {
		t.Fatal("generated admin credentials cannot authenticate:", err)
	}
	if !grant.Has(ops.CapAdmin) || len(grant.Scopes) != 0 {
		t.Fatal("admin authentication changed explicit capability or scope authority")
	}
	if _, err := daemon.Authenticate(home, "admin", "invalid-token"); !errors.Is(err, ops.ErrForbidden) {
		t.Fatal("invalid admin token was not rejected")
	}
}
