package capture

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/humzakhan/capsule-memory/internal/records"
)

type Source struct {
	ID, Harness, Adapter, StoreIdentity string
	Consent                             string
	Config                              json.RawMessage
}
type Stream struct {
	Key, Generation string
	Size            int64
	Modified        time.Time
}
type Checkpoint struct {
	Stream, Generation, Position, PrefixDigest, TailDigest, FileIdentity, AdapterState, ObservedModified string
	ObservedSize                                                                                         int64
	Reconciling, Discarding, CaughtUp                                                                    bool
}
type Limits struct {
	MaxBytes            int64
	StopAt              int64
	EndBounded          bool
	MaxEvents, MaxPages int
	Stability           time.Duration
}
type Observation struct {
	Origin                             records.Origin
	CWD, Title, Branch, HarnessVersion string
	Parent                             *records.SessionLink
	Bindings                           []records.Binding
	Links                              []records.SessionLink
	State                              string
}
type Event struct {
	Origin                records.Origin
	CWD                   string
	CorrectionScope       string
	Event                 records.Event
	ToolBody, PositionKey string
}
type Changes struct {
	Observations          []Observation
	Events                []Event
	Omissions             []records.Omission
	Next                  Checkpoint
	CaughtUp              bool
	Quarantine            string
	ObservedConsent       string
	ObservedPolicyVersion int
	ScannedBytes          int64
	Reconcile             bool
}
type Preview struct {
	Locations, Versions []string
	Streams             int
	Bytes               int64
	Unsupported         []string
}
type Adapter interface {
	Name() string
	Version() string
	ParseConfig(json.RawMessage) (any, error)
	Preview(context.Context, Source) (Preview, error)
	Discover(context.Context, Source, string, int) ([]Stream, string, error)
	ReadChanges(context.Context, Source, Stream, Checkpoint, Limits) (Changes, error)
}

var ErrUnavailable = errors.New("capture source unavailable")
var ErrInvalid = errors.New("invalid capture configuration or source shape")

// Adapter construction belongs to app, avoiding capture -> child -> capture cycles.
func Registry() map[string]Adapter { return make(map[string]Adapter) }
func DefaultLimits(l Limits) Limits {
	if l.MaxBytes <= 0 || l.MaxBytes > 4<<20 {
		l.MaxBytes = 4 << 20
	}
	if l.MaxEvents <= 0 || l.MaxEvents > 4096 {
		l.MaxEvents = 4096
	}
	if l.MaxPages <= 0 || l.MaxPages > 20 {
		l.MaxPages = 20
	}
	if l.Stability <= 0 {
		l.Stability = time.Minute
	}
	return l
}
