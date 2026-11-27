package auth

import (
	"sync"
	"time"
)

const (
	// FreeAttempts is how many wrong passwords a client gets before lockout.
	FreeAttempts = 5
	firstLockout = time.Minute
	maxLockout   = time.Hour
	forgetAfter  = 24 * time.Hour
)

// Limiter locks out clients after repeated failed logins. Each lockout
// beyond FreeAttempts doubles, from 1 minute up to 1 hour.
type Limiter struct {
	mu      sync.Mutex
	clients map[string]*attempts
}

type attempts struct {
	failures    int
	lockedUntil time.Time
	last        time.Time
}

func NewLimiter() *Limiter { return &Limiter{clients: map[string]*attempts{}} }

// Wait returns how long the client must wait before trying again (0 if it
// may try now).
func (l *Limiter) Wait(client string, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.clients[client]
	if !ok || !now.Before(a.lockedUntil) {
		return 0
	}
	return a.lockedUntil.Sub(now)
}

// Fail records a failed attempt and returns the lockout it triggers, if any.
func (l *Limiter) Fail(client string, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gcLocked(now)
	a := l.clients[client]
	if a == nil {
		a = &attempts{}
		l.clients[client] = a
	}
	a.failures++
	a.last = now
	if a.failures < FreeAttempts {
		return 0
	}
	d := firstLockout << (a.failures - FreeAttempts)
	if d > maxLockout || d <= 0 {
		d = maxLockout
	}
	a.lockedUntil = now.Add(d)
	return d
}

// Reset clears a client's record after a successful login.
func (l *Limiter) Reset(client string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.clients, client)
}

func (l *Limiter) gcLocked(now time.Time) {
	for k, a := range l.clients {
		if now.Sub(a.last) > forgetAfter && !now.Before(a.lockedUntil) {
			delete(l.clients, k)
		}
	}
}
