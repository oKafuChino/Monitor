package share

import (
 "sync"
 "time"
)
type bucket struct { start time.Time; count int }
type limiter struct { mu sync.Mutex; entries map[string]bucket }
func newLimiter() *limiter { return &limiter{entries:make(map[string]bucket)} }
func (l *limiter) allow(key string,limit int) bool {
 l.mu.Lock();defer l.mu.Unlock();now:=time.Now()
 if len(l.entries)>10000 {
  for k,v:=range l.entries { if now.Sub(v.start)>=time.Minute { delete(l.entries,k) } }
  if len(l.entries)>10000 { return false }
 }
 b:=l.entries[key];if now.Sub(b.start)>=time.Minute { b=bucket{start:now} };b.count++;l.entries[key]=b;return b.count<=limit
}
