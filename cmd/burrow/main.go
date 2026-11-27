// Command burrow serves a folder on this PC to your devices over Tailscale.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"tailscale.com/tsnet"

	"github.com/burrow-placeholder/burrow/internal/config"
)

const (
	hostname  = "burrow"
	localAddr = "127.0.0.1:8080" // never 0.0.0.0 — see CLAUDE.md §4.2
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("burrow: %v", err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	dataDir, err := config.AppDataDir()
	if err != nil {
		return err
	}

	tsLog := func(string, ...any) {}
	if os.Getenv("BURROW_DEBUG") != "" {
		tsLog = log.Printf
	}
	ts := &tsnet.Server{
		Hostname: hostname,
		Dir:      filepath.Join(dataDir, "tsnet"),
		Logf:     tsLog,
		UserLogf: tsLog,
	}
	defer ts.Close()

	handler := newHandler()

	// Local listener first, so localhost works even before Tailscale is up.
	localLn, err := net.Listen("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", localAddr, err)
	}
	fmt.Printf("🏠 Local:     http://%s\n", localAddr)
	go serve(localLn, handler)

	dnsName, err := waitForTailscale(ctx, ts)
	if err != nil {
		return err
	}

	tlsLn, err := listenTLSWithRetry(ctx, ts)
	if err != nil {
		return err
	}
	fmt.Printf("✅ Burrow is live at https://%s\n", dnsName)
	go serve(tlsLn, handler)

	<-ctx.Done()
	fmt.Println("\nShutting down.")
	return nil
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
	}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
		log.Printf("server on %s stopped: %v", ln.Addr(), err)
	}
}

func newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
		io.WriteString(w, helloPage)
	})
	return mux
}

const helloPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Burrow</title>
<style>body{font-family:system-ui,sans-serif;display:grid;place-items:center;min-height:90vh;margin:0}h1{font-size:2rem}</style>
</head><body><h1>Hello from Burrow 🐇</h1></body></html>
`
