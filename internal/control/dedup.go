package control

import (
	"sync"
	"time"
)

// dedup remembers a command's CommandResult for ttl so that a CP retry of the
// same id receives the original outcome rather than executing the action twice.
type dedup struct {
	ttl time.Duration

	mu      sync.Mutex
	entries map[string]dedupEntry
}

type dedupEntry struct {
	result    CommandResultBody
	expiresAt time.Time
}

func newDedup(ttl time.Duration) *dedup {
	return &dedup{ttl: ttl, entries: map[string]dedupEntry{}}
}

// Run returns the cached CommandResult for id (and reports hit=true) if one
// exists in the window. Otherwise it executes fn, caches the result, and
// returns it with hit=false. The empty id is never cached — used by tests
// and for fire-and-forget messages.
func (d *dedup) Run(id string, fn func() CommandResultBody) (CommandResultBody, bool) {
	now := time.Now()
	if id == "" {
		return fn(), false
	}
	d.mu.Lock()
	if e, ok := d.entries[id]; ok && now.Before(e.expiresAt) {
		d.mu.Unlock()
		return e.result, true
	}
	d.mu.Unlock()

	result := fn()

	d.mu.Lock()
	d.entries[id] = dedupEntry{result: result, expiresAt: now.Add(d.ttl)}
	// Opportunistically GC expired entries to keep the map small.
	for k, v := range d.entries {
		if now.After(v.expiresAt.Add(d.ttl)) {
			delete(d.entries, k)
		}
	}
	d.mu.Unlock()
	return result, false
}
