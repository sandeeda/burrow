# r/selfhosted

**Title:**

> I made Burrow: one command to reach a folder on your PC from your phone (Tailscale built in, no ports opened)

**Flair:** Release / Built by me (check the sub's current flair + self-promotion rules first)

---

**Body:**

I wanted my phone to see my photos and documents folder at home — browse, view, stream, upload from the camera — without Google Drive, without running Nextcloud, and without exposing anything to the internet. So I built **Burrow**.

```
burrow ~/Pictures
```

It prints a QR code. Scan it on your phone (with the Tailscale app on), log in, add it to your home screen.

**What it does**
- Photo grid with thumbnails (HEIC too), swipe viewer with pinch-zoom
- Video streaming with seeking (works with multi-GB files), preview frames if ffmpeg is installed
- Upload from the camera / photo library, never overwrites existing files
- Rename, new folder, delete → trash on the PC
- Filename search, filters (photos / videos / docs / folders), light/dark

**How it's built**
- One Go binary (~10 MB), Windows/macOS/Linux, amd64 + arm64. No Docker needed (though it would be easy to run in one).
- Tailscale is **embedded** (tsnet): the PC joins your tailnet itself and gets an HTTPS `*.ts.net` address. Listens only on the tailnet and 127.0.0.1 — never 0.0.0.0.
- Login always required (argon2id, lockout, CSRF), folder jail via Go's `os.Root` + fuzz-tested path checks, strict CSP, **no telemetry** (it even turns off tsnet's log upload).
- Files stay where they are. App data (index, thumbnails, trash) lives in its own config dir.

**Trade-offs, honestly**
- Single user (one password per instance). `--read-only` if you want family to view only.
- No sync, no sharing links, no office docs — use Nextcloud if you need those.
- Needs Tailscale (free for personal use) for remote access.
- The PC has to be on.

Install: `curl -fsSL https://raw.githubusercontent.com/sandeeda/burrow/main/install.sh | sh`, `brew install sandeeda/tap/burrow`, or the PowerShell one-liner on Windows. Guides for running it as a service (systemd / launchd / Task Scheduler) are in the repo.

Repo (MIT): https://github.com/sandeeda/burrow

[Optional AI disclosure — see docs/launch/README.md]

Feedback very welcome — especially if something breaks on your setup.
