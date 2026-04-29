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
 │ profiles.json        │ keyring +    │         │ live credential  │
 │ + env meta  │        │ file fallback│         │ I/O + envelope   │
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
| `cmd/ccswitch` | cobra wiring, prompts, subcommand glue | every internal pkg |
| `internal/profile` | profiles.json shape + CRUD, validation, fingerprint conflict checks | `secrets` (for ConfigDir) |
| `internal/secrets` | keyring abstraction, file fallback, key index | `cryptobox` |
| `internal/claude` | live credential read/write per OS, envelope canonicalization, type detection | (none) |
| `internal/envfile` | atomic `active.env` write, POSIX quoting | (none) |
| `internal/cryptobox` | AES-256-GCM + Argon2id helpers | (none) |
| `internal/export` | bundle build/seal/open/apply | `cryptobox`, `profile`, `secrets` |
| `internal/doctor` | check framework + Result/Runner + renderers | (none) |
| `internal/doctor/checks` | concrete checks (live cred, profiles store, ...) | `claude`, `profile`, `secrets`, `doctor` |

## Key invariants

1. **Credential bytes are opaque.** No package reconstructs the credential
   envelope. We canonicalize JSON only for fingerprinting.
2. **Active detection is fingerprint-based.** No "active pointer" file. The
   live credential's hash is matched against profile fingerprints.
3. **No telemetry, no network.** The binary makes no outbound connections.
4. **Atomicity at the boundary.** Every persistent file (`profiles.json`,
   `active.env`, `secrets.enc`, export bundles) is written with
   temp-file + rename.
5. **Live writes have rollback.** `claude.Bridge.WriteLive` snapshots the
   prior credential, writes the new one, verifies it back, and rolls back
   on mismatch.

## Storage layout

```
~/.config/ccswitch/                  # Linux/macOS (XDG)
%AppData%\ccswitch\                  # Windows
├── profiles.json                    # metadata only — never holds creds
├── active.env                       # per-profile env overlay (sourceable)
├── keyring-index.json               # known keyring keys (for List support)
└── secrets.enc                      # encrypted vault (file backend only)
```

## Concurrency model

- Single user, single process at a time; advisory protection only.
- All writes go through atomic temp+rename — concurrent invocations
  produce one of the writes, never a torn state.
- The `Store` types use a `sync.Mutex` for thread safety within a single
  process.

## Live credential storage

| OS | Backend | Location |
|---|---|---|
| macOS | Keychain (via `security` CLI) | service `Claude Code-credentials` |
| Linux | File (Claude Code's own format) | `${CLAUDE_CONFIG_DIR:-~/.claude}/.credentials.json` |
| Windows | File (Claude Code's own format) | `%CLAUDE_CONFIG_DIR%\.credentials.json` (else `%USERPROFILE%\.claude\.credentials.json`) |

`ccswitch` profiles live separately under our own service name
(`ccswitch`) so we never conflict with the live entry.

## Crypto

- **File vault and export bundles** share `internal/cryptobox`:
  AES-256-GCM with a key derived via Argon2id (default
  `time=2, memMB=64, threads=4`).
- Magic prefixes (`CCFB` for vault, `CCEX` for export) prevent decrypting
  one as the other.
- Header (magic + version + params + salt + nonce) is bound into the GCM
  auth tag, so any tampering — including with the work-factor params —
  fails verification.

## Extending

- Add a new command: implement `cmd/ccswitch/cmd_<name>.go` and register
  in `registry.go`.
- Add a new doctor check: implement under `internal/doctor/checks/` and
  register in `cmd/ccswitch/cmd_doctor_wiring.go`.
- Add a new credential type: extend `internal/claude/envelope.go` —
  detection only; keep envelope bytes opaque.
