# r/golang

**Title:**

> Burrow: a single-binary "private Google Drive" using os.Root, tsnet, go:embed and pure-Go SQLite + HEIC

---

**Body:**

I built a small app that shares a folder from your PC to your phone, and Go made the "one binary, no runtime, no cgo" goal pleasant. A few things that might interest this sub:

**`os.Root` as the folder jail.** Every file operation (open, stat, readdir, mkdir, rename, link, remove, walking via `root.FS()`) goes through an `*os.Root`, so symlinks pointing outside the folder are refused by the runtime. On top of that there's a `Clean()` layer for client paths that rejects `..`, backslashes, NUL/control chars, and on Windows drive letters, `:` streams, reserved names (`CON`, `NUL`, `COM1`…) and trailing dots/spaces. Both are fuzzed (`FuzzClean`, `FuzzJailOpen` with real escape attempts through symlinks) in CI on Linux and Windows.

Uploads write to a temp file inside the root, then `root.Link(tmp, final)` — which fails atomically if `final` exists — so there's no check-then-rename race for "never overwrite" (with a fallback for FAT/exFAT).

**tsnet** embeds Tailscale: `ListenTLS("tcp", ":443")` gets a real cert for `*.ts.net`, `LocalClient().WhoIs()` tells me which tailnet user a request comes from (used to allow first-time password setup only from the owner's devices). Tip: tsnet uploads logs to log.tailscale.com by default — `logtail.Disable()` turns that off.

**Everything else:** `modernc.org/sqlite` (no cgo) for the filename index; `go:embed all:dist` for the React app; `http.ServeContent` for range requests (video seeking); `http.CrossOriginProtection` (1.25) plus CSRF tokens; argon2id from x/crypto; HEIC thumbnails via `github.com/gen2brain/h265` (pure-Go HEVC decoder — wrapped with size limits, `recover()` and a fuzz target since it parses untrusted uploads). Video frames use ffmpeg if present, but ffmpeg never gets a path: the file is opened through the root and served to it by a loopback `net/http` server under a one-time token, with the demuxer forced so playlist tricks can't read other files.

Releases are GoReleaser with `CGO_ENABLED=0` for 6 targets, ~10 MB each.

Code: https://github.com/sandeeda/burrow (MIT). Happy to hear where the Go could be more idiomatic.

[Optional AI disclosure — see docs/launch/README.md]
