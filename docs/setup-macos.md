# Burrow on macOS

Works on macOS 12+ (Apple Silicon and Intel). Allow about 5 minutes.

## 1. Tailscale (once)

1. Install the **Tailscale** app on your phone and sign in.
2. Open **https://login.tailscale.com/admin/dns**, make sure **MagicDNS** is on, then click **Enable HTTPS**.

You don't need Tailscale installed on the Mac, because Burrow has it built in. If you already use it there, that's fine too.

## 2. Install Burrow

In **Terminal**:

```sh
curl -fsSL https://raw.githubusercontent.com/sandeeda/burrow/main/install.sh | sh
```

This picks the right build, verifies its checksum and installs `burrow` to `/usr/local/bin` (or `~/.local/bin`, and tells you if that needs adding to your PATH).

Homebrew users, once the tap is published: `brew install sandeeda/tap/burrow`.

## 3. First run

```sh
burrow ~/Pictures
```

1. **Create a password** when asked (typing is hidden).
2. Open the **`https://login.tailscale.com/a/…` link** Burrow prints. Sign in with the **same account as your phone**, then approve the device.
3. Burrow prints its address and a **QR code**.

> **macOS privacy prompt:** sharing Pictures, Documents, Desktop or Downloads makes macOS ask whether Terminal may access that folder. Click **Allow**.
>
> **"burrow can't be opened because Apple cannot check it":** the build isn't notarised yet. The install script clears this for you. If you downloaded it by hand, run `xattr -d com.apple.quarantine /path/to/burrow`.

## 4. On your phone

With the Tailscale app **on**, scan the QR code, log in, then **Share → Add to Home Screen**.

## Start Burrow automatically

After the first run, create a LaunchAgent. Change the folder to yours:

```sh
FOLDER="$HOME/Pictures"
BIN="$(command -v burrow)"
mkdir -p ~/Library/LaunchAgents
cat > ~/Library/LaunchAgents/dev.burrow.plist <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>dev.burrow</string>
  <key>ProgramArguments</key><array><string>$BIN</string><string>$FOLDER</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>$HOME/Library/Logs/burrow.log</string>
  <key>StandardErrorPath</key><string>$HOME/Library/Logs/burrow.log</string>
</dict></plist>
EOF
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/dev.burrow.plist
```

To share a protected folder (Pictures, Documents…) from the background, macOS may need you to add `burrow` under **System Settings → Privacy & Security → Files and Folders** (or Full Disk Access).

Stop and remove it: `launchctl bootout gui/$(id -u)/dev.burrow && rm ~/Library/LaunchAgents/dev.burrow.plist`.

## Troubleshooting

| Problem | Fix |
|---|---|
| "Waiting for Tailscale HTTPS" | Enable HTTPS on the [DNS page](https://login.tailscale.com/admin/dns). |
| Phone can't open the address | Is the Tailscale app on, with the same account? Is Burrow running? |
| Forgot the password | `burrow --reset-password ~/Pictures` |
| Logs (when run in the background) | `~/Library/Logs/burrow.log` |

Burrow's own data lives in `~/Library/Application Support/burrow`.
