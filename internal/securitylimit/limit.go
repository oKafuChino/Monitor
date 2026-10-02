// Package securitylimit provides bounded, expiring attempt counters.
package securitylimit

import (
	"sync"
	"time"
)

type entry struct { count int; until time.Time }
type Limiter struct { mu sync.Mutex; items map[string]entry; capacity int }
func New(capacity int) *Limiter { return &Limiter{items: make(map[string]entry), capacity: capacity} }
// Allow never evicts active counters to accommodate a new attacker-controlled key.
func (l *Limiter) Allow(key string, budget int, window time.Duration) bool {
	l.mu.Lock(); defer l.mu.Unlock()
	now := time.Now()
	e, found := l.items[key]
	if !found || !now.Before(e.until) {
		if len(l.items) >= l.capacity {
			for k, v := range l.items { if !now.Before(v.until) { delete(l.items, k) } }
			if len(l.items) >= l.capacity && !found { return false }
		}
		e = entry{until: now.Add(window)}
	}
	if e.count >= budget { return false }
	e.count++; l.items[key] = e; return true
}
