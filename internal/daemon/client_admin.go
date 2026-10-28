package daemon

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/humzakhan/capsule-memory/internal/memory"
)

func profileInfo(ctx context.Context, daemonHome, profile, home string) (memory.ProfileInfo, error) {
	svc, e := memory.OpenWith(ctx, home, memory.LockShared)
	if e == nil {
		defer svc.Close()
		return svc.Info(ctx)
	}
	if !errors.Is(e, memory.ErrProfileBusy) {
		return memory.ProfileInfo{}, e
	}
	client, e := Dial(ctx, daemonHome, "admin", filepath.Join(daemonHome, "clients", "admin.token"))
	if e != nil {
		return memory.ProfileInfo{}, e
	}
	defer client.Close()
	client.Profile = profile
	var info memory.ProfileInfo
	e = client.Call(ctx, "admin.profile.info", map[string]any{}, &info)
	return info, e
}
