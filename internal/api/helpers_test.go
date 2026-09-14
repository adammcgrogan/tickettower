package api

import (
	"fmt"
	"testing"
	"time"
)

// Entries that expire and are never read again are still cleared out, once
// a write comes along after the sweep interval.
func TestTTLCacheSweepsExpiredEntries(t *testing.T) {
	c := newTTLCache()
	for i := range 100 {
		c.set(fmt.Sprintf("k%d", i), i, time.Millisecond)
	}
	c.set("keep", 1, time.Hour)
	time.Sleep(5 * time.Millisecond)
	// Pretend the last sweep was long ago, so the next write sweeps.
	c.mu.Lock()
	c.swept = time.Time{}
	c.mu.Unlock()
	c.set("new", 2, time.Hour)
	if n := c.size(); n != 2 {
		t.Errorf("cache holds %d entries after the sweep, want 2 (keep and new)", n)
	}
	if _, ok := c.get("keep"); !ok {
		t.Error("a live entry was swept")
	}
}
