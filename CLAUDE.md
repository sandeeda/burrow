# Burrow — Project Brief for Claude Code

> **Read this whole file before doing anything.** It is the single source of truth for this project.
> The project owner (Sandeep) will not write code himself. You (Claude Code) build everything.
> He will handle only the steps marked **[HUMAN]**, and you must stop and ask him when you reach one.

---

## 1. What we are building

**Burrow** turns any folder on your own PC into a private Google Drive you can reach from your phone anywhere — with one command, and later, AI search over your own files.

```
burrow ~/Documents
→ ✅ Burrow is live at https://burrow.<tailnet>.ts.net
→ 📱 Scan this QR code on your phone
```

- Files stay on the user's own disk. Nothing is uploaded to any cloud.
- Remote access goes over **Tailscale** (private, encrypted, free for personal use). No router setup, no open ports.
- The phone uses a web app (PWA) that can be installed on the home screen.

**One-line pitch:** Your own Google Drive with a brain, in one command.

### Non-goals (do NOT build these unless the owner asks)
- Two-way sync (Dropbox-style copies on every device)
- Offline copies on the phone
- Collaborative document editing
- A public multi-tenant cloud service
- User accounts on any server we host

---

## 2. Tech stack (decided — don't change without asking)

| Layer | Choice | Notes |
|---|---|---|
| Server | **Go** (latest stable, ≥ 1.24) | Single binary, no runtime needed |
| Remote access | **`tailscale.com/tsnet`** embedded in the binary | The PC needs no separate Tailscale install |
| Folder jail | **`os.Root`** (Go 1.24+) | Traversal-resistant file access; never use raw `os.Open` on user paths |
| Database | **SQLite via `modernc.org/sqlite`** (pure Go, no cgo) | Keeps cross-compiling easy |
| Frontend | **React + Vite + TypeScript**, built to `web/dist`, bundled with `go:embed` | Must be an installable PWA |
| Video/file serving | `http.ServeContent` | Handles Range requests (video seeking) |
| Local AI (v1.1) | **Ollama** HTTP API, embedding model `nomic-embed-text` | Optional; detect if running |
| Cloud AI (v1.1) | **Claude API**, user's own key, model name configurable | Optional, opt-in only |
| Vector search (v1.1) | Embeddings stored as BLOBs in SQLite, cosine similarity in Go | No cgo extensions |
| Releases | **GoReleaser** + GitHub Actions | Windows, Linux, macOS (amd64 + arm64) |
| License | MIT | |

---

## 3. Architecture

```
Phone (PWA in browser)
   │  HTTPS over Tailscale (private, encrypted)
   ▼
Your PC: one `burrow` binary
   ├── tsnet listener (https://burrow.<tailnet>.ts.net) + localhost:8080 for local use
   ├── Web server + REST API (serves PWA, login, files, search)
   ├── Folder jail (os.Root) ──► Chosen folder on disk
   ├── Thumbnail cache
   ├── Indexer ──► SQLite (file metadata, later embeddings)
   └── (v1.1, optional) Ollama locally / Claude API with user's key — matched snippets only
```

**HTTPS note:** Browsers require HTTPS for PWAs and service workers. Use `tsnet`'s `ListenTLS` on `:443`, which uses Tailscale's built-in certificates. This needs MagicDNS and HTTPS certificates enabled in the Tailscale admin console — see the [HUMAN] steps.

**App data directory:** `os.UserConfigDir()/burrow/` (tsnet state, SQLite DB, thumbnails, config). Never write app data inside the user's shared folder.

---

## 4. Security rules (NON-NEGOTIABLE)

A single bug that exposes someone's hard drive would kill this project. Treat these as hard requirements, with tests.

1. **Folder jail:** All file access goes through `os.Root` opened on the chosen folder. Reject `..`, absolute paths, symlinks escaping the root, Windows drive letters, UNC paths, and NUL bytes. Write fuzz tests for path handling.
2. **Never on the public internet:** Listen only on the Tailscale interface (tsnet) and `127.0.0.1`. Never `0.0.0.0`.
3. **Login always required**, even over Tailscale. Password hashed with argon2id. Rate-limit login attempts and lock out after repeated failures.
4. **Sessions:** HttpOnly, Secure, SameSite=Strict cookies. CSRF token on every state-changing request. Sessions expire; "remember this device" is optional.
5. **Safe file serving:** Correct MIME types, `X-Content-Type-Options: nosniff`, `Content-Disposition` for downloads, a strict Content-Security-Policy. Never execute or render uploaded HTML/SVG inline as same-origin content.
6. **Uploads:** Sanitize filenames, write to a temp file then rename atomically, never overwrite silently.
7. **Read-only mode flag** (`--read-only`) disables all write endpoints.
8. **AI privacy (v1.1):** Local embeddings by default. Claude calls send only matched snippets, and the UI says so.
9. **Supply chain:** `govulncheck` and CodeQL in CI. Keep dependencies minimal.
10. **No telemetry** unless opt-in and clearly documented.

---

## 5. Repo structure

```
burrow/
├── CLAUDE.md               ← this file
├── PROGRESS.md             ← you maintain this (see §7)
├── README.md
├── LICENSE                 (MIT)
├── SECURITY.md
├── CONTRIBUTING.md
├── go.mod
├── cmd/burrow/main.go      ← CLI entry point
├── internal/
│   ├── config/             ← flags, config file, app data dir
│   ├── fsjail/             ← os.Root wrapper + path validation (+ fuzz tests)
│   ├── api/                ← HTTP handlers, middleware (auth, CSRF, headers)
│   ├── auth/               ← password hashing, sessions, rate limiting
│   ├── net/                ← tsnet + localhost listeners
│   ├── thumbs/             ← thumbnail generation + cache
│   └── index/              ← SQLite, file indexer, (v1.1) embeddings + search
├── web/                    ← React + Vite PWA
│   └── dist/               ← build output, embedded via go:embed
├── docs/                   ← setup guides per OS
├── .github/workflows/      ← CI (test, lint, govulncheck, CodeQL) + release
└── .goreleaser.yaml
```

---

## 6. Build plan — phases and acceptance criteria

Work **one phase at a time**. Do not start the next phase until the current one meets its acceptance criteria **and** the owner has confirmed the [HUMAN] checks.

### Phase 0 — Repo + hello world over Tailscale
- Init Go module `github.com/<owner>/burrow` (ask the owner for his GitHub username).
- Add MIT license, `.gitignore`, a stub README, `PROGRESS.md`.
- `cmd/burrow/main.go`: start a `tsnet.Server` with hostname `burrow`, state dir under the app data dir, and serve "Hello from Burrow 🐇" over HTTPS (`ListenTLS` on `:443`). Also serve on `127.0.0.1:8080`.
- On first run, print the Tailscale login URL clearly.

**Acceptance:** `go build` works; [HUMAN] opens `https://burrow.<tailnet>.ts.net` on his phone **with Wi-Fi off** and sees the page.

### Phase 1 — Core file server
- CLI: `burrow <folder>` with flags `--port`, `--read-only`, `--hostname`.
- `fsjail` package with fuzz tests (do this first).
- REST API (JSON): list folder, file metadata, download (with Range), upload (multipart, streaming, large files), create folder, rename, delete (move to a `.burrow-trash` inside the app data dir, not permanent delete).
- Filename search over an index in SQLite (walk the folder on startup, then watch for changes with `fsnotify` or periodic rescan).
- Unit + integration tests for every endpoint.

**Acceptance:** All tests pass on Linux and Windows in CI. Path-traversal fuzz tests run clean.

### Phase 2 — Web UI (PWA)
- React + Vite + TypeScript in `web/`. Clean, minimal, mobile-first design that also works on desktop.
- Screens: folder grid/list with thumbnails, breadcrumb navigation, photo viewer (swipe), video player (seek works), PDF/text preview, upload (drag-and-drop on desktop, camera/photo picker on phone), rename/delete, search box.
- PWA manifest + service worker (cache the app shell only, never file contents). Burrow icon + name.
- `go:embed` the built `web/dist` into the binary. One binary serves everything.
- Thumbnails: generate on demand, cache in app data dir. Support JPEG, PNG, WebP, GIF. HEIC (iPhone photos): note as a known gap, handle later.

**Acceptance:** [HUMAN] installs Burrow to his phone home screen, browses folders, views a photo, plays a video and skips ahead, uploads a photo from his phone camera — all over mobile data.

### Phase 3 — Login, security hardening, packaging
- First-run setup: prompt in the terminal to create a password (or `--password-file`).
- Login page, sessions, CSRF, rate limiting, security headers (§4).
- Terminal output on start: the URL + a QR code (use a QR library that prints to the terminal).
- `.goreleaser.yaml` + GitHub Actions release workflow on git tags.
- Install paths: `curl -fsSL ... | sh` script (Linux/macOS), GitHub Releases binaries, Homebrew tap, winget manifest (prepare files; [HUMAN] submits).
- Write `SECURITY.md`, `CONTRIBUTING.md`, and setup docs in `docs/` for Windows, Linux and macOS.
- Self-review the whole codebase against §4, list findings in PROGRESS.md, and fix them.

**Acceptance:** On a fresh machine, install → running → phone access in **under 5 minutes**, following only the README.

### Phase 4 — Launch prep (v0.1.0)
- README: headline, demo GIF placeholder, one-line install, features, comparison table (Burrow vs Google Drive vs Nextcloud vs FileBrowser), FAQ, security notes, roadmap.
- Draft (as Markdown files in `docs/launch/`): Show HN post, Reddit posts tailored to r/selfhosted, r/homelab and r/golang, an X/LinkedIn thread, a Product Hunt description.
- Tag `v0.1.0`.

**Acceptance:** [HUMAN] reviews all launch drafts and records the demo GIF.

### Phase 5 — AI search (v1.1)
- Indexer extracts text: plain text, Markdown, PDF, DOCX. (OCR and image captions are later stretch goals — ask before adding.)
- Chunk text, get embeddings from Ollama (`nomic-embed-text`) if it's running; store vectors in SQLite.
- `/api/search?q=` returns semantic matches + filename matches, merged.
- Optional "Ask" feature: if the user has set a Claude API key in settings, send the question + top matched snippets to Claude and return an answer with links to the source files. Show clearly which snippets were sent.
- Settings page: AI on/off, Ollama status, Claude key (stored locally, never logged).
- Background indexing must not slow the PC noticeably (throttle, low priority, resumable).

**Acceptance:** [HUMAN] searches "tax receipts 2024" or similar on his phone and gets the right files even when filenames don't match.

---

## 7. How you (Claude Code) should work

1. **Small steps.** Build in small, testable increments. Run `go build`, `go vet` and `go test ./...` after every meaningful change. Fix failures before moving on.
2. **Commit often** with clear Conventional Commit messages (`feat:`, `fix:`, `test:`, `docs:`, `chore:`). One logical change per commit.
3. **Maintain `PROGRESS.md`:** current phase, what's done, what's next, open questions, known issues, and any decision you made and why. Update it at the end of every work session so a fresh session can pick up exactly where you left off.
4. **Stop at [HUMAN] steps.** Tell the owner exactly what to do in plain, non-technical language, numbered steps, and wait for confirmation.
5. **Explain simply.** The owner is an experienced Java developer but is not writing this code. Summaries should say what changed and how to test it, not walk through every line.
6. **Don't expand scope.** If you think a feature outside the current phase is needed, add it to "Ideas" in PROGRESS.md and ask.
7. **Security first.** If a shortcut would weaken §4, don't take it. Flag it instead.
8. **Cross-platform.** The owner's main machine may be Windows. Test path handling for both Windows and Unix; avoid shell-specific scripts where a Go or npm script works.
9. **Dependencies:** prefer the standard library. Before adding a dependency, check it's maintained and widely used, and note why in PROGRESS.md.
10. **No secrets in the repo.** Auth keys, passwords and API keys come from flags, env vars or the app data dir — never committed.

---

## 8. [HUMAN] steps — things only the owner can do

You must ask the owner to do these at the right time, with simple instructions:

- [ ] Confirm the name "Burrow" is free on GitHub / npm / Homebrew, and buy a domain if wanted (e.g. burrow.dev, getburrow.app).
- [ ] Create the GitHub repo (or let you do it with `gh repo create` after he runs `gh auth login`).
- [ ] Install Go and Node.js LTS if missing (give him the exact commands/links for his OS).
- [ ] Install Tailscale on his phone and log in with the same account he'll use for Burrow.
- [ ] In the Tailscale admin console: enable **MagicDNS** and **HTTPS certificates**.
- [ ] Approve the `burrow` device on first run (open the login URL it prints).
- [ ] Test on the phone with Wi-Fi off after each phase.
- [ ] Install Ollama + pull `nomic-embed-text` (Phase 5).
- [ ] Provide a Claude API key for testing the Ask feature (Phase 5) — via env var, never pasted into code.
- [ ] Record the demo GIF/video and post the launch.

---

## 9. Definition of done for v0.1.0

- One command starts Burrow and prints a URL + QR code.
- Phone can browse, view photos, stream video, upload and download over Tailscale, from anywhere.
- Login required; all §4 rules implemented and tested.
- Installers for Windows, Linux, macOS via GitHub Releases.
- README good enough that a stranger gets it running in under 5 minutes.

---

## 10. First message to start

When the owner says **"Start"**, do this:
1. Read this file and summarize the plan back in 5 lines.
2. Check which tools are installed (`go version`, `node --version`, `git --version`, `gh --version`) and tell him what's missing.
3. Ask for his GitHub username.
4. Begin Phase 0.
