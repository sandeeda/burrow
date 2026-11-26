# Burrow — Progress

## Current phase
**Phase 0 — Repo + hello world over Tailscale** (in progress)

## Done
- Git repo initialised (`main` branch)
- MIT LICENSE, `.gitignore`, stub README, this file

## Next
- [ ] Owner installs Go (≥ 1.24) and GitHub CLI
- [ ] Owner provides GitHub username → `go mod init github.com/<user>/burrow`
- [ ] `cmd/burrow/main.go`: tsnet server (hostname `burrow`, state in `os.UserConfigDir()/burrow/tsnet`), `ListenTLS :443` + `127.0.0.1:8080`, "Hello from Burrow 🐇", print login URL on first run
- [ ] [HUMAN] Tailscale on phone, MagicDNS + HTTPS certs enabled, approve device, test with Wi-Fi off

## Environment (owner's machine, 2026-11-26)
- Windows 11 Pro
- Node v24.13.0, npm 11.6.2, git 2.53.0 — OK
- Go — **missing**
- gh — **missing**

## Open questions
- GitHub username (for module path)

## Decisions
- Default branch `main`.

## Known issues
- None yet

## Ideas (out of scope for now)
- None yet
