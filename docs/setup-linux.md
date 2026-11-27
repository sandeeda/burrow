# Burrow on Linux

Works on any modern 64-bit Linux (x86-64 or ARM64, including Raspberry Pi 4/5 on a 64-bit OS). Allow about 5 minutes.

## 1. Tailscale (once)

1. Install the **Tailscale** app on your phone and sign in.
2. Open **https://login.tailscale.com/admin/dns**, make sure **MagicDNS** is on, then click **Enable HTTPS**.

You don't need Tailscale installed on the machine, because Burrow has it built in.

## 2. Install Burrow

```sh
curl -fsSL https://raw.githubusercontent.com/sandeeda/burrow/main/install.sh | sh
```

This installs to `/usr/local/bin` if you can write there, otherwise to `~/.local/bin`. Use `BURROW_INSTALL_DIR=/some/dir` to choose. The download's checksum is always verified.

## 3. First run

```sh
burrow ~/Pictures
```

1. **Create a password** when asked.
2. Open the **`https://login.tailscale.com/a/…` link** it prints. Sign in with the **same account as your phone**, then approve the device.
3. Burrow prints its address and a **QR code**.

## 4. On your phone

With the Tailscale app **on**, scan the QR code, log in, and install it to your home screen.

## Run it as a service (systemd)

After the first run, create a user service:

```sh
mkdir -p ~/.config/systemd/user
cat > ~/.config/systemd/user/burrow.service <<'EOF'
[Unit]
Description=Burrow - your folder on your phone
After=network-online.target

[Service]
ExecStart=%h/.local/bin/burrow %h/Pictures
Restart=on-failure

[Install]
WantedBy=default.target
EOF
systemctl --user daemon-reload
systemctl --user enable --now burrow
```

Adjust the `burrow` path (`command -v burrow`) and the folder. To keep it running when you're logged out (for example on a home server): `sudo loginctl enable-linger $USER`.

Logs: `journalctl --user -u burrow -f`.

**Headless or first run without a terminal?** Just start it. With no password set, Burrow shows a **"Welcome to Burrow"** screen where you choose the password from your phone. Only devices signed in to the same Tailscale account as this machine can do that. Alternatively, put the password in a file readable only by you (`chmod 600`) and add `--password-file /path/to/file` once. Burrow only uses it if no password exists yet (add `--reset-password` to replace it), so you can delete the file afterwards.

## Optional: video previews

Install **ffmpeg** and restart Burrow to see a preview frame for each video:

```sh
sudo apt install ffmpeg   # or your distro's package manager
```

Burrow prints `Video thumbnails: on` at startup when it finds ffmpeg. To use a specific ffmpeg, set `BURROW_FFMPEG=/path/to/ffmpeg`.

## Troubleshooting

| Problem | Fix |
|---|---|
| "Waiting for Tailscale HTTPS" | Enable HTTPS on the [DNS page](https://login.tailscale.com/admin/dns). |
| Phone can't open the address | Is the Tailscale app on, with the same account? Is Burrow running (`systemctl --user status burrow`)? |
| Forgot the password | `burrow --reset-password ~/Pictures` (stop the service first) |
| More detail | `BURROW_DEBUG=1 burrow ~/Pictures` |

Burrow's own data lives in `~/.config/burrow` (or `$XDG_CONFIG_HOME/burrow`).
