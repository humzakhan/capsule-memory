package app

import "time"

// SyncRetryStatus describes the last failed background attempt, not relay
// coverage or a durable publication receipt. RetryAfter is the earliest retry;
// the scheduler cadence and preceding bounded scope work can delay it further.
type SyncRetryStatus struct {
	Reason     string    `json:"reason"`
	RetryAfter time.Time `json:"retry_after"`
}

func (h *Host) SetSyncRetry(profile, scope string, status SyncRetryStatus) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if status.Reason == "" {
		delete(h.syncRetries[profile], scope)
		return
	}
	if h.syncRetries == nil {
		h.syncRetries = map[string]map[string]SyncRetryStatus{}
	}
	if h.syncRetries[profile] == nil {
		h.syncRetries[profile] = map[string]SyncRetryStatus{}
	}
	h.syncRetries[profile][scope] = status
}

func (h *Host) ClearSyncRetries(profile string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.syncRetries, profile)
}

func (h *Host) SyncRetries(profile string) map[string]SyncRetryStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := map[string]SyncRetryStatus{}
	for scope, status := range h.syncRetries[profile] {
		out[scope] = status
	}
	return out
}
