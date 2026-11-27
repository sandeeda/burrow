# Burrow on Windows

Works on Windows 10 and 11 (x64 and ARM). Allow about 5 minutes.

## 1. Tailscale (once)

1. Install the **Tailscale** app on your phone and sign in.
2. Open **https://login.tailscale.com/admin/dns**, make sure **MagicDNS** is on, then click **Enable HTTPS**.

You don't need to install Tailscale on the PC, because Burrow has it built in.

## 2. Install Burrow

Open **PowerShell** (Start → type "PowerShell") and paste:

```powershell
irm https://raw.githubusercontent.com/sandeeda/burrow/main/install.ps1 | iex
```

This downloads the right build, checks its checksum, installs it to `%LOCALAPPDATA%\Programs\Burrow` and adds it to your PATH. **Close PowerShell and open a new window** afterwards.

*Prefer doing it by hand?* Download `burrow_<version>_windows_amd64.zip` (or `arm64`) from [Releases](https://github.com/sandeeda/burrow/releases), unzip it, and put `burrow.exe` somewhere convenient.

## 3. First run

```powershell
burrow "$HOME\Pictures"
```

1. **Create a password** when asked (typing is hidden). You'll use it on your phone.
2. Burrow prints a **`https://login.tailscale.com/a/…` link**. Open it and sign in with the **same account as your phone**, then approve the device.
3. Burrow prints its address (`https://burrow.<something>.ts.net`) and a **QR code**.

Leave the window open: Burrow runs as long as it's open. Press **Ctrl+C** to stop it.

> If Windows SmartScreen warns about an unrecognised app, choose **More info → Run anyway**. The builds aren't code-signed yet.

## 4. On your phone

With the Tailscale app **on**, scan the QR code (or type the address), log in, then add it to your home screen (iPhone: Share → **Add to Home Screen**; Android: ⋮ → **Install app**).

## Start Burrow automatically when you log in

After the first run (so the password and Tailscale approval are done), paste this into PowerShell. Change the folder to yours:

```powershell
$folder = "$HOME\Pictures"
$exe    = "$env:LOCALAPPDATA\Programs\Burrow\burrow.exe"
$action = New-ScheduledTaskAction -Execute $exe -Argument "`"$folder`""
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit 0 -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask -TaskName "Burrow" -Action $action -Trigger $trigger -Settings $settings
```

Burrow will start in a small window each time you log in. To remove it: `Unregister-ScheduledTask -TaskName Burrow`.

## Optional: video previews

Install **ffmpeg** and restart Burrow to see a preview frame for each video:

```sh
winget install Gyan.FFmpeg
```

Burrow prints `Video thumbnails: on` at startup when it finds ffmpeg. To use a specific ffmpeg, set `BURROW_FFMPEG=/path/to/ffmpeg`.

## Troubleshooting

| Problem | Fix |
|---|---|
| "Waiting for Tailscale HTTPS" | Enable HTTPS on the [DNS page](https://login.tailscale.com/admin/dns). Burrow picks it up within 10 seconds. |
| Phone can't open the address | Is the Tailscale app on, and signed in to the same account? Is Burrow still running? |
| "can't share … it contains Burrow's private data folder" | You picked a folder that contains `%APPDATA%` (such as your whole user folder). Share a subfolder like `Pictures` or `Documents`. |
| Forgot the password | `burrow --reset-password "$HOME\Pictures"` |
| Want to see more detail | Set `$env:BURROW_DEBUG = "1"` before running Burrow. |

Burrow's own data lives in `%APPDATA%\burrow`. Delete that folder to start completely fresh (new password, new Tailscale device).
