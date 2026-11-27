package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// SessionTTL is how long a normal login lasts.
	SessionTTL = 12 * time.Hour
	// RememberTTL is how long a "remember this device" login lasts since it
	// was last used.
	RememberTTL = 30 * 24 * time.Hour
	// MaxSessions caps stored sessions; the oldest are dropped first.
	MaxSessions = 100
)

var (
	ErrNoPassword    = errors.New("auth: no password set")
	ErrWrongPassword = errors.New("auth: wrong password")
)

// LockedError means too many failed logins; try again after RetryAfter.
type LockedError struct{ RetryAfter time.Duration }

func (e *LockedError) Error() string {
	return fmt.Sprintf("too many attempts, try again in %s", e.RetryAfter.Round(time.Second))
}

// Session is a logged-in device.
type Session struct {
	ID       string    `json:"id"` // sha256 of the cookie token; the token itself is never stored
	CSRF     string    `json:"csrf"`
	Created  time.Time `json:"created"`
	Expires  time.Time `json:"expires"`
	Remember bool      `json:"remember"`
	Client   string    `json:"client"` // e.g. tailnet IP, for the owner's information
}

type passwordFile struct {
	Hash  string    `json:"hash"`
	SetAt time.Time `json:"setAt"`
}

// Manager owns the password and the session store. It persists both in the
// app data dir with 0600 permissions.
type Manager struct {
	dir     string
	limiter *Limiter
	now     func() time.Time
	verify  chan struct{} // bounds concurrent argon2 work (64 MiB each)

	mu       sync.Mutex
	hash     string
	sessions map[string]*Session
}

// Open loads (or initialises) auth state from dir.
func Open(dir string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	m := &Manager{
		dir:      dir,
		limiter:  NewLimiter(),
		now:      time.Now,
		verify:   make(chan struct{}, 2),
		sessions: map[string]*Session{},
	}
	var pf passwordFile
	if err := readJSON(m.path("password.json"), &pf); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	m.hash = pf.Hash
	var list []*Session
	if err := readJSON(m.path("sessions.json"), &list); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	now := m.now()
	for _, s := range list {
		if s != nil && s.ID != "" && now.Before(s.Expires) {
			m.sessions[s.ID] = s
		}
	}
	return m, nil
}

func (m *Manager) path(name string) string { return filepath.Join(m.dir, name) }

// HasPassword reports whether a password has been set.
func (m *Manager) HasPassword() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hash != ""
}

// SetPassword sets a new password and logs out every device.
func (m *Manager) SetPassword(pw string) error {
	h, err := HashPassword(pw)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := writeJSON(m.path("password.json"), passwordFile{Hash: h, SetAt: m.now().UTC()}); err != nil {
		return err
	}
	m.hash = h
	m.sessions = map[string]*Session{}
	return m.saveSessionsLocked()
}

// ErrAlreadySetUp is returned by SetPasswordIfUnset when a password exists.
var ErrAlreadySetUp = errors.New("auth: a password is already set")

// SetPasswordIfUnset sets the first password. It fails with ErrAlreadySetUp
// if one exists, so two devices racing through first-time setup can't both
// win.
func (m *Manager) SetPasswordIfUnset(pw string) error {
	h, err := HashPassword(pw)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hash != "" {
		return ErrAlreadySetUp
	}
	if err := writeJSON(m.path("password.json"), passwordFile{Hash: h, SetAt: m.now().UTC()}); err != nil {
		return err
	}
	m.hash = h
	return nil
}

// Login checks the password and, if right, creates a session. It returns the
// cookie token (shown to the client once, never stored) and the session.
// client identifies the caller for rate limiting (e.g. its IP).
func (m *Manager) Login(pw string, remember bool, client string) (string, *Session, error) {
	if wait := m.limiter.Wait(client, m.now()); wait > 0 {
		return "", nil, &LockedError{RetryAfter: wait}
	}
	m.mu.Lock()
	hash := m.hash
	m.mu.Unlock()
	if hash == "" {
		return "", nil, ErrNoPassword
	}

	m.verify <- struct{}{}
	ok, err := VerifyPassword(pw, hash)
	<-m.verify
	if err != nil {
		return "", nil, err
	}
	if !ok {
		if wait := m.limiter.Fail(client, m.now()); wait > 0 {
			return "", nil, &LockedError{RetryAfter: wait}
		}
		return "", nil, ErrWrongPassword
	}
	m.limiter.Reset(client)

	token := randomToken()
	now := m.now().UTC()
	s := &Session{
		ID:       tokenID(token),
		CSRF:     randomToken(),
		Created:  now,
		Expires:  now.Add(ttl(remember)),
		Remember: remember,
		Client:   client,
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = s
	m.pruneLocked(now)
	if err := m.saveSessionsLocked(); err != nil {
		return "", nil, err
	}
	return token, s, nil
}

// Validate returns the session for a cookie token, if it's valid. Remembered
// sessions slide forward so a device used regularly stays logged in.
func (m *Manager) Validate(token string) (*Session, bool) {
	if token == "" || len(token) > 200 {
		return nil, false
	}
	id := tokenID(token)
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, false
	}
	now := m.now().UTC()
	if !now.Before(s.Expires) {
		delete(m.sessions, id)
		m.saveSessionsLocked()
		return nil, false
	}
	// Extend remembered sessions at most once a day to limit disk writes.
	if s.Remember && s.Expires.Sub(now) < RememberTTL-24*time.Hour {
		s.Expires = now.Add(RememberTTL)
		m.saveSessionsLocked()
	}
	cp := *s
	return &cp, true
}

// Logout ends the session for a cookie token.
func (m *Manager) Logout(token string) {
	id := tokenID(token)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; ok {
		delete(m.sessions, id)
		m.saveSessionsLocked()
	}
}

// LogoutAll ends every session.
func (m *Manager) LogoutAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = map[string]*Session{}
	return m.saveSessionsLocked()
}

func (m *Manager) pruneLocked(now time.Time) {
	for id, s := range m.sessions {
		if !now.Before(s.Expires) {
			delete(m.sessions, id)
		}
	}
	for len(m.sessions) > MaxSessions {
		var oldest *Session
		for _, s := range m.sessions {
			if oldest == nil || s.Created.Before(oldest.Created) {
				oldest = s
			}
		}
		delete(m.sessions, oldest.ID)
	}
}

func (m *Manager) saveSessionsLocked() error {
	list := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	return writeJSON(m.path("sessions.json"), list)
}

func ttl(remember bool) time.Duration {
	if remember {
		return RememberTTL
	}
	return SessionTTL
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func tokenID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("auth: %s is corrupt: %w", filepath.Base(path), err)
	}
	return nil
}

// writeJSON writes atomically (temp file + rename) with owner-only access.
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	// CreateTemp makes the file 0600 (owner only) on Unix; on Windows the
	// app data dir is already private to the user.
	_, werr := tmp.Write(b)
	if err := errors.Join(werr, tmp.Close()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
