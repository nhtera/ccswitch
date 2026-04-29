# ccswitch

> Switch between multiple Claude Code accounts in <100 ms.

`ccswitch` is a single-binary Go CLI that captures your currently-logged-in
Claude Code credential into a named profile, then lets you swap between
profiles with one command. It never replaces `claude /login` — it only
snapshots and restores the credential envelope your platform's secret store
already holds.

- **Fast.** P95 `use` latency < 150 ms on M-series Mac.
- **Single static binary.** ~5 MB, no runtime deps.
- **No telemetry.** The only network call is the optional Anthropic quota check (`list --usage`); everything else is local.
- **OAuth (Pro/Max), Console API key, Enterprise SSO** — all in one tool.
- **Identity-aware.** Shows email + org per profile, auto-suggests names from `~/.claude.json`, finds profiles by email or index.
- **Quota-aware.** `list --usage` fetches Anthropic's 5h/7d quota for each OAuth profile (cached 30s).
- **Running-instance footer.** `list` shows which CLI sessions and IDEs currently have Claude Code attached.

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
make build         # → ./bin/ccswitch
# or, without make:
go build -o ccswitch ./cmd/ccswitch
```

Requires Go 1.25 or newer. See the `Makefile` for `make install`,
`make test`, `make sandbox` (run against an isolated config dir),
`make release` (stripped, trimpath build), etc.

[releases]: https://github.com/nhtera/ccswitch/releases

## Quickstart

```sh
$ claude /login                       # native flow → browser auth
$ ccswitch add                        # name auto-suggested from ~/.claude.json
Captured "erai-dev" (oauth, you@example.com [Erai Dev]).

$ claude /logout && claude /login     # log into another account
$ ccswitch add personal

$ ccswitch list
   NAME              TYPE   IDENTITY                                LAST USED         NOTE
 * erai-dev          oauth  you@example.com [Erai Dev]              2026-04-29 03:12  -
   personal          oauth  me@gmail.com                            2026-04-28 18:20  -

Running instances:
  ● CLI       ~/Code/projects/example  (2 sessions)
  ● VS Code   ~/Code/projects/example  (IDE)

$ ccswitch use personal               # swap by name
$ ccswitch use 2                      # …or by index
$ ccswitch use me@gmail.com           # …or by email
$ ccswitch next                       # …or rotate to the next profile

$ ccswitch list --usage               # per-account quota + reset clock
Accounts:
  1: erai-dev — you@example.com [Erai Dev] (active)
       5h:  63%   resets 00:49  in 4m
       7d:   9%   resets May 6 15:00  in 6d 14h

  2: personal — me@gmail.com
       5h:  48%   resets 02:29  in 1h 44m
       7d:  87%   resets May 1 22:00  in 1d 21h
```

## Commands

| Command | What it does |
|---|---|
| `add [name]` | Capture the live credential. Name auto-suggested from `~/.claude.json` when omitted. `--replace` refreshes an existing profile in place. |
| `use <name\|email\|index>` | Swap the live credential, restore `oauthAccount` block in `~/.claude.json`, and rewrite `active.env`. |
| `next` (alias `switch`, `rotate`) | Rotate to the next profile in alphabetical order. |
| `list`, `ls` | Show all profiles (table). `--json` for scripts. `--usage` to also fetch 5h/7d quota per profile. |
| `current` | Print the active profile name (or `untracked`). |
| `rename <old> <new>` | Rename a profile (keyring + metadata atomic). |
| `remove <name>`, `rm` | Delete a profile (`--force` skips confirm). |
| `prune` (alias `purge`) | Wipe **all** ccswitch data (profiles, secrets, cache, `active.env`). Leaves Claude Code's own login alone. |
| `env <name> --set / --unset / --list` | Manage per-profile env overlay. |
| `export [--out FILE] [--profile NAME]` | Encrypt to portable `.cce` bundle (AES-256-GCM + Argon2id). |
| `import FILE [--rename old=new]` | Decrypt and apply. |
| `doctor` | Read-only diagnostics. `--json`, `--verbose`, `--check-shell`. Fix hints on every WARN/FAIL. |
| `version` | Print version, commit, build date, Go version, platform. |

Global flags: `--debug` (verbose error output), `-y/--yes` (assume yes for confirmations).

## Per-profile env overlay

Each profile can carry environment variables (`ANTHROPIC_BASE_URL`,
`HTTPS_PROXY`, etc.) that get written to `~/.config/ccswitch/active.env`
on `use`. Source it from your shell rc:

```sh
# .zshrc / .bashrc
[ -f ~/.config/ccswitch/active.env ] && . ~/.config/ccswitch/active.env
```

See [docs/shell-integration.md](docs/shell-integration.md) for fish + verification.

## How it works

`ccswitch` treats the Claude Code credential envelope as opaque bytes.
We read it from the OS secret store, store a copy under our own service
name, and restore it verbatim on `use`. Type detection (oauth / api / sso)
is done via SHA-256 fingerprints of canonicalized envelopes — never by
reconstructing fields.

Active-profile detection uses a **stable identity hash** (SHA-256 of
`accountUuid` + `orgUuid` from `~/.claude.json`) so the marker stays
correct even after Claude Code rotates the OAuth token. The volatile
full-blob fingerprint is kept as a fallback for legacy profiles captured
before the stable hash existed.

Identity hints (email, org name, `oauthAccount` JSON block) are copied
from `~/.claude.json` at capture time. On `use`, the snapshotted
`oauthAccount` is written back so `claude /status` shows the correct
account immediately.

For the full design see [docs/system-architecture.md][arch] and the
[implementation plan][plan].

[arch]: docs/system-architecture.md
[plan]: plans/260429-0313-ccswitch-v1/plan.md

## Documentation

- [Install](docs/install.md) — platform-specific notes, dev build, checksum verification
- [Shell integration](docs/shell-integration.md) — sourcing `active.env` (zsh/bash/fish)
- [System architecture](docs/system-architecture.md) — package layout, invariants, storage
- [Troubleshooting](docs/troubleshooting.md) — what to do when `doctor` complains

## License

MIT — see [LICENSE](LICENSE).
