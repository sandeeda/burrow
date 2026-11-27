# Burrow — Progress

## Current phase
**Phase 0 — ✅ DONE (2026-11-26).** Owner saw "Hello from Burrow" on phone over mobile data at https://burrow.taile8ab63.ts.net. Next: Phase 1.

## Done
- Git repo (`main`), MIT LICENSE, `.gitignore`, `.gitattributes` (LF endings), stub README
- Go 1.27.4 + GitHub CLI 2.106 installed via winget (2026-11-26)
- `internal/config.AppDataDir()` → `os.UserConfigDir()/burrow` (0700), with test
- `cmd/burrow/main.go`:
  - `127.0.0.1:8080` plain HTTP (local only)
  - `tsnet.Server` hostname `burrow`, state in `<appdata>/burrow/tsnet`
  - Prints Tailscale login URL on first run (polls LocalClient status), warns if HTTPS certs aren't enabled
  - `ListenTLS :443` once connected, prints `https://<magicdns-name>`
  - Hello page with nosniff + strict CSP; handler test
- Verified locally: build/vet/test pass, localhost serves page, login URL printed

## Next
- [x] [HUMAN] Device approved, MagicDNS + HTTPS enabled, phone test passed
- [ ] [HUMAN] `gh auth login` → then replace placeholder module path `github.com/burrow-placeholder/burrow` with real username, create GitHub repo, push
- [ ] Then Phase 1 (start with `fsjail` + fuzz tests)

## Open questions
- GitHub username (module path is a placeholder until then)

## Decisions
- If tailnet HTTPS isn't enabled yet, Burrow waits and retries every 10s instead of exiting (localhost keeps working). Ctrl+C exits cleanly.
- Default branch `main`.
- tsnet logs are silenced by default; set `BURROW_DEBUG=1` to see them.
- Login URL detected by polling tsnet LocalClient status (more reliable than parsing log lines).

## Known issues
- None yet

## Ideas (out of scope for now)
- None yet
