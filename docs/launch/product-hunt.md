# Product Hunt

**Name:** Burrow

**Tagline** (≤ 60 chars):
> Your own Google Drive, in one command

(alt: "Your PC's folders on your phone — private, no cloud")

**Topics:** Privacy · Developer Tools · Open Source · Productivity

**Links:** https://github.com/sandeeda/burrow

---

**Description** (≤ 260 chars):

> Burrow turns any folder on your computer into a private drive for your phone. Run one command, scan the QR code, and browse photos, stream videos and upload from your camera — over Tailscale, with no cloud and no open ports. Free & open source.

---

**Maker comment:**

Hi Product Hunt! 👋 I'm Sandeep.

I had thousands of photos on my PC and kept uploading them to cloud drives just to look at them on my phone. Burrow fixes that: the files stay on your computer, and your phone gets a private, installable app to reach them from anywhere.

**How it works**
1. `burrow ~/Pictures` on your computer
2. Scan the QR code on your phone (with the free Tailscale app on)
3. Log in, add to home screen — done

**What you get**
• Photo grid and swipe viewer (iPhone HEIC included)
• Smooth video streaming, even for huge files
• Upload from your camera
• Search, filters, rename, delete-to-trash
• Light & dark mode

**Why it's private**
Nothing is uploaded anywhere. Burrow has Tailscale built in, so only your own devices can reach it — no router setup, no ports, no accounts with us. A password is always required, and there's zero telemetry.

It's one small app for Windows, macOS and Linux, MIT-licensed. Coming next: AI search over your own files, running locally by default.

I'd love your feedback! 🐇

[Optional AI disclosure — see docs/launch/README.md]

---

**Gallery (in order):**
1. Demo GIF (phone): open → scroll photos → swipe viewer → play video → upload
2. Home screen with folder cards (light)
3. Photo grid with category chips (dark)
4. Full-screen viewer
5. Terminal: `burrow ~/Pictures` with the QR code
6. "How it works" graphic: PC ⇄ Tailscale ⇄ phone, "files never leave your disk"
