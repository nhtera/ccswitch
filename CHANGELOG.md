# Changelog

All notable changes to ccswitch are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [1.0.0] — 2026-04-29

First public release.

### Added
- `ccswitch add <name>` — capture the currently-logged-in Claude Code
  account into a named profile. Supports `--type`, `--note`, `--env`.
- `ccswitch use <name>` — atomically swap the live credential to the named
  profile and rewrite `~/.config/ccswitch/active.env`.
- `ccswitch list` (`ls`) — table or `--json` listing of profiles, with the
  active one marked.
- `ccswitch current` — print the active profile name (or `(untracked)`).
- `ccswitch rename <old> <new>` — rename a profile (keyring + metadata).
- `ccswitch remove <name>` (`rm`) — delete a profile, with confirm.
- `ccswitch env <name>` — `--set / --unset / --list` per-profile env vars;
  rewrites `active.env` immediately if the profile is active.
- `ccswitch export` — encrypt all (or one) profiles to a portable `.cce`
  bundle. AES-256-GCM with Argon2id KDF; passphrase always prompted.
- `ccswitch import <file>` — decrypt a bundle and apply, with `--rename`
  for resolving name conflicts.
- `ccswitch doctor` — read-only diagnostics: platform, secrets backend,
  Claude credential, profiles store, fingerprint match, orphan secrets,
  optional shell-hook check.
- macOS Keychain backend; Linux libsecret / Windows Credential Manager
  via `go-keyring`; AES-GCM encrypted file fallback for headless sessions.
- Single static Go binary; CI matrix (mac/linux/win); Homebrew tap.

### Security
- Credentials never written to plaintext disk except the encrypted file
  fallback (mode 0600). No telemetry or network calls anywhere.

[Unreleased]: https://github.com/nhtera/ccswitch/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/nhtera/ccswitch/releases/tag/v1.0.0
