# r/homelab

**Title:**

> Your NAS/home-server folder as a phone app over Tailscale — single binary, no reverse proxy

---

**Body:**

Homelab angle on a small tool I made: **Burrow** serves one folder from a box at home (NAS, mini PC, Pi 4/5 on 64-bit, or your desktop) as an installable phone web app — photo grid, video streaming with seeking, camera uploads, search.

What made me build it: every option I had needed either port forwarding + a reverse proxy + certs, or a full Nextcloud stack, just to look at photos on my phone.

Burrow embeds Tailscale (tsnet), so:
- no ports opened, no proxy, no DNS — it gets a `https://burrow.<tailnet>.ts.net` address with a valid cert by itself
- it only listens on the tailnet and localhost
- the box doesn't even need the Tailscale client installed

Run it as a systemd user service:

```ini
[Service]
ExecStart=%h/.local/bin/burrow %h/Pictures
Restart=on-failure
```

Headless first run? Start it, open it from your phone (same Tailscale account) and set the password there — or use `--password-file`.

Resource use is small: Go binary, SQLite (pure Go, no cgo) filename index, thumbnails cached in its config dir and generated with ≤4 workers. Several instances can run side by side (`--hostname photos`, `--hostname docs`).

Not a sync tool, not multi-user. Login always on; `--read-only` mode for shared viewing.

https://github.com/sandeeda/burrow (MIT)

[Optional AI disclosure — see docs/launch/README.md]
