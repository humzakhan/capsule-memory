package daemon

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"

	"github.com/humzakhan/capsule-memory/internal/app"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)
var safeName=regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
type ClientConfig struct{Version int `json:"version"`;Name string `json:"name"`;TokenSHA256 string `json:"token_sha256"`;Grant ops.Grant `json:"grant"`}
func privateDir(path string)error{if e:=os.MkdirAll(path,0700);e!=nil{return e};fi,e:=os.Lstat(path);if e!=nil||!fi.IsDir()||fi.Mode().Perm()!=0700{return memory.ErrInvalid};return nil}
func privateRead(path string,max int64)([]byte,error){fi,e:=os.Lstat(path);if e!=nil||!fi.Mode().IsRegular()||fi.Mode().Perm()!=0600||fi.Size()>max{return nil,memory.ErrInvalid};b,e:=os.ReadFile(path);if e!=nil{return nil,memory.ErrInvalid};return b,nil}
func writeNew(path string, body []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(body)
	var syncErr error
	if writeErr == nil {
		syncErr = file.Sync()
	}
	if err := errors.Join(writeErr, syncErr, file.Close()); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	syncErr = dir.Sync()
	return errors.Join(syncErr, dir.Close())
}
func Init(ctx context.Context, home string, profiles []app.ProfileRef) (map[string]any, error) {
	if len(profiles) == 0 || len(profiles) > 32 {
		return nil, memory.ErrInvalid
	}
	if err := privateDir(home); err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		if !safeName.MatchString(profile.Name) || !filepath.IsAbs(profile.Home) {
			return nil, memory.ErrInvalid
		}
	}
	encoded, err := app.EncodeConfig(app.DaemonConfig{Version: 1, Profiles: profiles})
	if err != nil {
		return nil, err
	}
	if err := writeNew(filepath.Join(home, "daemon.json"), encoded); err != nil {
		return nil, err
	}
	caps := []ops.Capability{
		ops.CapAdmin, ops.CapStatus, ops.CapMemoryRead, ops.CapMemoryWrite,
		ops.CapKnowledge, ops.CapSessionRead, ops.CapCandidateReview, ops.CapHint,
	}
	path, err := addClient(home, "admin", ops.Grant{Client: "admin", Caps: caps})
	return map[string]any{
		"config": filepath.Join(home, "daemon.json"), "admin_token_file": path, "installed": false,
	}, err
}

func AddClient(ctx context.Context, home, name string, grant ops.Grant) (string, error) {
	if name == "admin" || grant.Has(ops.CapAdmin) || !grant.Has(ops.CapStatus) {
		return "", ops.ErrForbidden
	}
	config, err := app.LoadDaemonConfig(home)
	if err != nil {
		return "", err
	}
	var ref *app.ProfileRef
	for _, profile := range config.Profiles {
		if profile.Name == grant.Profile {
			copy := profile
			ref = &copy
		}
	}
	if ref == nil {
		return "", memory.ErrInvalid
	}
	info, err := profileInfo(ctx, home, ref.Name, ref.Home)
	if err != nil {
		return "", err
	}
	for _, scope := range grant.Scopes {
		valid := false
		for _, available := range info.Scopes {
			if available.ID == scope {
				valid = true
				break
			}
		}
		if !valid {
			return "", ops.ErrForbidden
		}
	}
	for _, cap := range grant.Caps {
		switch cap {
		case ops.CapStatus, ops.CapMemoryRead, ops.CapMemoryWrite, ops.CapKnowledge,
			ops.CapSessionRead, ops.CapCandidateReview, ops.CapHint:
		default:
			return "", ops.ErrForbidden
		}
	}
	return addClient(home, name, grant)
}

func addClient(home, name string, grant ops.Grant) (string, error) {
	if !safeName.MatchString(name) {
		return "", memory.ErrInvalid
	}
	dir := filepath.Join(home, "clients")
	if err := privateDir(dir); err != nil {
		return "", err
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(token)
	sum := sha256.Sum256([]byte(encoded))
	grant.Client = name
	config := ClientConfig{
		Version: 1, Name: name, TokenSHA256: hex.EncodeToString(sum[:]), Grant: grant,
	}
	body, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+".token")
	if err := writeNew(path, []byte(encoded+"\n")); err != nil {
		return "", err
	}
	if err := writeNew(filepath.Join(dir, name+".json"), body); err != nil {
		return "", err
	}
	return path, nil
}

func Authenticate(home, name, token string) (ops.Grant, error) {
	if !safeName.MatchString(name) || len(token) > 128 {
		return ops.Grant{}, ops.ErrForbidden
	}
	body, err := privateRead(filepath.Join(home, "clients", name+".json"), 1<<20)
	if err != nil {
		return ops.Grant{}, ops.ErrForbidden
	}
	var config ClientConfig
	if strictjson.Decode(body, 1<<20, &config) != nil || config.Version != 1 || config.Name != name {
		return ops.Grant{}, ops.ErrForbidden
	}
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return ops.Grant{}, ops.ErrForbidden
	}
	sum := sha256.Sum256([]byte(token))
	want, err := hex.DecodeString(config.TokenSHA256)
	if err != nil || subtle.ConstantTimeCompare(sum[:], want) != 1 {
		return ops.Grant{}, ops.ErrForbidden
	}
	config.Grant.Client = name
	return config.Grant, nil
}

var ErrUnavailable = errors.New("capsule-memory daemon unavailable")
