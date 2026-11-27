package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"math"
	"net"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/sandeeda/burrow/internal/auth"
)

const sessionCookie = "burrow_session"

type ctxKey struct{}

func sessionFrom(r *http.Request) *auth.Session {
	s, _ := r.Context().Value(ctxKey{}).(*auth.Session)
	return s
}

// authed requires a valid login session.
func (s *Server) authed(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "please log in")
			return
		}
		sess, ok := s.auth.Validate(c.Value)
		if !ok {
			clearSessionCookie(w, r)
			writeError(w, http.StatusUnauthorized, "please log in")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, sess)))
	})
}

// csrf additionally requires the session's CSRF token in X-CSRF-Token.
// Together with SameSite=Strict cookies and CrossOriginProtection this
// stops other websites from making changes on the user's behalf.
func (s *Server) csrf(h http.HandlerFunc) http.Handler {
	return s.authed(func(w http.ResponseWriter, r *http.Request) {
		want := sessionFrom(r).CSRF
		got := r.Header.Get("X-CSRF-Token")
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			writeError(w, http.StatusForbidden, "missing or invalid CSRF token")
			return
		}
		h(w, r)
	})
}

// write wraps a state-changing handler: login + CSRF token required, and
// refused in read-only mode.
func (s *Server) write(h http.HandlerFunc) http.Handler {
	return s.csrf(func(w http.ResponseWriter, r *http.Request) {
		if s.readOnly {
			writeError(w, http.StatusForbidden, "Burrow is in read-only mode")
			return
		}
		h(w, r)
	})
}

type sessionInfo struct {
	Authenticated bool   `json:"authenticated"`
	CSRF          string `json:"csrf,omitempty"`
	Name          string `json:"name,omitempty"`
	ReadOnly      bool   `json:"readOnly"`
	// No password yet: NeedsSetup if this device may choose it,
	// SetupBlocked if it may not (e.g. another Tailscale user).
	NeedsSetup   bool `json:"needsSetup,omitempty"`
	SetupBlocked bool `json:"setupBlocked,omitempty"`
}

// handleSession tells the app whether it's logged in. It's public, but
// reveals nothing about the files unless the session is valid.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(sessionCookie)
	if err == nil {
		if sess, ok := s.auth.Validate(c.Value); ok {
			if sess.Remember {
				// Keep the cookie's lifetime in step with the sliding session.
				setSessionCookie(w, r, c.Value, true)
			}
			writeJSON(w, http.StatusOK, s.infoFor(sess))
			return
		}
		clearSessionCookie(w, r)
	}
	info := sessionInfo{}
	if !s.auth.HasPassword() {
		if s.mayChoosePassword(r) {
			info.NeedsSetup = true
		} else {
			info.SetupBlocked = true
		}
	}
	writeJSON(w, http.StatusOK, info)
}

// mayChoosePassword reports whether this request may set the first
// password: it must come from this PC itself, or from a device signed in to
// the same Tailscale account as this PC (checked by Options.CanSetUp).
func (s *Server) mayChoosePassword(r *http.Request) bool {
	if ip := net.ParseIP(clientIP(r)); ip != nil && ip.IsLoopback() {
		return true
	}
	return s.canSetUp != nil && s.canSetUp(r)
}

type setupRequest struct {
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

// handleSetup sets the first password from the browser, then logs in.
// Only possible while no password exists, and only from an allowed device.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var req setupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.auth.HasPassword() {
		writeError(w, http.StatusConflict, "Burrow is already set up. Log in instead.")
		return
	}
	if !s.mayChoosePassword(r) {
		s.logf("⚠️  Refused first-time setup from %s (not this PC's Tailscale account)", clientIP(r))
		writeError(w, http.StatusForbidden, "Only a device signed in to this PC's Tailscale account can set up Burrow.")
		return
	}
	switch err := s.auth.SetPasswordIfUnset(req.Password); {
	case errors.Is(err, auth.ErrWeakPassword):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, auth.ErrAlreadySetUp):
		writeError(w, http.StatusConflict, "Burrow is already set up. Log in instead.")
		return
	case err != nil:
		s.logf("api: setup: %v", err)
		writeError(w, http.StatusInternalServerError, "setup failed")
		return
	}
	s.logf("🔐 Password chosen from %s — Burrow is set up.", clientIP(r))
	token, sess, err := s.auth.Login(req.Password, req.Remember, clientIP(r))
	if err != nil {
		s.logf("api: setup login: %v", err)
		writeError(w, http.StatusInternalServerError, "setup failed")
		return
	}
	setSessionCookie(w, r, token, req.Remember)
	writeJSON(w, http.StatusOK, s.infoFor(sess))
}

func (s *Server) infoFor(sess *auth.Session) sessionInfo {
	return sessionInfo{Authenticated: true, CSRF: sess.CSRF, Name: filepath.Base(s.jail.Dir()), ReadOnly: s.readOnly}
}

type loginRequest struct {
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	client := clientIP(r)
	token, sess, err := s.auth.Login(req.Password, req.Remember, client)
	var locked *auth.LockedError
	switch {
	case errors.As(err, &locked):
		secs := int(math.Ceil(locked.RetryAfter.Seconds()))
		s.logf("⚠️  Login locked for %s after repeated wrong passwords (%ds)", client, secs)
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":      "Too many wrong passwords. Try again later.",
			"retryAfter": secs,
		})
		return
	case errors.Is(err, auth.ErrWrongPassword):
		s.logf("⚠️  Wrong password from %s", client)
		writeError(w, http.StatusUnauthorized, "Wrong password")
		return
	case err != nil:
		s.logf("api: login: %v", err)
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	s.logf("🔓 Logged in from %s", client)
	setSessionCookie(w, r, token, req.Remember)
	writeJSON(w, http.StatusOK, s.infoFor(sess))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.auth.Logout(c.Value)
	}
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

// setSessionCookie sets the login cookie: HttpOnly (scripts can't read it),
// SameSite=Strict (never sent on cross-site requests) and Secure whenever
// the connection is HTTPS. The localhost listener is plain HTTP on the
// loopback interface only, so Secure would just break local logins there.
func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, remember bool) {
	c := &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	}
	if remember {
		c.MaxAge = int(auth.RememberTTL.Seconds())
	}
	http.SetCookie(w, c)
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// clientIP identifies the caller for rate limiting: the tailnet peer's IP,
// or 127.0.0.1 locally. Burrow is never behind a proxy, so X-Forwarded-For
// is deliberately ignored (it could be forged).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
