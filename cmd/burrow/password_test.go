package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sandeeda/burrow/internal/auth"
	"github.com/sandeeda/burrow/internal/config"
)

func scripted(answers ...string) passwordPrompter {
	return func(string) (string, error) {
		if len(answers) == 0 {
			return "", errors.New("no more input")
		}
		a := answers[0]
		answers = answers[1:]
		return a, nil
	}
}

func newAM(t *testing.T) *auth.Manager {
	am, err := auth.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return am
}

func loginOK(am *auth.Manager, pw string) bool {
	_, _, err := am.Login(pw, false, "test")
	return err == nil
}

func TestFirstRunPrompts(t *testing.T) {
	am := newAM(t)
	// too short, then mismatch, then good
	err := ensurePassword(am, &config.Config{}, scripted("short", "goodpassword", "different", "goodpassword", "goodpassword"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !loginOK(am, "goodpassword") {
		t.Fatal("password not set")
	}
}

func TestPromptGivesUp(t *testing.T) {
	am := newAM(t)
	err := ensurePassword(am, &config.Config{}, scripted("a", "b", "c"), io.Discard)
	if err == nil || am.HasPassword() {
		t.Fatal("expected failure after 3 bad tries")
	}
}

func TestExistingPasswordNotAsked(t *testing.T) {
	am := newAM(t)
	am.SetPassword("existing-pass")
	asked := false
	err := ensurePassword(am, &config.Config{}, func(string) (string, error) { asked = true; return "", nil }, io.Discard)
	if err != nil || asked {
		t.Fatalf("err=%v asked=%v", err, asked)
	}
}

func TestNoTerminalDefersToBrowserSetup(t *testing.T) {
	am := newAM(t)
	var out strings.Builder
	if err := ensurePassword(am, &config.Config{}, nil, &out); err != nil {
		t.Fatalf("err = %v", err)
	}
	if am.HasPassword() || !strings.Contains(out.String(), "phone") {
		t.Errorf("hasPassword=%v out=%q", am.HasPassword(), out.String())
	}
}

func TestResetPassword(t *testing.T) {
	am := newAM(t)
	am.SetPassword("old-password")
	err := ensurePassword(am, &config.Config{ResetPassword: true}, scripted("new-password", "new-password"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if loginOK(am, "old-password") || !loginOK(am, "new-password") {
		t.Fatal("reset didn't take")
	}
}

func TestPasswordFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "pw.txt")
	os.WriteFile(file, []byte("from-a-file\r\nsecond line ignored\n"), 0o600)

	am := newAM(t)
	if err := ensurePassword(am, &config.Config{PasswordFile: file}, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !loginOK(am, "from-a-file") {
		t.Fatal("password from file not set (CRLF should be trimmed)")
	}

	// A later start with a different file doesn't replace it unless asked.
	os.WriteFile(file, []byte("another-password\n"), 0o600)
	ensurePassword(am, &config.Config{PasswordFile: file}, nil, io.Discard)
	if !loginOK(am, "from-a-file") {
		t.Fatal("password silently replaced")
	}
	ensurePassword(am, &config.Config{PasswordFile: file, ResetPassword: true}, nil, io.Discard)
	if !loginOK(am, "another-password") {
		t.Fatal("--reset-password with file didn't replace it")
	}

	os.WriteFile(file, []byte("short\n"), 0o600)
	if err := ensurePassword(newAM(t), &config.Config{PasswordFile: file}, nil, io.Discard); err == nil {
		t.Error("weak password from file accepted")
	}
	if err := ensurePassword(newAM(t), &config.Config{PasswordFile: filepath.Join(dir, "missing")}, nil, io.Discard); err == nil {
		t.Error("missing file accepted")
	}
}

func TestResetWithoutTerminalFails(t *testing.T) {
	am := newAM(t)
	am.SetPassword("old-password")
	if err := ensurePassword(am, &config.Config{ResetPassword: true}, nil, io.Discard); err == nil {
		t.Fatal("reset without a terminal should fail loudly, not be ignored")
	}
}
