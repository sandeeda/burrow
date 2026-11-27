# Burrow 🐇

**Your own Google Drive, in one command.**

Burrow turns any folder on your computer into a private drive you can open from your phone, anywhere: browse, view photos, stream videos, upload from your camera. Your files never leave your disk and nothing is uploaded to a cloud. Your phone reaches your PC through [Tailscale](https://tailscale.com), which is private and encrypted and needs no router setup or open ports.

```
$ burrow ~/Pictures
📁 Sharing:   /home/you/Pictures
✅ Burrow is live at https://burrow.tail1234.ts.net
📱 Scan with your phone's camera (Tailscale must be on):
   ▄▄▄▄▄▄▄ ▄ ▄▄ ▄▄▄▄▄▄▄
   ...
```

> 🚧 Early release. See [SECURITY.md](SECURITY.md) for how Burrow protects your files.

## What you get

- 📱 **A phone app** you can add to your home screen (iPhone and Android). It looks and feels like your phone's own Photos and Files apps.
- 🖼️ **Photos** in a grid with thumbnails and a swipeable viewer with pinch-zoom. **Videos** stream with seeking, even very large ones.
- ⬆️ **Uploads** from your camera or files. Nothing is ever overwritten.
- 🔎 **Search** by file name across the whole folder.
- ✏️ **Rename, new folder, delete.** Deleted items go to Burrow's trash on your PC, so you can recover them.
- 🔐 **A password is always required**, even inside your private Tailscale network.
- 🪶 **One small program** with no other software to install on your PC (Tailscale is built in).

## Quick start (about 5 minutes)

### 1. Set up Tailscale (once)

1. On your **phone**, install the **Tailscale** app ([iPhone](https://apps.apple.com/app/tailscale/id1470499037), [Android](https://play.google.com/store/apps/details?id=com.tailscale.ipn)) and sign in. This creates your free private network.
2. On a computer, open the Tailscale admin console's **[DNS page](https://login.tailscale.com/admin/dns)**:
   - make sure **MagicDNS** is enabled
   - scroll down and click **Enable HTTPS**

You don't need to install Tailscale on your PC, because Burrow has it built in.

### 2. Install Burrow on your PC

**macOS / Linux**
```sh
curl -fsSL https://raw.githubusercontent.com/sandeeda/burrow/main/install.sh | sh
```

**Windows** (PowerShell)
```powershell
irm https://raw.githubusercontent.com/sandeeda/burrow/main/install.ps1 | iex
```

Or download a build for your system from [Releases](https://github.com/sandeeda/burrow/releases). Every download comes with SHA-256 checksums, and both install scripts verify them.

### 3. Run it on a folder

```sh
burrow ~/Pictures                 # macOS / Linux
burrow "$HOME\Pictures"           # Windows
```

The first time:

1. **Create a password.** Burrow asks in the terminal. You'll type it on your phone.
2. **Approve this PC.** Burrow prints a `login.tailscale.com` link. Open it and sign in with the **same account** as your phone.
3. Burrow prints its address and a **QR code**.

### 4. Open it on your phone

Make sure the Tailscale app is **on**, then scan the QR code (or type the address), log in, and add it to your home screen:
- **iPhone (Safari):** Share → **Add to Home Screen**
- **Android (Chrome):** ⋮ → **Install app**

That's it. Your folder is now on your phone, wherever you are.

Detailed guides: [Windows](docs/setup-windows.md) · [macOS](docs/setup-macos.md) · [Linux](docs/setup-linux.md), including starting Burrow automatically when your PC starts.

## Options

```
burrow [flags] <folder>

  --read-only          view and download only: no uploads, renames or deletes
  --port 8080          port for http://127.0.0.1 on this PC (never exposed to the network)
  --hostname burrow    Tailscale machine name → https://<hostname>.<tailnet>.ts.net
  --reset-password     choose a new password (logs out every device)
  --password-file f    set the password from a file (for services / scripts)
  --version            print the version
```

## FAQ

**Is anything uploaded to the internet?**
No. Your files stay on your disk. Your phone talks to your PC directly over Tailscale's encrypted connection. Burrow has no account, no server of ours and no telemetry. It also switches off Tailscale's own diagnostic log upload.

**Who can reach it?**
Only devices signed in to *your* Tailscale network, and they still need Burrow's password. Burrow never listens on your local network or the public internet. On your PC it is also available at `http://127.0.0.1:8080`, for you only.

**I forgot the password.**
On the PC, run `burrow --reset-password <folder>`.

**Where does Burrow keep its own data?**
In `%APPDATA%\burrow` (Windows), `~/Library/Application Support/burrow` (macOS) or `~/.config/burrow` (Linux). This holds the Tailscale identity, password hash, search index, thumbnails and trash. It is never inside the shared folder, and Burrow refuses to share a folder that contains it.

**iPhone HEIC photos don't show thumbnails.**
That's a known gap for now: they show an icon. They open fine in Safari, and other browsers offer a download.

## Building from source

Needs Go ≥ 1.24 and Node.js 24.

```sh
npm --prefix web ci && npm --prefix web run build   # the phone app, embedded into the binary
go build -o burrow ./cmd/burrow
```

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
