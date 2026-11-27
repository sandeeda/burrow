# Burrow — Progress

## Current phase
**Phase 1 — Core file server** (in progress)

Phase 0 ✅ DONE (2026-11-26): owner saw "Hello from Burrow" on phone over mobile data at https://burrow.taile8ab63.ts.net.

## Done
### Phase 0
- Git repo (`main`), MIT LICENSE, `.gitignore`, `.gitattributes` (LF endings), stub README
- Tools installed via winget (2026-11-26): Go 1.27.4, GitHub CLI 2.106, Python 3.13
- `internal/config.AppDataDir()` → `os.UserConfigDir()/burrow` (0700), with test
- `cmd/burrow/main.go`: `127.0.0.1:8080` plain HTTP; `tsnet` hostname `burrow`, state in `<appdata>/burrow/tsnet`; prints login URL on first run; `ListenTLS :443`; hello page with nosniff + strict CSP

### Phase 1
- `internal/fsjail` ✅
  - `Clean(path)`: client paths are slash-separated, relative. Rejects `..`/`.`/empty segments, backslashes, NUL/control chars, invalid UTF-8, overlong paths; on Windows also `:` (drive letters, ADS), `*?"<>|`, trailing dot/space, reserved device names (CON, NUL, COM1…). Windows rules are testable on any OS via internal `clean(p, windows)`.
  - `Jail` wraps `os.Root`: Open, OpenFile, Stat, Lstat, ReadDir, Mkdir, Rename (no overwrite), Remove. Root itself can't be removed/renamed.
  - Tests: attack-path tables, real escape attempts (incl. symlinks `..`, absolute, dir), writes-stay-inside, no-overwrite rename.
  - Fuzz: `FuzzClean` (invariants + idempotence), `FuzzJailOpen` (secret file outside jail never readable). 60s each locally: ~72M execs, clean.

## Next (Phase 1)
- [ ] CLI: `burrow <folder>` with `--port`, `--read-only`, `--hostname`
- [ ] REST API: list, metadata, download (Range via `http.ServeContent`), upload (streaming multipart, temp+atomic no-overwrite), mkdir, rename, delete → trash in app data dir
- [ ] SQLite filename index (`modernc.org/sqlite`), startup walk + change watching
- [ ] Endpoint unit + integration tests
- [ ] CI (Linux + Windows): test, vet, govulncheck, short fuzz run
- [x] [HUMAN] `gh auth login` done → module path `github.com/sandeeda/burrow`, repo created (private), pushed

## Open questions
- When to make the repo public? (Proposed: at the v0.1.0 launch, Phase 4. Note: free CodeQL code scanning only works on public repos, so it will run once the repo is public.)

## Decisions
- Default branch `main`.
- tsnet logs silenced by default; `BURROW_DEBUG=1` shows them.
- Login URL detected by polling tsnet LocalClient status (more reliable than parsing logs).
- If tailnet HTTPS isn't enabled yet, Burrow retries every 10s instead of exiting; Ctrl+C exits cleanly.
- fsjail: Windows-only filename rules (`:`, `?`, `CON`…) are applied only on Windows, so legal Linux/macOS filenames aren't hidden. Backslash is rejected everywhere (it's a separator on Windows and the API is slash-only).
- fsjail Rename: existence check + rename is two steps (tiny race). Acceptable for single-user; uploads will use a race-free path.

## Known issues
- Symlink escape tests skip on the owner's Windows machine (symlinks need Developer Mode/admin). They will run in Linux CI.
- Bash tool on this machine can mangle `'\\'` inside heredocs — use the Write tool for Go files containing backslashes.

## Ideas (out of scope for now)
- None yet
