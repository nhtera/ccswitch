# CCSwitch — Product Requirements Document

**Status:** Final v1.0 (open questions resolved)
**Owner:** nhtera
**Date:** 2026-04-29
**Source:** [docs/ideas.md](./ideas.md)

---

## 1. Problem Statement

Claude Code stores a single OAuth credential at a time (macOS Keychain entry `Claude Code-credentials`; `~/.claude/.credentials.json` on Linux/Windows). Users with multiple accounts (personal Pro, work Max, Console API for scripting, Enterprise SSO) must `claude /logout` → `claude /login` repeatedly to switch — slow, browser-roundtrip every time, easy to mix up which account is active.

Existing tools fall into two camps:
- **claude-swap** — close to what we want, but inactive/limited polish.
- **ccs (kaitranntt)** — overengineered: multi-provider proxy, dashboard, OAuth control center.

**Gap:** A simple, fast, single-binary, Claude-Code-only account switcher with a clear path to a native-feeling macOS menu bar.

## 2. Goals & Non-Goals

### Goals (v1)
- One command to switch active Claude Code account: `ccswitch use <name>`.
- Capture currently-logged-in credentials into a named profile.
- List, rename, remove, export, import profiles.
- Show which account is currently active.
- Support OAuth (Pro/Max), Console API key, Enterprise SSO accounts.
- Single static Go binary, ~5 MB, zero runtime dependencies.
- macOS first; Linux/Windows builds work but lower-priority polish.

### Non-Goals (v1)
- Implementing our own OAuth flow (we never replace `claude /login`).
- Multi-provider routing (Codex, Copilot, Bedrock, Vertex, OpenRouter…).
- Per-shell concurrent multi-account sessions.
- Per-profile `settings.json` / agents / skills / MCPs (credentials only).
- Proxy layer, request routing, analytics dashboard.
- Auto-token-refresh logic (Claude Code itself owns refresh).

### Phase 2 (post-v1)
- macOS menu bar GUI (`ccswitch menubar`) in same Go binary, systray-based.
- Launchd auto-start on login.
- Account usage indicators (last-used timestamp).

### Phase 3 (later)
- Windows / Linux distribution polish.
- Optional per-profile `settings.json` overlay.
- Quota / usage display if Anthropic exposes an endpoint.

## 3. Users & Use Cases

| Persona | Need |
|---|---|
| Solo dev with personal + work accounts | Switch in <2s without browser. |
| Consultant | 3–5 client accounts, fast rotation. |
| Team lead | Backup/restore profile to share with new hire (with own creds). |
| API + interactive user | One Console API key profile + one OAuth profile. |

### Primary user journey
```
$ claude /login                      # native flow → browser auth
$ ccswitch add work                  # capture current token
$ claude /logout && claude /login    # second account
$ ccswitch add personal              # capture
$ ccswitch list
  * personal  (active)  pro       captured 2026-04-29
    work                max       captured 2026-04-29
$ ccswitch use work                  # swap
$ claude                             # uses work
```

## 4. Functional Requirements

### 4.1 Commands (v1)

| Command | Behavior |
|---|---|
| `ccswitch add <name> [--type oauth\|api\|sso] [--note "..."] [--env KEY=VAL ...]` | Snapshot currently-active credential into profile `<name>`. Optional `--env` attaches per-profile environment variables (e.g. `ANTHROPIC_BASE_URL` for Enterprise + custom gateway). Errors if no active credential. |
| `ccswitch use <name>` | Write profile `<name>`'s credential into the live Keychain entry / `.credentials.json`. Also writes `~/.config/ccswitch/active.env` with the profile's env vars (sourceable from shell rc). Becomes active. **No `--launch` flag** — composes via `ccswitch use X && claude`. |
| `ccswitch env <name> [--set KEY=VAL] [--unset KEY] [--list]` | Manage per-profile env vars after creation. |
| `ccswitch list` (alias `ls`) | Show profiles, type, active marker, capture date, optional note. JSON via `--json`. |
| `ccswitch current` | Print active profile name (or detected fingerprint if untracked). Exit 1 if none. |
| `ccswitch rename <old> <new>` | Rename profile. |
| `ccswitch remove <name>` (alias `rm`) | Delete profile from store. Confirms unless `--force`. |
| `ccswitch export [--out <file>] [--profile <name>]` | Export one or all profiles to encrypted file (passphrase prompt). |
| `ccswitch import <file>` | Import encrypted bundle. |
| `ccswitch doctor` | Diagnose: Keychain accessible? `claude` on PATH? credential format known? |
| `ccswitch version` | Print version + build info. |

### 4.2 Account types & credential format

The credential is a single JSON envelope:

```json
{ "claudeAiOauth": { "accessToken": "sk-ant-oat01-...", "refreshToken": "sk-ant-ort01-...",
  "expiresAt": 1775212290694, "scopes": ["..."], "subscriptionType": "max", "rateLimitTier": "default_claude_ai" } }
```

**Treat the entire envelope as opaque bytes** — snapshot/restore verbatim, never reconstruct fields. We parse only for fingerprinting (SHA-256) and type detection. This makes us forward-compatible if Anthropic adds new fields. `doctor` validates the envelope shape.

| Type | Detection rule | Notes |
|---|---|---|
| `oauth` | `claudeAiOauth.subscriptionType` ∈ {`pro`, `max`} | Default Pro/Max users. |
| `api` | bare `sk-ant-api...` string OR profile carries `ANTHROPIC_API_KEY` env var | Console API users. |
| `sso` | `claudeAiOauth.subscriptionType` indicates enterprise OR profile has `ANTHROPIC_BASE_URL` env | SSO uses identical credential format to OAuth; custom-gateway tenants use the env overlay. |

`add` auto-detects; user can override with `--type`.

### 4.3 Storage layout

```
~/.config/ccswitch/                  # Linux/macOS (XDG)
%AppData%\ccswitch\                  # Windows
├── profiles.json                    # metadata only: name, type, created, last_used, note, fingerprint, env
└── active.env                       # written by `use`; sourceable by shell rc; contains profile env vars
```

`profiles.json` schema (per profile):
```json
{
  "name": "work",
  "type": "sso",
  "created_at": "2026-04-29T03:13:05Z",
  "last_used_at": "2026-04-29T10:42:11Z",
  "note": "Acme Corp Enterprise",
  "fingerprint": "sha256:...",
  "env": { "ANTHROPIC_BASE_URL": "https://gw.acme.com" }
}
```

**Sensitive credential bytes never written to plaintext disk.** They live in:
- macOS: Keychain service `ccswitch.profile.<name>` (one entry per profile, plus the live `Claude Code-credentials` swap target).
- Linux: libsecret via `keyring` (gnome-keyring / KWallet).
- Windows: Credential Manager.
- Fallback (headless Linux): `~/.config/ccswitch/profiles/<name>.cred` mode 0600, AES-256-GCM with a key derived from a user passphrase set on first run.

### 4.4 Active-account detection

`current` works by comparing a SHA-256 fingerprint of the live credential payload against fingerprints stored in `profiles.json`. No need to keep an "active" pointer that can drift.

### 4.5 Switching algorithm (`use`)

1. Read profile `<name>`'s credential blob from secret store.
2. Read live `Claude Code-credentials` entry; if its fingerprint matches no known profile, prompt: *"current account isn't tracked — capture as profile? [y/N]"* (avoids data loss).
3. Atomically write profile blob into live entry.
4. Write profile's env map to `~/.config/ccswitch/active.env` (or empty file if none) — atomic temp+rename.
5. Update `last_used_at` in `profiles.json`.

**Shell integration for env overlay:** users add `[ -f ~/.config/ccswitch/active.env ] && source ~/.config/ccswitch/active.env` to `.zshrc` / `.bashrc` once. Profiles without env vars produce an empty `active.env` (idempotent source).

### 4.7 Encrypted export

- Format: AES-256-GCM, key derived via Argon2id from a fresh user passphrase.
- **Always prompts for a new passphrase** — never reuses keyring-stored secrets. Predictable, no hidden state.
- Bundle includes credential blobs + per-profile env vars + metadata. Versioned envelope for forward compat.
- Lost passphrase = unrecoverable bundle (documented prominently); local profiles still intact.

### 4.6 Add-account algorithm (`add`)

1. Read live credential.
2. Detect type (presence of `accessToken` → oauth/sso; bare key string → api).
3. Compute fingerprint; reject if a profile with same fingerprint already exists (suggest `rename`).
4. Write blob to secret store under `ccswitch.profile.<name>`.
5. Append metadata to `profiles.json`.

## 5. Non-Functional Requirements

| Concern | Requirement |
|---|---|
| Performance | `use` completes in <100 ms (no network). |
| Binary size | ≤ 8 MB stripped, single static binary per OS/arch. |
| Security | No plaintext credentials on disk except encrypted-at-rest fallback; profiles.json contains no secrets. |
| Privacy | **Zero telemetry, ever.** No network calls anywhere in the CLI. `doctor` is fully offline. No opt-in option in v1 (revisit only if user demand emerges). |
| Reliability | Atomic credential writes; rollback if Keychain write fails mid-op. |
| Compatibility | Tested against latest Claude Code stable + previous minor. Document supported version range. |
| OS support | macOS 13+ (arm64 + x86_64). Linux x86_64 and Windows x86_64 build but unsupported polish in v1. |

## 6. Architecture

```
                 ┌─────────────────────────────┐
                 │       ccswitch (Go)         │
                 │  cmd/ccswitch/main.go       │
                 └──────────────┬──────────────┘
                                │ cobra commands
        ┌───────────────────────┼─────────────────────────┐
        ▼                       ▼                         ▼
 ┌─────────────┐        ┌──────────────┐         ┌──────────────────┐
 │  profiles   │        │   secrets    │         │  claude-bridge   │
 │  (JSON      │        │  (keychain / │         │ (read/write live │
 │   metadata) │        │   libsecret /│         │  credential entry│
 │             │        │   credman)   │         │  for Claude Code)│
 └─────────────┘        └──────────────┘         └──────────────────┘
```

### Key Go libraries (proposed)
- CLI: `github.com/spf13/cobra`
- Keyring: `github.com/zalando/go-keyring`
- Encryption fallback: `crypto/aes` + `golang.org/x/crypto/argon2`
- Phase 2 systray: `github.com/getlantern/systray` or `fyne.io/fyne/v2`

### File ownership boundaries
```
cmd/ccswitch/         # entry point + command wiring
internal/profile/     # profiles.json read/write, fingerprinting, env overlay
internal/secrets/     # keyring abstraction (mac/linux/win/file fallback)
internal/claude/      # locate + manipulate live Claude Code credential envelope
internal/envfile/     # active.env writer (atomic temp+rename)
internal/export/      # AES-256-GCM bundle import/export, Argon2id KDF
internal/doctor/      # diagnostics (Keychain, claude on PATH, envelope shape, shell hook)
```

## 7. UX Details

### List output (default)
```
NAME       TYPE  ACTIVE  LAST USED         NOTE
personal   pro   *       2026-04-29 03:12  -
work       max           2026-04-28 18:20  client A
api-bot    api           2026-04-25 10:00  scripting
```

### Errors
- Clear, actionable: *"No profile named 'wrok'. Did you mean 'work'? Run `ccswitch list` to see all profiles."*
- `doctor` prints checklist with PASS/FAIL/WARN per check.

### Shell completion
Generated via cobra: `ccswitch completion zsh|bash|fish`.

## 8. Distribution

- **v1 primary:** Homebrew tap (`brew install nhtera/tap/ccswitch`).
- **v1 secondary:** Direct binary downloads from GitHub Releases (mac arm64/x86_64, linux x86_64, win x86_64).
- **Later:** Homebrew core formula (after stability), Scoop bucket, AUR.
- Code signing for macOS binary (Apple Developer ID); notarized for Gatekeeper.

## 9. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Anthropic changes credential schema in Keychain | Med | High | Detect via fingerprint + structure check; `doctor` reports schema mismatch; pin tested versions in README. |
| Keychain prompts annoy users | High | Med | Document one-time "Always Allow" approval; `doctor` explains. |
| User loses passphrase for encrypted export | Med | Med | Re-derive impossible by design; doc warns clearly; profiles still recoverable from local keyring. |
| Concurrent `claude` sessions confused after `use` | Low | Low | Documented limitation; v2 considers per-shell mode. |
| SSH / headless mac sessions can't access Keychain | Med | Med | Fallback to file-based encrypted store; `doctor` detects and recommends. |
| Naming collision with `cc` / `ccs` | Low | Low | Binary `ccswitch`, repo `CCSwitch`. Both available. |

## 10. Success Metrics

- **Adoption:** 100+ GitHub stars within 3 months of release.
- **Performance:** P95 `use` latency < 150 ms on M-series Mac.
- **Reliability:** Zero credential-loss bug reports in first 30 days.
- **Quality:** Test coverage > 70% on `internal/profile`, `internal/secrets`, `internal/claude`.
- **Polish:** `ccswitch doctor` returns all-PASS on a clean install.

## 11. Out-of-Scope Reminders

- We do **not** call Anthropic APIs.
- We do **not** intercept or proxy Claude Code traffic.
- We do **not** implement OAuth ourselves.
- We do **not** swap `~/.claude/settings.json`, agents, skills, or MCPs in v1.

## 12. Roadmap

| Phase | Timeline | Deliverable |
|---|---|---|
| **v0.1 — Skeleton** | Week 1 | Repo scaffolding, cobra wiring, `version`, `doctor` (mac only). |
| **v0.2 — Capture & Switch** | Week 2 | `add`, `use`, `list`, `current` working end-to-end on mac. |
| **v0.3 — Manage** | Week 3 | `rename`, `remove`, `export`, `import`. |
| **v1.0 — Release** | Week 4 | Homebrew tap, signed binary, README, demo gif, changelog. |
| **v1.1 — Linux/Win polish** | Month 2 | libsecret + Credential Manager hardening. |
| **v2.0 — Menu bar (mac)** | Month 3 | `ccswitch menubar` via systray, launchd plist, account switch from tray. |

## 13. Resolved Decisions Log

| # | Question | Decision | Rationale |
|---|---|---|---|
| 1 | Auto-launch claude after `use`? | **No** `--launch` flag. | Keep `use` single-purpose; users compose `ccswitch use X && claude`. Smaller surface, easier to script. |
| 2 | Per-version Keychain schema adapters? | **Treat envelope as opaque bytes.** | Snapshot/restore verbatim; parse only for fingerprint + type detect. Forward-compatible by default. |
| 3 | Enterprise SSO custom endpoint persistence? | **Per-profile env overlay** in v1 (`active.env` + shell source). | SSO uses identical credential format; custom-gateway tenants need only `ANTHROPIC_BASE_URL`-style env. |
| 4 | Encrypted export passphrase strategy? | **Always prompt fresh** (AES-256-GCM, Argon2id KDF). | Predictable, no hidden state, audit-friendly. |
| 5 | Telemetry / error reporting? | **Zero, ever.** | Tool handles secrets — strong privacy posture is core to trust. |

## 14. Remaining Unresolved Questions

*(Non-blocking — to be revisited during implementation or v1.1.)*

- **Atomicity on macOS Keychain:** Does the `keyring` lib expose a true atomic replace, or do we need write-new + delete-old + rename pattern with rollback?
- **Empty active.env behavior:** Should we `rm` the file when switching to an env-less profile, or write empty file? (Empty file is more idempotent for shell sourcing — leaning empty file.)
- **Doctor heuristic for shell hook:** How to detect that user's `.zshrc` / `.bashrc` sources `active.env` without false positives? (Probably opt-in `ccswitch doctor --check-shell` rather than heuristic.)

---

**Approval:** Ready for implementation planning.
