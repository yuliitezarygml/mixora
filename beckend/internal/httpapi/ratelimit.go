package httpapi

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type attempts struct {
	count int
	until time.Time
}
type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]attempts
}

func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for key, item := range l.entries {
		if now.After(item.until) {
			delete(l.entries, key)
		}
	}
	item, exists := l.entries[ip]
	if !exists {
		if len(l.entries) >= 10000 {
			return false
		}
		item.until = now.Add(time.Minute)
	}
	item.count++
	l.entries[ip] = item
	return item.count <= 20
}
func (s *Server) rateLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !s.limiter.allow(ip) {
			w.Header().Set("Retry-After", "60")
			fail(w, 429, "too many attempts; retry in a minute")
			return
		}
		next(w, r)
	}
}
