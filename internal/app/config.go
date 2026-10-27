package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/humzakhan/capsule-memory/internal/inference"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)
type ProfileRef struct{Name string `json:"name"`;Home string `json:"home"`}
type Schedule struct{CaptureFlushSeconds int `json:"capture_flush_seconds"`;PublishSeconds int `json:"publish_seconds"`;ReconcileSeconds int `json:"reconcile_seconds"`;JitterPercent int `json:"jitter_percent"`;ScanPage int `json:"scan_page"`}
type Storage struct{HighWatermarkBytes int64 `json:"high_watermark_bytes"`;LowWatermarkBytes int64 `json:"low_watermark_bytes"`;FreeSpaceFloorBytes int64 `json:"free_space_floor_bytes"`}
type DaemonConfig struct{
	Version int `json:"version"`
	Profiles []ProfileRef `json:"profiles"`
	Schedule Schedule `json:"schedule"`
	Storage Storage `json:"storage"`
	Inference inference.Config `json:"inference"`
	Derive struct{Idle string `json:"idle"`;WindowBytes int `json:"window_bytes"`} `json:"derive"`
}
func Defaults(c DaemonConfig)DaemonConfig{if c.Version==0{c.Version=1};if c.Schedule.CaptureFlushSeconds<=0{c.Schedule.CaptureFlushSeconds=5};if c.Schedule.PublishSeconds<=0{c.Schedule.PublishSeconds=30};if c.Schedule.ReconcileSeconds<=0{c.Schedule.ReconcileSeconds=300};if c.Schedule.ScanPage<=0{c.Schedule.ScanPage=64};if c.Schedule.JitterPercent==0{c.Schedule.JitterPercent=20};if c.Storage.HighWatermarkBytes>0&&c.Storage.LowWatermarkBytes<=0{c.Storage.LowWatermarkBytes=c.Storage.HighWatermarkBytes*8/10};if c.Storage.FreeSpaceFloorBytes<=0{c.Storage.FreeSpaceFloorBytes=1<<30};if c.Derive.Idle==""{c.Derive.Idle="15m"};return c}
func LoadDaemonConfig(home string)(DaemonConfig,error){var c DaemonConfig;fi,e:=os.Lstat(home);if e!=nil||!fi.IsDir()||fi.Mode().Perm()!=0700{return c,memory.ErrInvalid};path:=filepath.Join(home,"daemon.json");fi,e=os.Lstat(path);if e!=nil||!fi.Mode().IsRegular()||fi.Mode().Perm()!=0600||fi.Size()>1<<20{return c,memory.ErrInvalid};b,e:=os.ReadFile(path);if e!=nil||strictjson.Decode(b,1<<20,&c)!=nil||c.Version!=1||len(c.Profiles)==0||len(c.Profiles)>32{return c,memory.ErrInvalid};names:=map[string]bool{};homes:=map[string]bool{};for _,p:=range c.Profiles{if p.Name==""||names[p.Name]||homes[p.Home]||!filepath.IsAbs(p.Home){return c,memory.ErrInvalid};names[p.Name]=true;homes[p.Home]=true};c=Defaults(c);if c.Schedule.ScanPage>64||c.Schedule.JitterPercent<0||c.Schedule.JitterPercent>50{return c,memory.ErrInvalid};return c,nil}
func EncodeConfig(c DaemonConfig)([]byte,error){return json.MarshalIndent(Defaults(c),"","  ")}
