# Launch kit — v0.1.0

Drafts for Sandeep to review, edit in his own voice, and post. Nothing here is posted automatically.

| File | Where | Notes |
|---|---|---|
| [show-hn.md](show-hn.md) | Hacker News "Show HN" | Post the repo URL; first comment is the write-up |
| [reddit-selfhosted.md](reddit-selfhosted.md) | r/selfhosted | Lead with trade-offs; they dislike marketing |
| [reddit-homelab.md](reddit-homelab.md) | r/homelab | Angle: home server / NAS folder on your phone |
| [reddit-golang.md](reddit-golang.md) | r/golang | Technical: os.Root, tsnet, embed, pure-Go SQLite + HEIC |
| [x-thread.md](x-thread.md) | X / Twitter | 6-post thread |
| [linkedin.md](linkedin.md) | LinkedIn | Personal-story angle |
| [product-hunt.md](product-hunt.md) | Product Hunt | Tagline, description, maker comment, gallery list |

## Before launch day

- [ ] Publish the **v0.1.0 draft release** on GitHub (Releases → Edit → Publish). Until then the install scripts and Homebrew can't download it.
- [ ] Check `brew install sandeeda/tap/burrow` works (I can run this on a macOS CI machine once it's published).
- [ ] Record the **demo GIF**: ~15 s phone screen recording — open app → scroll photos → swipe in viewer → play a video and scrub → upload from camera. Use non-personal photos (e.g. a folder of landscapes). Save as `docs/demo.gif` (≤ 5 MB; convert with `ffmpeg -i demo.mp4 -vf "fps=15,scale=360:-1" docs/demo.gif`) and un-comment the line at the top of the README.
- [ ] Take 4–5 **screenshots** for Product Hunt (light + dark): home, photo grid, viewer, upload sheet, login.
- [ ] Decide on the **AI disclosure** line (see below).
- [ ] Optional: submit the **winget** manifest (artifact "winget-manifests" on the Release workflow run) as a PR to [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs) — I can prepare the exact steps.
- [ ] Optional: check the name "Burrow" isn't confusing with another project in your space; consider a domain.

## Posting order (suggested)

1. **Show HN** — weekday morning US time (Tue–Thu, ~8–10 am ET). Stay around for 2–3 hours to answer comments.
2. **r/selfhosted** the same day or next. **r/homelab** a day later (different angle).
3. **r/golang** any day — it's a technical post, not an announcement.
4. **X thread + LinkedIn** after HN, linking the repo.
5. **Product Hunt** a week later, with the demo GIF and screenshots ready (launches go live at 12:01 am PT).

## AI disclosure (your call)

Burrow was written with an AI coding assistant (Claude Code) under your direction. Many HN and Reddit readers ask about this and react badly if they find out later. Each draft has an optional line you can keep, reword or remove, e.g.:

> I built it with Claude Code as my pair programmer — I designed it, reviewed it and tested it on my own phone; the security rules (folder jail, login, CSRF, no telemetry) were requirements from day one, with fuzz tests in CI.

## Answers to questions you'll get

- **"Why not just Tailscale + Samba/SMB?"** — Works on a laptop, but on a phone you want thumbnails, a photo viewer, video seeking and camera upload in one tap; Burrow is that UI plus login and safety on top.
- **"Why not Nextcloud?"** — Nextcloud is a whole suite (users, sync clients, apps) with a server, DB and web server to maintain. Burrow is one binary for one person's folder. Different tools.
- **"Is Tailscale required?"** — For remote access, yes (it's embedded, no separate install on the PC). Without it you still get `http://127.0.0.1:8080` on the computer itself.
- **"What if Tailscale goes away?"** — The tailnet is the transport; Headscale (self-hosted control server) is a possible future option.
- **"Security?"** — Point to SECURITY.md: `os.Root` jail + fuzzed path checks, argon2id, lockout, SameSite=Strict + CSRF tokens, strict CSP, listens only on tailnet + loopback, no telemetry.
- **"Does it sync?"** — No, on purpose. It's remote access to one copy, not sync.
- **"Multiple users?"** — Not yet; one password per instance. `--read-only` for family viewing.
