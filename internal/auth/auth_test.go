package auth

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHashAndVerify(t *testing.T) {
	h, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Errorf("hash format = %q", h)
	}
	if ok, err := VerifyPassword("correct horse", h); !ok || err != nil {
		t.Errorf("right password rejected: %v", err)
	}
	for _, wrong := range []string{"correct hors", "Correct horse", "", strings.Repeat("x", MaxPasswordLen+1)} {
		if ok, _ := VerifyPassword(wrong, h); ok {
			t.Errorf("wrong password %q accepted", wrong)
		}
	}
	h2, _ := HashPassword("correct horse")
	if h == h2 {
		t.Error("same password produced the same hash (salt not random)")
	}
}

func TestPasswordStrength(t *testing.T) {
	for _, pw := range []string{"", "short", "1234567"} {
		if _, err := HashPassword(pw); !errors.Is(err, ErrWeakPassword) {
			t.Errorf("HashPassword(%q) = %v, want ErrWeakPassword", pw, err)
		}
	}
	if err := CheckPasswordStrength("ünïcødé!"); err != nil {
		t.Errorf("8 unicode chars rejected: %v", err)
	}
}

func TestMalformedHashes(t *testing.T) {
	good, _ := HashPassword("password123")
	parts := strings.Split(good, "$")
	for name, h := range map[string]string{
		"empty":        "",
		"bcrypt":       "$2a$10$abcdefghijklmnopqrstuv",
		"argon2i":      strings.Replace(good, "argon2id", "argon2i", 1),
		"bad version":  strings.Replace(good, "v=19", "v=16", 1),
		"huge memory":  strings.Replace(good, "m=65536", "m=99999999", 1),
		"tiny memory":  strings.Replace(good, "m=65536", "m=1", 1),
		"huge time":    strings.Replace(good, "t=3", "t=999", 1),
		"bad salt":     strings.Join(append(parts[:4:4], "!!!", parts[5]), "$"),
		"missing hash": strings.Join(parts[:5], "$"),
	} {
		if ok, err := VerifyPassword("password123", h); ok || err == nil {
			t.Errorf("%s: ok=%v err=%v, want rejection", name, ok, err)
		}
	}
}

func FuzzVerifyPassword(f *testing.F) {
	good, _ := HashPassword("password123")
	f.Add("password123", good)
	f.Add("x", "$argon2id$v=19$m=8192,t=1,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA")
	f.Fuzz(func(t *testing.T, pw, h string) {
		// Must never panic, and never accept a malformed hash.
		ok, err := VerifyPassword(pw, h)
		if ok && err != nil {
			t.Fatal("ok with error")
		}
	})
}

func newManager(t *testing.T) (*Manager, *time.Time) {
	t.Helper()
	m, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Start at the real time: Open() drops sessions already expired by the wall clock.
	now := time.Now().UTC()
	m.now = func() time.Time { return now }
	if err := m.SetPassword("hunter2hunter2"); err != nil {
		t.Fatal(err)
	}
	return m, &now
}

func TestLoginAndValidate(t *testing.T) {
	m, _ := newManager(t)
	if _, _, err := m.Login("nope", false, "1.2.3.4"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("wrong password: %v", err)
	}
	token, s, err := m.Login("hunter2hunter2", false, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 40 || s.CSRF == "" || s.CSRF == token {
		t.Errorf("weak token/csrf: %q %q", token, s.CSRF)
	}
	got, ok := m.Validate(token)
	if !ok || got.CSRF != s.CSRF {
		t.Fatal("valid token rejected")
	}
	for _, bad := range []string{"", "x", token + "x", s.ID, s.CSRF, strings.Repeat("a", 500)} {
		if _, ok := m.Validate(bad); ok {
			t.Errorf("Validate(%q) accepted", bad)
		}
	}
	m.Logout(token)
	if _, ok := m.Validate(token); ok {
		t.Error("token valid after logout")
	}
}

func TestTokenNotStoredOnDisk(t *testing.T) {
	m, _ := newManager(t)
	token, _, _ := m.Login("hunter2hunter2", true, "c")
	b, _ := os.ReadFile(filepath.Join(m.dir, "sessions.json"))
	if strings.Contains(string(b), token) {
		t.Fatal("raw session token written to disk")
	}
	pw, _ := os.ReadFile(filepath.Join(m.dir, "password.json"))
	if strings.Contains(string(pw), "hunter2") {
		t.Fatal("plain password written to disk")
	}
	if runtime.GOOS != "windows" {
		for _, f := range []string{"sessions.json", "password.json"} {
			fi, _ := os.Stat(filepath.Join(m.dir, f))
			if fi.Mode().Perm() != 0o600 {
				t.Errorf("%s mode = %v, want 0600", f, fi.Mode().Perm())
			}
		}
	}
}

func TestSessionExpiry(t *testing.T) {
	m, now := newManager(t)
	short, _, _ := m.Login("hunter2hunter2", false, "c")
	long, _, _ := m.Login("hunter2hunter2", true, "c")

	*now = now.Add(SessionTTL + time.Minute)
	if _, ok := m.Validate(short); ok {
		t.Error("normal session outlived SessionTTL")
	}
	if _, ok := m.Validate(long); !ok {
		t.Error("remembered session expired too early")
	}

	// Used regularly, a remembered session keeps sliding forward…
	for i := 0; i < 60; i++ {
		*now = now.Add(24 * time.Hour)
		if _, ok := m.Validate(long); !ok {
			t.Fatalf("remembered session expired on day %d despite daily use", i)
		}
	}
	// …but not if left unused.
	*now = now.Add(RememberTTL + time.Hour)
	if _, ok := m.Validate(long); ok {
		t.Error("remembered session survived 30 days unused")
	}
}

func TestSessionsPersistAcrossRestart(t *testing.T) {
	m, _ := newManager(t)
	token, s, _ := m.Login("hunter2hunter2", true, "c")
	m2, err := Open(m.dir)
	if err != nil {
		t.Fatal(err)
	}
	m2.now = m.now
	got, ok := m2.Validate(token)
	if !ok || got.CSRF != s.CSRF {
		t.Fatal("session lost after restart")
	}
	if !m2.HasPassword() {
		t.Fatal("password lost after restart")
	}
}

func TestSetPasswordLogsEveryoneOut(t *testing.T) {
	m, _ := newManager(t)
	token, _, _ := m.Login("hunter2hunter2", true, "c")
	if err := m.SetPassword("a whole new password"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Validate(token); ok {
		t.Error("session survived a password change")
	}
	if _, _, err := m.Login("hunter2hunter2", false, "c"); err == nil {
		t.Error("old password still works")
	}
	if _, _, err := m.Login("a whole new password", false, "c"); err != nil {
		t.Errorf("new password: %v", err)
	}
}

func TestNoPassword(t *testing.T) {
	m, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if m.HasPassword() {
		t.Fatal("fresh manager has a password")
	}
	if _, _, err := m.Login("anything", false, "c"); !errors.Is(err, ErrNoPassword) {
		t.Errorf("Login without password = %v", err)
	}
}

func TestLockout(t *testing.T) {
	m, now := newManager(t)
	for i := 1; i < FreeAttempts; i++ {
		if _, _, err := m.Login("wrong", false, "attacker"); !errors.Is(err, ErrWrongPassword) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	_, _, err := m.Login("wrong", false, "attacker")
	var locked *LockedError
	if !errors.As(err, &locked) || locked.RetryAfter != time.Minute {
		t.Fatalf("attempt %d: %v, want 1 minute lockout", FreeAttempts, err)
	}
	// Even the right password is refused while locked.
	if _, _, err := m.Login("hunter2hunter2", false, "attacker"); !errors.As(err, &locked) {
		t.Fatalf("locked client logged in: %v", err)
	}
	// Other clients are unaffected.
	if _, _, err := m.Login("hunter2hunter2", false, "owner-phone"); err != nil {
		t.Fatalf("other client blocked: %v", err)
	}
	// Lockouts double.
	*now = now.Add(time.Minute)
	_, _, err = m.Login("wrong", false, "attacker")
	if !errors.As(err, &locked) || locked.RetryAfter != 2*time.Minute {
		t.Fatalf("second lockout = %v, want 2m", err)
	}
	// ...up to an hour.
	for i := 0; i < 20; i++ {
		*now = now.Add(2 * time.Hour)
		m.Login("wrong", false, "attacker")
	}
	if w := m.limiter.Wait("attacker", *now); w != time.Hour {
		t.Errorf("max lockout = %v, want 1h", w)
	}
	// Success resets.
	*now = now.Add(2 * time.Hour)
	if _, _, err := m.Login("hunter2hunter2", false, "attacker"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Login("wrong", false, "attacker"); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("after reset: %v", err)
	}
}

func TestConcurrentLogins(t *testing.T) {
	m, _ := newManager(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := m.Login("hunter2hunter2", true, "c"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := len(m.sessions); n != 8 {
		t.Errorf("%d sessions, want 8", n)
	}
}

func TestMaxSessions(t *testing.T) {
	m, now := newManager(t)
	first, _, _ := m.Login("hunter2hunter2", true, "c")
	for i := 0; i < MaxSessions; i++ {
		*now = now.Add(time.Second)
		m.Login("hunter2hunter2", true, "c")
	}
	if len(m.sessions) != MaxSessions {
		t.Errorf("%d sessions stored", len(m.sessions))
	}
	if _, ok := m.Validate(first); ok {
		t.Error("oldest session not evicted")
	}
}
