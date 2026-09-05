package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxTrackedAuthSources = 4096

type authAttempt struct {
	startedAt time.Time
	count     int
}

type authAttemptLimiter struct {
	mu      sync.Mutex
	entries map[string]authAttempt
	limit   int
	window  time.Duration
	now     func() time.Time
}

func newAuthAttemptLimiter(limit int, window time.Duration) *authAttemptLimiter {
	return &authAttemptLimiter{entries: make(map[string]authAttempt), limit: limit, window: window, now: time.Now}
}

func (l *authAttemptLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	attempt, exists := l.entries[key]
	if exists && !now.Before(attempt.startedAt.Add(l.window)) {
		delete(l.entries, key)
		exists = false
	}
	if !exists && len(l.entries) >= maxTrackedAuthSources {
		for source, entry := range l.entries {
			if !now.Before(entry.startedAt.Add(l.window)) {
				delete(l.entries, source)
			}
		}
		if len(l.entries) >= maxTrackedAuthSources {
			return false
		}
	}
	if !exists {
		attempt = authAttempt{startedAt: now}
	}
	if attempt.count >= l.limit {
		return false
	}
	attempt.count++
	l.entries[key] = attempt
	return true
}

func (l *authAttemptLimiter) Reset(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

func requestSourceIP(request *http.Request) string {
	remoteAddress := strings.TrimSpace(request.RemoteAddr)
	host, _, err := net.SplitHostPort(remoteAddress)
	if err == nil {
		return host
	}
	if ip := net.ParseIP(remoteAddress); ip != nil {
		return ip.String()
	}
	return "unknown"
}
