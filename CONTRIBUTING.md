# Contributing to Burrow

Thanks for helping! Bug reports, ideas and pull requests are welcome. For security problems, see [SECURITY.md](SECURITY.md) instead of opening an issue.

## Development setup

You need **Go ≥ 1.24** and **Node.js 24**.

```sh
git clone https://github.com/sandeeda/burrow
cd burrow
npm --prefix web ci
npm --prefix web run build      # builds the phone app into web/dist (embedded into the binary)
go build -o burrow ./cmd/burrow
./burrow --port 8080 ~/some-test-folder
```

**Working on the web app:** run Burrow on port 8080, then `npm --prefix web run dev`. Vite serves the app with hot reload and passes `/api` calls through to Burrow.

**Your own data stays separate:** point `APPDATA` (Windows) or `XDG_CONFIG_HOME` (Linux) at a scratch folder while testing, so a test Burrow doesn't touch your real password or Tailscale identity.

## Layout

| Path | What's there |
|---|---|
| `cmd/burrow` | CLI entry point: flags, password setup, Tailscale, QR code |
| `internal/fsjail` | the folder jail (`os.Root` + path validation), with fuzz tests |
| `internal/api` | HTTP handlers and middleware (auth, CSRF, headers, gzip) |
| `internal/auth` | argon2id passwords, sessions, login rate limiting |
| `internal/index` | SQLite file-name index |
| `internal/thumbs` | thumbnail generation and cache |
| `web/` | React + Vite + TypeScript PWA |
| `tools/genicon` | draws the app icons |

## Tests

```sh
go vet ./...
go test ./...
go test -race ./...                                   # Linux/macOS
go test ./internal/fsjail -run '^$' -fuzz FuzzClean -fuzztime 60s
go test ./internal/fsjail -run '^$' -fuzz FuzzJailOpen -fuzztime 60s
npm --prefix web run typecheck
```

CI runs all of this on Linux and Windows, plus `govulncheck` and `npm audit`.

## Ground rules

- **Security first.** Read the security section of `CLAUDE.md`. In short: every file access goes through `fsjail`, never through raw `os.Open` on a user-supplied path. Login is never optional. New endpoints that change files need `s.write(...)`.
- **Few dependencies.** Prefer the standard library. If you add a dependency, explain why in the PR.
- **Cross-platform.** Think about Windows paths (`\`, drive letters, reserved names) as well as Unix.
- **Tests with every change**, especially for anything touching paths, auth or uploads.
- **Commits:** [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, `test:`, `chore:`), one logical change per commit.

## Releasing (maintainers)

```sh
git tag v0.1.0 && git push origin v0.1.0
```

The Release workflow tests, builds every platform with GoReleaser and creates a **draft** release. Review it and publish. winget manifests are attached to the workflow run for submission to `microsoft/winget-pkgs`.
