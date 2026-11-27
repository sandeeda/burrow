# LinkedIn post

Attach the demo GIF or 2–3 screenshots.

---

I've stopped uploading my photos to the cloud just to see them on my phone.

Instead, I built **Burrow** — an open-source tool that turns any folder on your computer into a private drive you can open from your phone, from anywhere.

You run one command, scan a QR code, and your folder shows up on your phone as an app: photos, videos, uploads from the camera, search. The files never leave your own disk.

A few things I cared about:

🔒 **Private by design** — it uses Tailscale's encrypted network, so nothing is exposed to the internet and no ports are opened. A password is always required.
📦 **Simple** — one small program for Windows, macOS or Linux. No server to maintain.
📱 **Feels like an app** — swipe through photos, stream long videos, upload straight from the camera.
🧪 **Built carefully** — automated security tests (including fuzzing) run on every change.

Next on the roadmap: AI search over your own files, running locally by default.

It's free and MIT-licensed: https://github.com/sandeeda/burrow

[Optional: a line on how you built it — e.g. "I built it with Claude Code as my pair programmer: I set the design and security requirements, reviewed the work and tested it on my phone."]

I'd love to hear what you'd use it for.

#opensource #privacy #selfhosted #golang
