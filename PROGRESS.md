# Burrow — Progress

## Current phase
**Phase 2 — Web UI (PWA)** — built and verified in desktop Chrome; waiting for [HUMAN] phone test (install to home screen, browse, photo, video + seek, camera upload, over mobile data).

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

`Entry = {name, path, isDir, size, modTime, type}`. All POSTs need header `X-Burrow: 1`; JSON POSTs need `Content-Type: application/json`.
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
  - `http.CrossOriginProtection` + required `X-Burrow` header + JSON content-type (CSRF, until Phase 3 tokens)
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

## Next
- [x] CI green on Linux + Windows for the Phase 1 code → Phase 1 done
- [x] Phase 2 code: thumbnails, PWA, embedding, CI
- [ ] [HUMAN] Phase 2 phone test over mobile data (test folder of photo copies is being served)
- [ ] Then Phase 3: login, sessions, CSRF tokens, QR code, packaging

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
- Dependency added: `golang.org/x/image` (Go team; WebP decoding + high-quality scaling) — already an indirect dep via tsnet.
- Frontend deps pinned to exact versions at least 2 weeks old (supply-chain caution); `ignore-scripts=true`. No PWA plugin — the manifest and service worker are hand-written (~70 lines).
- Hash routing (`#/folder/sub`) so the server needs no SPA fallback routes.
- Viewer shows 1280px previews instead of originals to save mobile data; "open original" is one tap away.
- Uploads go one file at a time (more reliable on mobile than parallel).
- No video thumbnails: would need ffmpeg. Videos show an icon + play badge.
- UI theme (owner request, 2026-11-27): "liquid glass" with iOS feel and spring animations. Frosted translucent material (backdrop blur + saturation, edge highlights, sheen), iOS 26-style layout (large collapsing title, glass circle buttons, floating bottom search bar + add button), Photos-style grid, drag-to-dismiss sheets, iOS alerts, pull-down menu. Springs: pre-computed CSS `linear()` curves + a ~90-line JS spring (`web/src/spring.ts`) for gesture-driven motion — no animation library. Viewer: finger-following swipe with rubber-band, flick velocity, drag-down to close, pinch + double-tap zoom with momentum pan. Light and dark follow the system setting automatically. Respects Reduce Motion and Reduce Transparency.

## Known issues
- **No login yet (Phase 3).** Anyone on the owner's tailnet (his own devices) can use the API. Localhost is protected from websites by the Host check + CSRF guards.
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
