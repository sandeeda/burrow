# Security

Burrow gives your phone access to a folder on your computer. A bug that exposed the rest of your disk would be serious, so security is part of the design, and every rule below is backed by tests.

## Reporting a vulnerability

Please **don't open a public issue**. Use GitHub's private reporting instead: **Security → Report a vulnerability** on this repository. You'll get a reply within a few days. Please include the version (`burrow --version`), your OS, and steps to reproduce.

## How Burrow protects your files

### Who can connect
- Burrow listens on **two places only**: your Tailscale network (HTTPS, via the built-in Tailscale client) and `127.0.0.1` on your own computer. It never listens on `0.0.0.0`, your LAN or the internet, and no ports are opened on your router.
- Requests must use Burrow's own host name. This blocks "DNS rebinding" attacks, where a website tries to reach `127.0.0.1` through your browser.

### Login
- **A password is always required**, even inside your private Tailscale network.
- Passwords are hashed with **argon2id** (64 MiB, 3 passes). The password itself is never stored.
- **Brute-force protection:** after 5 wrong passwords from a device, it's locked out for 1 minute, doubling up to 1 hour. Failed attempts are printed in Burrow's terminal.
- **Sessions** are random 256-bit tokens in a cookie marked `HttpOnly`, `SameSite=Strict` and (over HTTPS) `Secure`. Only a SHA-256 of each token is stored on disk, so a copy of Burrow's data folder can't be used to log in.
- A normal login lasts 12 hours. **"Remember this device"** lasts 30 days from last use. Changing the password logs out every device.
- **CSRF:** every change (upload, rename, delete…) needs a per-session token in a request header, on top of SameSite cookies and Go's cross-origin request protection.

### Staying inside the shared folder
- All file access goes through Go's [`os.Root`](https://pkg.go.dev/os#Root), which refuses to resolve anything outside the shared folder, including through symlinks.
- On top of that, every path is checked before use. Burrow rejects `..`, absolute paths, Windows drive letters, UNC and device paths (`\\server\share`, `\\?\`), NUL and control characters, backslashes, NTFS alternate streams (`file:stream`), reserved device names (`CON`, `NUL`, …), and trailing dots or spaces.
- This path handling is **fuzz-tested** in CI on Linux and Windows, including real escape attempts through symlinks.
- Burrow keeps its own data (Tailscale keys, password hash, sessions, index, thumbnails, trash) **outside** the shared folder, and refuses to share a folder that contains it.

### Serving files safely
- File types come from a fixed table, never guessed by the browser (`X-Content-Type-Options: nosniff`).
- Only images, video, audio, PDF and plain text are ever shown in the browser. HTML, SVG and anything unknown are always downloads, with a sandboxed Content-Security-Policy. An uploaded web page can't run as part of Burrow.
- The app has a strict Content-Security-Policy: no inline scripts and no third-party code. HSTS is on for the HTTPS address.
- The app's offline cache holds **only the app itself**, never your files or folder listings.

### Uploads
- File names are cleaned to rules that are safe on every OS.
- Uploads stream to a temporary file and are then moved into place atomically. **Existing files are never overwritten**: a second `photo.jpg` becomes `photo (1).jpg`.
- `--read-only` turns off every endpoint that changes files.
- Deleting moves items to Burrow's trash in its data folder, rather than erasing them. Symlinks are moved as links and never followed.

### Supply chain and privacy
- **No telemetry.** Burrow sends nothing anywhere, and it also turns off the Tailscale library's diagnostic log upload.
- Few dependencies, each one chosen for a reason (see `PROGRESS.md`). Go dependencies are scanned with `govulncheck` and web dependencies with `npm audit` on every push. Web dependencies are pinned to exact versions and their install scripts are disabled. CodeQL runs once the repository is public.
- Release binaries are built reproducibly in GitHub Actions, and the installers verify SHA-256 checksums.

## Known limitations

- The binaries aren't code-signed or notarised yet, so macOS and Windows may warn on first run.
- Anyone who can log in to Burrow can see and change everything in the shared folder. Share the folder you mean to, and use `--read-only` when you only need to look.
- On `http://127.0.0.1` the session cookie can't be `Secure`, because that address is plain HTTP. Traffic there never leaves your computer.
