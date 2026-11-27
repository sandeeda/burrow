<p align="center">
  <img src="web/public/icons/icon-192.png" width="96" height="96" alt="Burrow logo: a white rabbit peeking out of its burrow on a violet tile">
</p>

<h1 align="center">Burrow</h1>

<p align="center"><strong>Your own Google Drive, in one command.</strong><br>
Turn any folder on your computer into a private drive you can open from your phone, anywhere.</p>

<p align="center">
  <a href="https://github.com/sandeeda/burrow/actions/workflows/ci.yml"><img src="https://github.com/sandeeda/burrow/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/sandeeda/burrow/releases"><img src="https://img.shields.io/github/v/release/sandeeda/burrow?include_prereleases&sort=semver" alt="Latest release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT license"></a>
</p>

<!-- Demo: record a ~15 s phone screen capture (open app → swipe photos → play a video → upload from camera),
     save it as docs/demo.gif and replace this comment with:
     <p align="center"><img src="docs/demo.gif" width="320" alt="Burrow on a phone"></p> -->

```console
$ burrow ~/Pictures
📁 Sharing:   /home/you/Pictures
🎞️  Video thumbnails: on (ffmpeg found)
✅ Burrow is live at https://burrow.tail1234.ts.net
📱 Scan with your phone's camera (Tailscale must be on)
```

Your files **never leave your disk**. Your phone reaches your computer directly through [Tailscale](https://tailscale.com): private, encrypted and free for personal use. There's no router setup, no open ports, no cloud account and no subscription.

## Install

```sh
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/sandeeda/burrow/main/install.sh | sh

# macOS / Linux with Homebrew
brew install sandeeda/tap/burrow
```

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/sandeeda/burrow/main/install.ps1 | iex
```

Or download a build from [Releases](https://github.com/sandeeda/burrow/releases). It's one ~10 MB file with nothing else to install, and the install scripts verify its SHA-256 checksum.

## Quick start (about 5 minutes)

1. **Phone:** install the Tailscale app ([iPhone](https://apps.apple.com/app/tailscale/id1470499037) · [Android](https://play.google.com/store/apps/details?id=com.tailscale.ipn)) and sign in.
2. **Tailscale admin → [DNS](https://login.tailscale.com/admin/dns):** keep **MagicDNS** on and click **Enable HTTPS**.
3. **Computer:** run `burrow ~/Pictures` (any folder works).
   - **Choose a password.** Burrow asks in the terminal, or on your phone if there's no terminal.
   - **Approve the computer.** Open the `login.tailscale.com` link Burrow prints, with the same account as your phone.
4. **Phone:** scan the QR code, log in, then **Add to Home Screen**.

Step-by-step guides, including starting Burrow automatically: [Windows](docs/setup-windows.md) · [macOS](docs/setup-macos.md) · [Linux](docs/setup-linux.md).

## What you get

- 📱 **A real phone app.** Install it to your home screen on iPhone or Android. Fast, with spring animations and the swipe gestures you expect.
- 🖼️ **Photos.** A thumbnail grid, a full-screen viewer with swipe, pinch and double-tap zoom, and a 1280-pixel preview to save mobile data. **iPhone HEIC photos** work in every browser.
- 🎬 **Videos** stream with instant seeking, even multi-GB files. If ffmpeg is installed you also get preview frames.
- ⬆️ **Uploads** from your camera, photo library or files, with progress. Existing files are never overwritten.
- 🔎 **Search** across every file name in the folder. Filter by photos, videos, docs or folders.
- ✏️ **Rename, new folder, delete.** Deleted items go to a trash folder on your computer, so you can recover them.
- 🌓 **Light and dark mode**, following your phone.
- 🔐 **Login always required**, even on your private network. See [Security](#security).

## How it compares

|  | **Burrow** | Google Drive | Nextcloud | FileBrowser |
|---|---|---|---|---|
| Your files stay on your own disk | ✅ | ❌ uploaded to Google | ✅ | ✅ |
| Setup | one command | sign up | server + database + web server | one binary |
| Reach it from anywhere without opening ports | ✅ built-in Tailscale | ✅ | ⚠️ you set up remote access | ⚠️ you set up remote access |
| Installable phone app | ✅ web app (PWA) | ✅ | ✅ | ⚠️ mobile-friendly website |
| Photo viewer with swipe and zoom, HEIC | ✅ | ✅ | ✅ | ⚠️ basic |
| Video streaming with seeking | ✅ | ✅ | ✅ | ✅ |
| Sync folders between devices | ❌ not a goal | ✅ | ✅ | ❌ |
| Multiple users and sharing links | ❌ just you | ✅ | ✅ | ✅ |
| Office docs, calendars, apps | ❌ | ✅ | ✅ | ❌ |
| Storage limit | your disk | 15 GB free | your disk | your disk |
| Account or telemetry | none | Google account | none | none |

Burrow does one thing: **your folder, on your phone, privately.** If you need sync clients, several users or an office suite, Nextcloud is the better tool. If you want a web file manager on a server you already expose, FileBrowser is great.

## Security

Burrow gives a phone access to a folder on your computer, so it's built defensively:

- **Never on the public internet.** It listens only on your private Tailscale network and on `127.0.0.1`.
- **Password always required:** argon2id hashing, lockout after repeated wrong guesses, HttpOnly/SameSite=Strict cookies, and CSRF tokens on every change.
- **Locked to your folder.** Every file access goes through Go's [`os.Root`](https://pkg.go.dev/os#Root), on top of strict path checks that are fuzz-tested in CI on Linux and Windows.
- **Safe file serving.** Uploaded HTML or SVG is never rendered as part of the app, and there's a strict Content-Security-Policy.
- **No telemetry.** Burrow even switches off the Tailscale library's own diagnostic log upload.

Details and how to report a problem: [SECURITY.md](SECURITY.md).

## FAQ

**Does my computer need to stay on?**
Yes. Burrow serves files from your computer, so it has to be awake and running Burrow. The setup guides show how to start it automatically.

**Is anything uploaded anywhere?**
No. Your phone talks to your computer over Tailscale's encrypted connection. Tailscale helps the two devices find each other but can't read your traffic.

**Can I share more than one folder?**
Run Burrow once per folder with a different `--hostname` and `--port`, for example `burrow --hostname photos ~/Pictures` and `burrow --hostname docs --port 8081 ~/Documents`. Each one gets its own Tailscale device name and its own password.

**Can my family use it?**
Anyone on your Tailscale network who knows the password can. For a read-only view, start Burrow with `--read-only`. Only devices on the computer's own Tailscale account can do the first-time password setup.

**What about my whole home folder?**
Burrow refuses to share a folder that contains its own data folder (Tailscale identity, password hash), which includes your home folder. Share `Pictures`, `Documents` and so on instead.

**I forgot the password.**
On the computer, run `burrow --reset-password <folder>`. This logs out every device.

**Does it need anything else installed?**
No. Optionally install **ffmpeg** (`winget install Gyan.FFmpeg`, `brew install ffmpeg` or `sudo apt install ffmpeg`) to get video preview frames.

**Where does Burrow keep its own data?**
In `%APPDATA%\burrow` (Windows), `~/Library/Application Support/burrow` (macOS) or `~/.config/burrow` (Linux). It is never inside the shared folder.

## Options

```
burrow [flags] <folder>

  --read-only          view and download only: no uploads, renames or deletes
  --port 8080          port for http://127.0.0.1 on this computer (never exposed to the network)
  --hostname burrow    Tailscale machine name → https://<hostname>.<tailnet>.ts.net
  --reset-password     choose a new password (logs out every device)
  --password-file f    set the first password from a file (for services)
  --version            print the version
```

## Roadmap

- **v1.1: AI search.** Find files by what's inside them ("tax receipts 2024"), using a local model through [Ollama](https://ollama.com) by default. Optionally, "Ask" questions with your own Claude API key, sending only the matching snippets.
- Restore from trash in the app.
- Move files between folders.
- Signed and notarised builds for macOS and Windows, and a winget package.

Ideas and bug reports are welcome in [Issues](https://github.com/sandeeda/burrow/issues).

## Building from source

Needs Go ≥ 1.24 and Node.js 24.

```sh
npm --prefix web ci && npm --prefix web run build   # the phone app, embedded into the binary
go build -o burrow ./cmd/burrow
```

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE). Burrow is not affiliated with Tailscale Inc.
