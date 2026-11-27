# Show HN

**Title** (≤ 80 chars):

> Show HN: Burrow – turn any folder on your PC into a private drive for your phone

**URL:** https://github.com/sandeeda/burrow

---

**First comment** (post immediately after submitting):

Hi HN, I'm Sandeep. I wanted my photos and files from my home PC on my phone without uploading them to Google Drive or iCloud, and without setting up Nextcloud, port forwarding or a reverse proxy. Burrow is what I ended up with:

    burrow ~/Pictures

prints a QR code; scan it on your phone and you get an installable web app with a photo grid, a swipeable viewer (HEIC included), video streaming with seeking, camera uploads, rename/delete and filename search. Files never leave your disk.

How it works:

- It's a single Go binary with Tailscale embedded via `tsnet`, so the PC joins your tailnet on its own and gets a `https://burrow.<tailnet>.ts.net` address with a real certificate. No open ports, nothing listening on 0.0.0.0 — only the tailnet and 127.0.0.1.
- All file access goes through Go 1.24's `os.Root`, plus path validation that rejects `..`, drive letters, UNC paths, NTFS streams, reserved device names etc. Both are fuzz-tested in CI on Linux and Windows.
- A password is always required (argon2id, lockout, SameSite=Strict cookies + CSRF tokens), even inside the tailnet. First-time setup from the browser is only allowed for devices on the PC's own Tailscale account (checked with tsnet WhoIs).
- The phone app is React + Vite, embedded with `go:embed`; the service worker caches only the app shell, never files.
- Thumbnails: JPEG/PNG/WebP/GIF with EXIF rotation, HEIC through a pure-Go decoder, and video frames via ffmpeg if you have it (ffmpeg never gets a filesystem path — the file is opened through the jail and served to it on a one-time loopback URL).
- No telemetry; it also disables tsnet's default log upload.

What it isn't: sync, multi-user, or a Nextcloud replacement. It's remote access to one copy of your files, for you.

Next up is optional AI search over file contents with a local model (Ollama), with an opt-in "ask" mode using your own Claude key that only sends matched snippets.

[Optional AI disclosure — see docs/launch/README.md]

I'd love feedback, especially on the security model and anything that breaks on your setup. MIT licensed.
