# System Architecture

```
                 ┌─────────────────────────────┐
                 │       ccswitch (Go)         │
                 │  cmd/ccswitch/main.go       │
                 └──────────────┬──────────────┘
                                │ cobra commands
        ┌───────────────────────┼─────────────────────────┐
        ▼                       ▼                         ▼
 ┌─────────────┐        ┌──────────────┐         ┌──────────────────┐
 │ profile     │        │ secrets      │         │ claude (bridge)  │
 │ profiles.json        │ keyring +    │         │ live cred I/O,   │
 │ + env meta  │        │ file fallback│         │ envelope, usage, │
 │ + identity  │        │ + key index  │         │ sessions, IDEs   │
 └─────────────┘        └──────────────┘         └──────────────────┘
        │                       │                         │
        ▼                       ▼                         ▼
 ┌─────────────┐        ┌──────────────┐         ┌──────────────────┐
 │ envfile     │        │ cryptobox    │         │ doctor + checks  │
 │ active.env  │        │ AES-GCM +    │         │ read-only diags  │
 │ writer      │        │ Argon2id     │         │                  │
 └─────────────┘        └──────────────┘         └──────────────────┘
                                ▲
                                │
                        ┌──────────────┐
                        │ export       │
                        │ .cce bundle  │
                        └──────────────┘
```

## Package responsibilities

| Package | Owns | Imports |
|---|---|---|
| `cmd/ccswitch` | cobra wiring, prompts, subcommand glue, table/usage rendering, ANSI-aware columns, running-instance footer, usage cache | every internal pkg |
| `internal/profile` | `profiles.json` shape + CRUD, name validation, identity hints (email/org), stable + volatile fingerprints, name suggestions, lookup-by-(name\|email\|index), rotation | `secrets` (for `ConfigDir`) |
| `internal/secrets` | keyring abstraction, file-vault fallback, `keyring-index.json` for portable `List(prefix)` | `cryptobox` |
| `internal/claude` | live credential read/write per-OS, envelope canonicalization + type detection, `~/.claude.json` reader (account_info), Anthropic usage API client, delegated `claude /status` token refresh, sessions/IDE-instance discovery | (none) |
| `internal/envfile` | atomic `active.env` write, POSIX quoting | (none) |
| `internal/cryptobox` | AES-256-GCM + Argon2id helpers (shared by file vault + export) | (none) |
| `internal/export` | bundle build / seal / open / inspect / apply (`.cce` format) | `cryptobox`, `profile`, `secrets` |
| `internal/doctor` | check framework, `Result`/`Runner` types, text + JSON renderers | (none) |
| `internal/doctor/checks` | concrete checks (live cred, profiles store, fingerprint match, orphan secrets, shell hook, secrets backend) | `claude`, `profile`, `secrets`, `doctor` |

## Key invariants

1. **Credential bytes are opaque.** No package reconstructs the credential
   envelope. We canonicalize JSON only for fingerprinting.
2. **Active detection is fingerprint-based.** No "active pointer" file. The
   live credential's hash is matched against profile fingerprints —
   stable identity hash first (refresh-safe), volatile blob hash as
   fallback for legacy profiles.
3. **No telemetry.** The only outbound call is the optional Anthropic
   usage API (`list --usage`); all other commands are fully local.
4. **Atomicity at the boundary.** Every persistent file (`profiles.json`,
   `active.env`, `secrets.enc`, export bundles, the usage cache) is
   written with temp-file + rename.
5. **Live writes have rollback.** `claude.Bridge.WriteLive` snapshots the
   prior credential, writes the new one, verifies it back, and rolls back
   on mismatch.
6. **`prune` never touches Claude Code itself.** `ccswitch prune` wipes
   our keychain entries + config dir but leaves the system
   `Claude Code-credentials` keychain item and `~/.claude.json` intact.

## Storage layout

```
~/.config/ccswitch/                  # Linux/macOS (XDG)
%AppData%\ccswitch\                  # Windows
├── profiles.json                    # metadata only — never holds creds
├── active.env                       # per-profile env overlay (sourceable)
├── keyring-index.json               # known keyring keys (for List support)
├── secrets.enc                      # encrypted vault (file backend only)
└── cache/
    └── usage.json                   # 30s TTL, invalidated on profile-set change
```

Override the location for testing with `CCSWITCH_CONFIG_DIR=/tmp/whatever`
(see `make sandbox`).

## Concurrency model

- Single user, single process at a time; advisory protection only.
- All writes go through atomic temp+rename — concurrent invocations
  produce one of the writes, never a torn state.
- The `Store` types use a `sync.Mutex` for thread safety within a single
  process.
- `list --usage` fans out to the Anthropic API in parallel (cap 5
  in-flight) and merges results into a 30 s on-disk cache keyed by the
  hash of the profile-name set, so adding/removing profiles auto-invalidates.

## Live credential storage

| OS | Backend | Location |
|---|---|---|
| macOS | Keychain (via `security` CLI) | service `Claude Code-credentials` |
| Linux | File (Claude Code's own format) | `${CLAUDE_CONFIG_DIR:-~/.claude}/.credentials.json` |
| Windows | File (Claude Code's own format) | `%CLAUDE_CONFIG_DIR%\.credentials.json` (else `%USERPROFILE%\.claude\.credentials.json`) |

`ccswitch` profiles live separately under our own service name
(`ccswitch`) so we never conflict with the live entry.

## Identity & active detection

On `add`, `ccswitch` reads `~/.claude.json` (best-effort) to capture:

- `email` and `org_name` — shown in the `IDENTITY` column.
- `stable_fingerprint` — SHA-256 of `accountUuid + orgUuid`, survives
  OAuth token rotation.
- The raw `oauthAccount` JSON block — restored on `use` so
  `claude /status` shows the right account immediately.

`Lookup` resolves a single argument to a profile by trying, in order:
exact name, 1-based index into the alphabetical list, email match. This
powers `ccswitch use you@example.com` and `ccswitch use 2`.

## Crypto

- **File vault and export bundles** share `internal/cryptobox`:
  AES-256-GCM with a key derived via Argon2id (default
  `time=2, memMB=64, threads=4`).
- Magic prefixes (`CCFB` for vault, `CCEX` for export) prevent decrypting
  one as the other.
- Header (magic + version + params + salt + nonce) is bound into the GCM
  auth tag, so any tampering — including with the work-factor params —
  fails verification.

## Running-instance discovery

`list` (and `list --usage`) appends a "Running instances:" footer driven
by Claude Code's own bookkeeping:

- `~/.claude/sessions/{pid}.json` — one per CLI/SDK session (filtered by
  live PID, grouped by `cwd` so multiple sessions in the same project
  collapse to a single row with a session count).
- `~/.claude/ide/{port}.lock` — one per attached IDE (VS Code, Cursor,
  Windsurf), deduped by IDE+workspace.

A missing directory yields no error — the section is simply omitted.

## Quota / usage API

`list --usage` calls Anthropic's quota endpoint per OAuth profile.
Non-OAuth profiles (raw API key, SSO) are silently skipped — the
endpoint requires the OAuth bearer token.

When the active profile's token is rejected (401), `ccswitch` runs
`claude /status` once (with an 8 s timeout) to delegate a token refresh,
then retries. Inactive profiles can't be refreshed without disturbing
the live keychain entry, so they show "usage unavailable" with a hint
to `ccswitch use <name>` after `claude /login`.

## Extending

- Add a new command: implement `cmd/ccswitch/cmd_<name>.go` and register
  in `registry.go`.
- Add a new doctor check: implement under `internal/doctor/checks/` and
  register in `cmd/ccswitch/cmd_doctor_wiring.go`.
- Add a new credential type: extend `internal/claude/envelope.go` —
  detection only; keep envelope bytes opaque.
