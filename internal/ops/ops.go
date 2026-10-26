package ops

import (
	"context"
	"encoding/json"
	"errors"
)

type Capability string
const (
	CapStatus Capability = "status.read"
	CapMemoryRead Capability = "memory.read"
	CapMemoryWrite Capability = "memory.write"
	CapKnowledge Capability = "knowledge.search"
	CapSessionRead Capability = "session.read"
	CapCandidateReview Capability = "candidate.review"
	CapHint Capability = "capture.hint"
	CapAdmin Capability = "admin"
)
type Grant struct { Client, Profile string; Scopes []string; Caps []Capability }
// Admin is an explicit CLI grant. It never implies session.read to MCP clients.
func (g Grant) Has(c Capability) bool { for _, v := range g.Caps { if v == c { return true } }; return false }
func (g Grant) AllowsScope(scope string) bool { for _, v := range g.Scopes { if v == scope { return true } }; return false }
type Handler func(context.Context, Grant, json.RawMessage) (any, error)
type Method struct { Name string; Cap Capability; Handler Handler }
type Registry struct { methods map[string]Method }
var ErrForbidden = errors.New("operation not permitted by client grant")
var ErrUnknownMethod = errors.New("unknown method")
func NewRegistry() *Registry { return &Registry{methods: make(map[string]Method)} }
func (r *Registry) Add(m Method) { if m.Name == "" || m.Handler == nil { panic("invalid operation") }; if _, ok := r.methods[m.Name]; ok { panic("duplicate operation") }; r.methods[m.Name] = m }
func (r *Registry) Call(ctx context.Context, g Grant, name string, params json.RawMessage) (any, error) {
	m, ok := r.methods[name]; if !ok { return nil, ErrUnknownMethod }; if !g.Has(m.Cap) { return nil, ErrForbidden }; return m.Handler(ctx, g, params)
}
