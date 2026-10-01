package ratelimit

import (
	"sync"
	"time"
)

// Limiter is a fixed-window rate limiter keyed by an arbitrary string such as a
// client IP address. It is safe for concurrent use.
type Limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string][]time.Time
	lastGC time.Time
}

func New(limit int, window time.Duration) *Limiter {
	return &Limiter{
		limit:  limit,
		window: window,
		hits:   make(map[string][]time.Time),
		lastGC: time.Now(),
	}
}

// Allow records an attempt for key and reports whether it stays within the
// configured limit for the current window.
func (l *Limiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastGC) > l.window {
		for candidate, times := range l.hits {
			if len(times) == 0 || now.Sub(times[len(times)-1]) > l.window {
				delete(l.hits, candidate)
			}
		}
		l.lastGC = now
	}
	kept := l.hits[key][:0]
	for _, hit := range l.hits[key] {
		if now.Sub(hit) <= l.window {
			kept = append(kept, hit)
		}
	}
	if len(kept) >= l.limit {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}
