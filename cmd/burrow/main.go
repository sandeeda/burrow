// Command burrow serves a folder on this PC to your devices over Tailscale.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
	"tailscale.com/logtail"
	"tailscale.com/tsnet"

	"github.com/sandeeda/burrow/internal/api"
	"github.com/sandeeda/burrow/internal/auth"
	"github.com/sandeeda/burrow/internal/config"
	"github.com/sandeeda/burrow/internal/fsjail"
	"github.com/sandeeda/burrow/internal/index"
	"github.com/sandeeda/burrow/internal/thumbs"
	"github.com/sandeeda/burrow/web"
)

const rescanInterval = 5 * time.Minute

// version is set at release time (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	cfg, err := config.Parse(os.Args[1:], os.Stderr)
	if errors.Is(err, config.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "burrow: %v\n", err)
		os.Exit(2)
	}
	if cfg.ShowVersion {
		fmt.Println("burrow", version)
		return
	}
	if err := run(cfg); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("burrow: %v", err)
	}
}

func run(cfg *config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	dataDir, err := config.AppDataDir()
	if err != nil {
		return err
	}
	if err := checkNotNested(cfg.Folder, dataDir); err != nil {
		return err
	}

	am, err := auth.Open(filepath.Join(dataDir, "auth"))
	if err != nil {
		return err
	}
	var prompt passwordPrompter
	if term.IsTerminal(int(os.Stdin.Fd())) {
		prompt = terminalPrompter
	}
	if err := ensurePassword(am, cfg, prompt, os.Stdout); err != nil {
		return err
	}

	jail, err := fsjail.Open(cfg.Folder)
	if err != nil {
		return err
	}
	defer jail.Close()

	ix, err := index.Open(filepath.Join(dataDir, indexFileName(cfg.Folder)), jail)
	if err != nil {
		return err
	}
	defer ix.Close()
	go ix.Run(ctx, rescanInterval)

	srv := api.New(api.Options{
		Jail:     jail,
		Index:    ix,
		Auth:     am,
		Thumbs:   thumbs.New(jail, filepath.Join(dataDir, "thumbs"), thumbWorkers()),
		UI:       web.Dist(),
		TrashDir: filepath.Join(dataDir, "trash"),
		ReadOnly: cfg.ReadOnly,
	})

	go func() {
		if n := srv.CleanStaleUploads(ctx, 24*time.Hour); n > 0 {
			log.Printf("Removed %d leftover upload file(s) from interrupted uploads", n)
		}
	}()

	fmt.Printf("📁 Sharing:   %s", cfg.Folder)
	if cfg.ReadOnly {
		fmt.Print("  (read-only)")
	}
	fmt.Println()

	// Local listener first, so localhost works even before Tailscale is up.
	// Never 0.0.0.0 — see CLAUDE.md §4.2.
	localAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Port))
	localLn, err := net.Listen("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", localAddr, err)
	}
	fmt.Printf("🏠 Local:     http://%s\n", localAddr)
	go serve(localLn, srv.Handler(localAddr, "localhost:"+strconv.Itoa(cfg.Port)))

	tsLog := func(string, ...any) {}
	if os.Getenv("BURROW_DEBUG") != "" {
		tsLog = log.Printf
	}
	// No telemetry (CLAUDE.md §4.10): tsnet would otherwise upload its
	// diagnostic logs to log.tailscale.com.
	logtail.Disable()

	ts := &tsnet.Server{
		Hostname: cfg.Hostname,
		Dir:      filepath.Join(dataDir, "tsnet"),
		Logf:     tsLog,
		UserLogf: tsLog,
	}
	defer ts.Close()

	dnsName, err := waitForTailscale(ctx, ts)
	if err != nil {
		return err
	}
	tlsLn, err := listenTLSWithRetry(ctx, ts)
	if err != nil {
		return err
	}
	fmt.Printf("✅ Burrow is live at https://%s\n", dnsName)
	printQR(os.Stdout, "https://"+dnsName, term.IsTerminal(int(os.Stdout.Fd())))
	go serve(tlsLn, srv.Handler(dnsName, dnsName+":443"))

	<-ctx.Done()
	fmt.Println("\nShutting down.")
	return nil
}

// checkNotNested refuses to share a folder that contains Burrow's own data
// (Tailscale keys, index, trash), or that lives inside it.
func checkNotNested(folder, dataDir string) error {
	// Resolve symlinks and Windows 8.3 short names (C:\Users\JOHNSM~1) so
	// different spellings of the same folder compare equal.
	folder, dataDir = canonical(folder), canonical(dataDir)
	if within(dataDir, folder) {
		return fmt.Errorf("can't share %s: it contains Burrow's private data folder (%s). Share a subfolder instead, e.g. Documents", folder, dataDir)
	}
	if within(folder, dataDir) {
		return fmt.Errorf("can't share %s: it is inside Burrow's data folder", folder)
	}
	return nil
}

func canonical(p string) string {
	if c, err := filepath.EvalSymlinks(p); err == nil {
		return c
	}
	return p
}

// within reports whether path is dir or inside it.
func within(path, dir string) bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		// Case-insensitive file systems by default.
		path, dir = strings.ToLower(path), strings.ToLower(dir)
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}

// thumbWorkers limits concurrent image decoding so browsing a big photo
// folder doesn't make the PC sluggish.
func thumbWorkers() int {
	return min(4, max(1, runtime.NumCPU()/2))
}

// indexFileName gives each shared folder its own index database.
func indexFileName(folder string) string {
	sum := sha256.Sum256([]byte(folder))
	return "index-" + hex.EncodeToString(sum[:8]) + ".db"
}

// waitForTailscale starts tsnet, prints the login URL if the device is not yet
// approved, and returns this node's MagicDNS name once it is connected.
func waitForTailscale(ctx context.Context, ts *tsnet.Server) (string, error) {
	if err := ts.Start(); err != nil {
		return "", fmt.Errorf("start tailscale: %w", err)
	}
	lc, err := ts.LocalClient()
	if err != nil {
		return "", fmt.Errorf("tailscale local client: %w", err)
	}

	fmt.Println("🔌 Connecting to Tailscale…")
	var shownURL string
	for {
		st, err := lc.Status(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", fmt.Errorf("tailscale status: %w", err)
		}
		if st.AuthURL != "" && st.AuthURL != shownURL {
			shownURL = st.AuthURL
			fmt.Println()
			fmt.Println("🔑 This PC needs to join your Tailscale network.")
			fmt.Println("   Open this link in a browser and approve the device:")
			fmt.Println()
			fmt.Printf("   %s\n\n", st.AuthURL)
		}
		if st.BackendState == "Running" && st.Self != nil {
			return strings.TrimSuffix(st.Self.DNSName, "."), nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// listenTLSWithRetry keeps trying to open the tailnet HTTPS listener, so a user
// who enables HTTPS certificates in the admin console after starting Burrow
// doesn't need to restart it. Localhost keeps serving in the meantime.
func listenTLSWithRetry(ctx context.Context, ts *tsnet.Server) (net.Listener, error) {
	warned := false
	for {
		ln, err := ts.ListenTLS("tcp", ":443")
		if err == nil {
			return ln, nil
		}
		if !warned {
			warned = true
			fmt.Printf("⏳ Waiting for Tailscale HTTPS: %v\n", err)
			fmt.Println("   Enable MagicDNS and HTTPS in https://login.tailscale.com/admin/dns — Burrow will pick it up automatically.")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}

func serve(ln net.Listener, h http.Handler) {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No ReadTimeout/WriteTimeout: uploads and video streams can be long.
	}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
		log.Printf("server on %s stopped: %v", ln.Addr(), err)
	}
}
