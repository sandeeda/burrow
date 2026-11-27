package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// Config is Burrow's runtime configuration.
type Config struct {
	Folder   string // absolute path of the shared folder
	Port     int    // localhost port
	ReadOnly bool   // disable all write endpoints
	Hostname string // Tailscale machine name

	PasswordFile  string // read the password from this file instead of prompting
	ResetPassword bool   // set a new password (logs out every device)
	ShowVersion   bool   // print the version and exit
}

// ErrHelp is returned by Parse when the user asked for help.
var ErrHelp = flag.ErrHelp

var hostnameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Parse parses command-line arguments (without the program name). Usage and
// errors are written to out.
func Parse(args []string, out io.Writer) (*Config, error) {
	fs := flag.NewFlagSet("burrow", flag.ContinueOnError)
	fs.SetOutput(out)
	c := &Config{}
	fs.IntVar(&c.Port, "port", 8080, "localhost port (only reachable from this PC)")
	fs.BoolVar(&c.ReadOnly, "read-only", false, "disable uploads, renames and deletes")
	fs.StringVar(&c.Hostname, "hostname", "burrow", "Tailscale machine name (https://<hostname>.<tailnet>.ts.net)")
	fs.StringVar(&c.PasswordFile, "password-file", "", "set the login password from the first line of this file (for scripts and services)")
	fs.BoolVar(&c.ResetPassword, "reset-password", false, "choose a new login password (logs out every device)")
	fs.BoolVar(&c.ShowVersion, "version", false, "print the version and exit")
	fs.Usage = func() {
		fmt.Fprintln(out, "Usage: burrow [flags] <folder>")
		fmt.Fprintln(out, "\nShares <folder> with your devices over Tailscale.")
		fmt.Fprintln(out, "\nFlags:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if c.ShowVersion {
		return c, nil
	}

	if fs.NArg() != 1 {
		fs.Usage()
		return nil, errors.New("expected exactly one folder")
	}
	if c.Port < 1 || c.Port > 65535 {
		return nil, fmt.Errorf("invalid --port %d", c.Port)
	}
	if !hostnameRE.MatchString(c.Hostname) {
		return nil, fmt.Errorf("invalid --hostname %q: use lowercase letters, digits and dashes", c.Hostname)
	}

	abs, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return nil, fmt.Errorf("folder: %w", err)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("folder: %w", err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("folder: %s is not a directory", abs)
	}
	c.Folder = abs
	return c, nil
}
