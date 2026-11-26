package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHelloHandler(t *testing.T) {
	h := newHandler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Hello from Burrow") {
		t.Error("body missing greeting")
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("nosniff header = %q", got)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /nope = %d, want 404", rec.Code)
	}
}
