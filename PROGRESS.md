# Burrow — Progress

## Current phase
**Phase 3 — Login, security hardening, packaging** — code, packaging and docs done; waiting on [HUMAN] steps (set real password, decide when to go public, 5-minute fresh-install test).

Phase 2 ✅ DONE (2026-11-27): owner tested on phone — installed, browsing, photos, video, uploads all good. Follow-up tweaks from his testing: liquid-glass iOS theme, double-tap zoom-out fix, upload panel auto-hide.

Phase 1 ✅ DONE (2026-11-27): all tests + fuzzing green in CI on Linux and Windows (run 45930813599).

Phase 0 ✅ DONE (2026-11-26): owner saw "Hello from Burrow" on phone over mobile data at https://burrow.taile8ab63.ts.net.

## How to run (dev)
```
go build -o burrow.exe ./cmd/burrow
./burrow.exe [--port 8080] [--read-only] [--hostname burrow] <folder>
```
App data: `%APPDATA%\burrow` (Windows), `~/.config/burrow` (Linux), `~/Library/Application Support/burrow` (macOS) — holds `tsnet/`, `index-<hash>.db`, `trash/`, `thumbs/`.

Frontend dev: run Burrow on :8080, then `npm --prefix web run dev` (Vite proxies `/api`).
Icons: `go run ./tools/genicon` regenerates `web/public/icons`.

## API (Phase 1)
All JSON. Paths are slash-separated, relative to the shared folder (`""` = top).

| Method | Endpoint | Body / query | Result |
|---|---|---|---|
| GET | `/api/session` | public | `{authenticated, csrf?, name?, readOnly}` |
| POST | `/api/login` | `{password, remember}` public | sets cookie; 401 wrong, 429 + Retry-After locked |
| POST | `/api/logout` | CSRF | 204, clears cookie |
| GET | `/api/info` | | `{name, readOnly}` |
| GET | `/api/list?path=` | | `{path, entries:[Entry]}` folders first |
| GET | `/api/stat?path=` | | `Entry` |
| GET | `/api/download?path=&inline=1` | Range supported | file bytes |
| GET | `/api/search?q=&limit=` | | `{query, results:[Entry]}` |
| POST | `/api/upload?path=<folder>` | multipart, field `file` (repeatable) | 201 `{files:[Entry]}` |
| POST | `/api/mkdir` | `{path}` | 201 `Entry` |
| POST | `/api/rename` | `{path, name}` (same folder) | `Entry` |
| POST | `/api/delete` | `{path}` | 204, moved to trash |
| GET | `/api/thumb?path=&size=256|1280&v=<modTime>` | | JPEG (415 unsupported type, 422 undecodable) |
| GET | `/` and app files | | embedded PWA from `web/dist` |

`Entry = {name, path, isDir, size, modTime, type}`. Everything except session/login needs the `burrow_session` cookie (401 otherwise). Changes need header `X-CSRF-Token` (from session/login). JSON POSTs need `Content-Type: application/json`.
Errors: `{error}` with 400 invalid path, 403 permission/read-only/CSRF, 404, 409 exists, 415, 421 wrong Host.

## Done
### Phase 0
- Repo, MIT LICENSE, `.gitignore`, `.gitattributes`, stub README; GitHub `sandeeda/burrow` (private)
- Tools installed via winget (2026-11-26): Go 1.27.4, GitHub CLI 2.106, Python 3.13
- tsnet hostname, state in app data dir, login URL printed, `ListenTLS :443` with retry, `127.0.0.1` local listener

### Phase 1
- `internal/fsjail` — `Clean` + `os.Root` wrapper (Open, OpenFile, Stat, Lstat, ReadDir, Mkdir, Rename no-overwrite, Link, Readlink, Remove, RemoveAll, FS). Fuzz: `FuzzClean`, `FuzzJailOpen`.
- `internal/config` — `Parse` for `burrow [flags] <folder>`; `AppDataDir`.
- `internal/index` — SQLite (`modernc.org/sqlite`) filename index: full walk at startup + every 5 min, immediate updates on API writes; multi-word, case-insensitive, `%`/`_` literal; batched commits.
- `internal/api` — endpoints above, with:
  - Host allowlist per listener (blocks DNS rebinding against localhost)
  - `http.CrossOriginProtection` + JSON content-type (CSRF); Phase 3 replaced the interim `X-Burrow` header with per-session CSRF tokens
  - Security headers on every response (nosniff, CSP, X-Frame-Options, Referrer-Policy, COOP, CORP)
  - Fixed MIME table (not the Windows registry); inline only for images/video/audio/PDF/plain text; HTML/SVG/unknown are attachments; file CSP has `sandbox` (except PDF)
  - Uploads stream to `.burrow-upload-*.tmp`, then hard-link into place (atomic no-overwrite; falls back to check+rename on FAT/exFAT); collisions → `name (1).ext`; names sanitised to Windows-safe rules on every OS. Fuzz: `FuzzSanitizeUploadName`.
  - Delete = copy into `<appdata>/trash/<timestamp-id>/item` + `info.json`, then remove. Symlinks are trashed as links, never followed.
  - Read-only mode blocks every write endpoint.
- `cmd/burrow` — wires it all up; refuses to share a folder that contains (or is inside) the app data dir, after resolving symlinks and 8.3 short names.
- Tests: ~40 API tests (every endpoint, traversal, CSRF, host, read-only, range, large streaming upload, aborted upload cleanup, symlink cases), plus fsjail/index/config/main tests.
- Manual end-to-end on Windows (2026-11-27): list, search, download, range, upload (collision → `cat (1).jpg`), mkdir, rename, delete → trash; attacks (`../`, `..\`, `C:/`, UNC, evil Host, cross-site POST, missing header, `../../evil.txt` upload) all rejected.

### Phase 2 (2026-11-27)
- `internal/thumbs` — on-demand JPEG thumbnails (256 grid / 1280 viewer) for JPEG, PNG, GIF, WebP; EXIF orientation; 100 MP decompression-bomb cap; bounded workers (≤4); per-key de-duplication; cache in `<appdata>/thumbs` keyed by folder+path+size+mtime+size. Real iPhone JPEGs: 25–180 ms first, ~1 ms cached; 1280 preview ≈ 200 KB vs multi-MB originals.
- `web/` — React 19 + Vite 8 + TS 7 PWA (24 npm packages, exact pins, `ignore-scripts`):
  - Grid/list, thumbnails, video badges, paged rendering (150 at a time) for huge folders, breadcrumbs, sort (name/newest/largest, remembered), search (debounced, shows folder of each hit)
  - Viewer: swipe/arrow keys/buttons, swipe-down to close, back button closes viewer, neighbour preloading, 1280 previews with blurred placeholder, "open original", HEIC fallback message (non-Safari), video/audio with seeking
  - Text files as plain text (first 256 KB, never rendered as HTML); PDFs open in a new tab
  - Upload queue (one file at a time, progress, cancel), file picker + camera capture + desktop drag-and-drop; new folder, rename (selects name without extension), delete-to-trash; all hidden in read-only mode
  - Manifest + icons (generated by `tools/genicon`), service worker caches app shell only — never `/api/`
- Server: embedded via `web/embed.go` (`go:embed all:dist`), strict app CSP (no inline scripts/styles), fixed static MIME table, immutable caching for hashed assets, dotfiles hidden, placeholder page if not built; `sw.js` has its own `default-src 'self'` CSP.
- Performance: gzip for JSON + app code; listing uses ReadDir info (only symlinks re-resolved through the jail). 6300-file folder: 2.5 s/763 KB → 7 ms/37 KB.
- Verified in desktop Chrome (2026-11-27): grid, viewer + arrows, HEIC fallback, video seek, back button, rename, upload (collision → "(1)"), search, delete, text viewer XSS check, service worker install + shell cache, 6300-file folder. No console errors.
- CI: new "Web app" job (npm ci, typecheck, build, npm audit, Go build with embedded app). Run 45941183114 green.

### Phase 3 (2026-11-27)
- `internal/auth`: argon2id (m=64 MiB, t=3, p=2, PHC string, strict parse, fuzzed). Sessions: 256-bit random tokens, only SHA-256 stored in `<appdata>/auth/sessions.json` (0600), per-session CSRF token, 12 h normal / 30 d sliding "remember", max 100 sessions, password change logs out all. Lockout: 5 free tries per client IP, then 1 min doubling to 1 h.
- API: every endpoint behind `authed`; changes behind `write` = session + CSRF + not read-only; logout = session + CSRF. Cookie `burrow_session`: HttpOnly, SameSite=Strict, Secure when TLS, Max-Age only with remember. Login/lockout events printed in the terminal.
- CLI: first run prompts for a password (hidden, twice, ≥8 chars); `--password-file` (only sets if none exists unless `--reset-password`); `--reset-password`; `--version`; QR code of the tailnet URL (rsc.io/qr, half-block, explicit colours, ANSI enabled on Windows).
- Web: glass login screen (password-manager friendly, remember switch, shake, lockout countdown), "Can't reach your PC" screen, 401 → back to login, Log Out in ⋯ menu.
- Packaging: `.goreleaser.yaml` (6 targets, CGO off, reproducible, checksums, draft release, Homebrew cask w/ quarantine removal — needs tap + token, winget manifests generated with skip_upload). `release.yml` on v* tags. `install.sh` + `install.ps1` (checksum-verified; `BURROW_VERSION`, `BURROW_INSTALL_DIR`, `BURROW_DOWNLOAD_URL`). `installers.yml` runs both against a snapshot on real Linux/macOS/Windows runners. Local snapshot release verified (6 × ~10 MB); install.ps1 verified locally.
- Docs: README quick start, `docs/setup-{windows,macos,linux}.md` (incl. auto-start: Task Scheduler / launchd / systemd), `SECURITY.md`, `CONTRIBUTING.md`.

### Phase 3 security self-review (§4)
| # | Rule | Status |
|---|---|---|
| 1 | Folder jail | ✅ os.Root everywhere (files, thumbs, index walk, trash, cleanup); Clean() + fuzz (Clean, JailOpen, upload names). Symlinks resolved via jail; escaping ones hidden. |
| 2 | Not on public internet | ✅ tsnet + 127.0.0.1 only; Host allowlist per listener (DNS rebinding). |
| 3 | Login always | ✅ fixed this phase: argon2id, lockout, terminal warnings. |
| 4 | Sessions/CSRF | ✅ fixed this phase. Note: cookie not `Secure` on http://127.0.0.1 (loopback only) — documented. |
| 5 | Safe serving | ✅ fixed MIME table, nosniff, attachment by default, sandbox CSP on files, strict app CSP; **added HSTS** on the tailnet listener. PDF can't be sandboxed (viewer limitation). |
| 6 | Uploads | ✅ sanitised, temp+hard-link, never overwrite; **added cleanup** of temp files left by crashes (>24 h). |
| 7 | Read-only | ✅ all write endpoints; logout still allowed. |
| 8 | AI privacy | n/a until Phase 5. |
| 9 | Supply chain | ✅ govulncheck + npm audit in CI; **added CodeQL** workflow (runs once repo is public); exact-pinned npm deps, ignore-scripts. |
| 10 | No telemetry | ⚠️→✅ **found tsnet uploads logs to log.tailscale.com by default — now disabled** with `logtail.Disable()`. |
| — | Other | Errors never leak paths; JSON bodies ≤64 KB; ReadHeaderTimeout; secrets never logged; data dir 0700. |

## Next
- [x] CI green on Linux + Windows for the Phase 1 code → Phase 1 done
- [x] Phase 2 code: thumbnails, PWA, embedding, CI
- [x] [HUMAN] Phase 2 phone test — owner confirmed "all looks good" (2026-11-27)
- [x] Phase 3 code: login, sessions, CSRF, QR, hardening, packaging, docs
- [ ] [HUMAN] Run burrow in a terminal and set the real password
- [ ] [HUMAN] Decide: make repo public now (needed for installers/releases to download without a GitHub login), or at launch
- [ ] [HUMAN] Fresh-machine test: install → phone access in < 5 minutes following only the README
- [ ] [HUMAN] Optional: create github.com/sandeeda/homebrew-tap + HOMEBREW_TAP_TOKEN secret
- [ ] Then Phase 4: launch prep

## Open questions
- When to make the repo public? Proposed: at v0.1.0 launch (Phase 4). Free CodeQL only runs on public repos.

## Decisions
- Default branch `main`.
- tsnet logs silenced by default; `BURROW_DEBUG=1` shows them.
- Login URL detected by polling tsnet LocalClient status.
- Tailnet HTTPS not ready → retry every 10s, don't exit.
- fsjail: Windows-only name rules applied only on Windows (legal Linux names stay visible). Backslash rejected everywhere.
- fsjail Rename: check-then-rename (tiny race), acceptable for single user. Uploads use hard links (race-free).
- Change detection: periodic rescan (5 min) + updates on API writes, instead of `fsnotify`. Avoids a dependency and Linux per-directory watch limits; changes made outside Burrow show up in search within 5 minutes (folder listings are always live).
- Index DB per shared folder: `index-<sha256(folder)[:8]>.db`.
- Upload names use Windows rules on every OS so files stay portable.
- Rename is in-place only (`{path, name}`); moving between folders isn't in scope.
- CSRF before Phase 3: `CrossOriginProtection` + custom header. Real CSRF tokens come with sessions in Phase 3.
- Dependency added: `modernc.org/sqlite` (pure-Go SQLite, widely used, actively maintained) — required by the tech stack.
- Dependency added: `rsc.io/qr` (QR encoding; tiny, no deps, by Russ Cox) — for the terminal QR code. `golang.org/x/crypto` (argon2) and `golang.org/x/term` (hidden password input) were already indirect deps via tsnet.
- Sharing a folder that *contains* the app data dir stays **refused** for v0.1 (simple + safe). Allowing it would need identity checks on every path (symlinks can alias the data dir) and blocking deletes/renames of its ancestors — revisit if users ask.
- Releases are drafts; the owner publishes them.
- Dependency added: `golang.org/x/image` (Go team; WebP decoding + high-quality scaling) — already an indirect dep via tsnet.
- Frontend deps pinned to exact versions at least 2 weeks old (supply-chain caution); `ignore-scripts=true`. No PWA plugin — the manifest and service worker are hand-written (~70 lines).
- Hash routing (`#/folder/sub`) so the server needs no SPA fallback routes.
- Viewer shows 1280px previews instead of originals to save mobile data; "open original" is one tap away.
- Uploads go one file at a time (more reliable on mobile than parallel).
- No video thumbnails: would need ffmpeg. Videos show an icon + play badge.
- UI theme (owner request, 2026-11-27, revised same day): look and feel of iOS system apps (Photos, Files) rather than a branded app. iOS system colours (white/black, systemGray fills, iOS blue tint), standard UIKit materials and metrics, UINavigationBar with large collapsing title and "‹ Parent" back button, edge-to-edge Photos grid with 2px gaps, Files-style blue folder / document icons, classic UIAlertController action sheets and alerts, UIMenu pull-down, Photos-style viewer bars, Settings-style login. Kept: iOS-like spring motion (CSS linear() curves + web/src/spring.ts for gestures), finger-following swipes, drag-to-dismiss, pinch/double-tap zoom, floating bottom search (iOS 26). An earlier warm "liquid glass" theme was replaced at the owner's request. Light/dark follow the system.

## Known issues
- Sharing a folder that *contains* the app data dir (e.g. the whole home folder) is refused. Better fix for the Phase 3 review: allow it but hide/deny the app data dir inside the jail.
- Delete copies to trash, so deleting a very large folder takes as long as copying it (trash is usually on a different drive). Could add a same-volume fast path later.
- Trash is never emptied automatically.
- PDF responses can't use CSP `sandbox` (browser PDF viewers refuse it). PDFs open in a new tab; check on the phone.
- HEIC (iPhone photos): no thumbnails yet (icon shown); viewer works in Safari only, other browsers get a download button. Known gap per CLAUDE.md.
- iPhone .MOV (HEVC) videos may not play in Chrome on Android/Windows; Safari is fine.
- Thumbnail cache is never pruned (~15 KB per grid thumbnail, ~200 KB per viewed photo).
- Symlink tests skip on the owner's Windows machine (needs Developer Mode); they run in Linux CI.
- Tooling: the Bash tool here can mangle `'\\'` in heredocs — use the Write tool for Go files with backslashes.

## Ideas (out of scope for now)
- Restore from trash / empty trash in the UI
- Move files between folders
- Same-volume fast path for delete
- HEIC thumbnails (needs a decoder — pure-Go options are limited; maybe use the embedded JPEG preview inside HEIC files)
- Video thumbnails (ffmpeg if installed)
- Pinch-zoom inside the in-app photo viewer (today: "open original" for full zoom)
- Thumbnail cache size limit
