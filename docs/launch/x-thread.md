# X / Twitter thread

Attach the demo GIF to post 1.

**1/**
I wanted my home PC's photos on my phone — without uploading them to Google Drive or setting up Nextcloud.

So I built Burrow 🐇

One command turns any folder into a private drive you can open from your phone, anywhere.

github.com/sandeeda/burrow

**2/**
```
burrow ~/Pictures
```
→ prints a QR code
→ scan it, log in, add to home screen
→ your folder, on your phone: photos, videos, uploads, search

No ports opened. No cloud. Files never leave your disk.

**3/**
The trick: Tailscale is built in.

Your PC joins your private network by itself and gets an HTTPS address only your devices can reach. Nothing is exposed to the internet — it doesn't even listen on your LAN.

**4/**
It's a real phone app:
• photo grid + swipe viewer (iPhone HEIC too)
• stream multi-GB videos with instant seeking
• upload straight from the camera
• search, rename, delete-to-trash
• light & dark mode, spring animations

**5/**
Security wasn't an afterthought: password always required (argon2id + lockout), CSRF tokens, every file access jailed with Go's os.Root and fuzz-tested, strict CSP — and zero telemetry.

**6/**
One ~10 MB binary for Windows, macOS and Linux. MIT licensed.

Next: AI search over your own files, with a local model by default.

⭐ github.com/sandeeda/burrow
