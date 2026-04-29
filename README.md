# ccswitch

> Switch between multiple Claude Code accounts in <100 ms.

`ccswitch` is a single-binary Go CLI that captures your currently-logged-in
Claude Code credential into a named profile, then lets you swap between
profiles with one command. It never replaces `claude /login` — it only
snapshots and restores the credential envelope your platform's secret store
already holds.

- **Fast.** P95 `use` latency < 150 ms on M-series Mac.
- **Single static binary.** ~5 MB, no runtime deps.
- **Zero telemetry.** No network calls anywhere.
- **OAuth (Pro/Max), Console API key, Enterprise SSO** — all in one tool.

## Install

### Homebrew (macOS, recommended)

```sh
brew install nhtera/tap/ccswitch
```

### Direct download

Binaries for macOS / Linux / Windows on the [Releases page][releases].

### Build from source

```sh
git clone https://github.com/nhtera/ccswitch
cd ccswitch
go build -o ccswitch ./cmd/ccswitch
```

[releases]: https://github.com/nhtera/ccswitch/releases

## Quickstart

```sh
$ claude /login                  # native flow → browser auth
$ ccswitch add work              # capture current token
$ claude /logout && claude /login
$ ccswitch add personal
$ ccswitch list
NAME       TYPE  ACTIVE  LAST USED         NOTE
personal   pro   *       2026-04-29 03:12  -
work       max           2026-04-28 18:20  client A

$ ccswitch use work              # swap
$ claude                         # uses work
```

## Commands

| Command | What it does |
|---|---|
| `add <name>` | Capture the live credential into a profile |
| `use <name>` | Swap the live credential + rewrite `active.env` |
| `list`, `ls` | Show all profiles (table or `--json`) |
| `current` | Print the active profile name |
| `rename <old> <new>` | Rename a profile (keyring + metadata) |
| `remove <name>`, `rm` | Delete a profile (`--force` skips confirm) |
| `env <name> --set/--unset/--list` | Manage per-profile env overlay |
| `export [--out FILE] [--profile NAME]` | Encrypt to portable `.cce` |
| `import FILE [--rename old=new]` | Decrypt and apply |
| `doctor` | Read-only diagnostics; fix hints on every WARN/FAIL |

## Per-profile env overlay

Each profile can carry environment variables (`ANTHROPIC_BASE_URL`,
`HTTPS_PROXY`, etc.) that get written to `~/.config/ccswitch/active.env`
on `use`. Source it from your shell rc:

```sh
# .zshrc / .bashrc
[ -f ~/.config/ccswitch/active.env ] && . ~/.config/ccswitch/active.env
```

See [docs/shell-integration.md](docs/shell-integration.md) for details.

## How it works

`ccswitch` treats the Claude Code credential envelope as opaque bytes.
We read it from the OS secret store, store a copy under our own service
name, and restore it verbatim on `use`. Type detection (oauth / api / sso)
and active-account detection are done via SHA-256 fingerprints of
canonicalized envelopes — never by reconstructing fields.

For full design see [docs/system-architecture.md][arch] and the
[implementation plan][plan].

[arch]: docs/system-architecture.md
[plan]: plans/260429-0313-ccswitch-v1/plan.md

## Documentation

- [Install](docs/install.md) — platform-specific notes
- [Shell integration](docs/shell-integration.md) — sourcing `active.env`
- [Troubleshooting](docs/troubleshooting.md) — what to do when `doctor` complains

## License

MIT — see [LICENSE](LICENSE).
