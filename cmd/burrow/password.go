package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/sandeeda/burrow/internal/auth"
	"github.com/sandeeda/burrow/internal/config"
)

// passwordPrompter reads a password without echoing it.
type passwordPrompter func(prompt string) (string, error)

// terminalPrompter reads from the terminal with echo off.
func terminalPrompter(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return string(b), err
}

// ensurePassword makes sure a login password exists, asking for one on the
// first run (or with --reset-password). prompt is nil when there's no
// terminal to ask on.
func ensurePassword(am *auth.Manager, cfg *config.Config, prompt passwordPrompter, out io.Writer) error {
	if cfg.PasswordFile != "" {
		pw, err := readPasswordFile(cfg.PasswordFile)
		if err != nil {
			return err
		}
		// Only replace an existing password when asked to, so a service
		// restarting with --password-file doesn't log every device out.
		if am.HasPassword() && !cfg.ResetPassword {
			return nil
		}
		if err := am.SetPassword(pw); err != nil {
			return fmt.Errorf("--password-file: %w", err)
		}
		fmt.Fprintln(out, "🔐 Password set from file.")
		return nil
	}
	if am.HasPassword() && !cfg.ResetPassword {
		return nil
	}
	if prompt == nil && cfg.ResetPassword {
		return errors.New("--reset-password needs a terminal to type the new password (or use --password-file)")
	}
	if prompt == nil {
		// No terminal (a service, or started remotely): the owner chooses the
		// password on first visit from one of their own devices.
		fmt.Fprintln(out, "🔐 No password yet. Open Burrow on your phone (signed in to this PC's")
		fmt.Fprintln(out, "   Tailscale account) and choose one there — or run burrow in a terminal.")
		return nil
	}

	if cfg.ResetPassword {
		fmt.Fprintln(out, "🔐 Choose a new password for Burrow. Every device will need to log in again.")
	} else {
		fmt.Fprintln(out, "🔐 Welcome! Create a password for Burrow.")
		fmt.Fprintln(out, "   You'll type it on your phone to log in. Use at least", auth.MinPasswordLen, "characters.")
	}
	for attempt := 0; attempt < 3; attempt++ {
		pw, err := prompt("   Password: ")
		if err != nil {
			return err
		}
		if err := auth.CheckPasswordStrength(pw); err != nil {
			fmt.Fprintf(out, "   ✗ That's too short — %v.\n", err)
			continue
		}
		again, err := prompt("   Type it again: ")
		if err != nil {
			return err
		}
		if pw != again {
			fmt.Fprintln(out, "   ✗ Those didn't match. Let's try again.")
			continue
		}
		if err := am.SetPassword(pw); err != nil {
			return err
		}
		fmt.Fprintln(out, "   ✓ Password saved.")
		return nil
	}
	return errors.New("no password set")
}

func readPasswordFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("--password-file: %w", err)
	}
	defer f.Close()
	line, err := bufio.NewReader(io.LimitReader(f, auth.MaxPasswordLen+2)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("--password-file: %w", err)
	}
	pw := strings.TrimRight(line, "\r\n")
	if err := auth.CheckPasswordStrength(pw); err != nil {
		return "", fmt.Errorf("--password-file: %w", err)
	}
	return pw, nil
}
