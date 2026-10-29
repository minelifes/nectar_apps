package api

import (
	"sync"
	"time"
)

// loginLimiter limits failed logins per IP (fixed window).
type loginLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string]*window
	now    func() time.Time
}

type window struct {
	count int
	reset time.Time
}

func newLoginLimiter(max int, w time.Duration) *loginLimiter {
	return &loginLimiter{max: max, window: w, hits: map[string]*window{}, now: time.Now}
}

// allowed reports whether ip may try to log in now.
func (l *loginLimiter) allowed(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	w, ok := l.hits[ip]
	if !ok || l.now().After(w.reset) {
		return true
	}
	return w.count < l.max
}

// fail records a failed attempt.
func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.hits) > 10000 { // bound memory: drop expired windows
		for k, w := range l.hits {
			if now.After(w.reset) {
				delete(l.hits, k)
			}
		}
	}
	w, ok := l.hits[ip]
	if !ok || now.After(w.reset) {
		w = &window{reset: now.Add(l.window)}
		l.hits[ip] = w
	}
	w.count++
}

// reset clears the counter after a successful login.
func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, ip)
}
