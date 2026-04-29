# Troubleshooting

Always start with:

```sh
ccswitch doctor
```

Each line is a check; non-PASS lines come with a `fix:` hint.

## Common situations

### `ClaudeCredential — no live credential present`

You haven't logged into Claude Code yet on this machine, or your previous
session expired and was wiped. Run:

```sh
claude /login
```

Then `ccswitch add <name>` to capture it.

### `FingerprintMatch — live credential is untracked`

You're logged in, but the credential isn't tied to any profile. Capture
it:

```sh
ccswitch add <name>
```

If you're seeing this *after* a `ccswitch use`, something else
overwrote the live credential between commands (Claude itself refreshed
the token, or another tool wrote to the keychain). Run `ccswitch doctor`
again — if the credential matches the profile you switched to, you're
fine.

### `OrphanSecrets — N orphan keyring entry/entries`

Keyring entries that no profile in `profiles.json` knows about. Usually
left over from a manual delete or a crashed `add`. Open Keychain
Access (macOS) or Seahorse (Linux), find entries with service
`ccswitch`, and delete the ones you don't recognize. We don't auto-clean
to avoid losing data.

### `ShellHook — no shell rc file sources active.env` (only with --check-shell)

See [shell-integration.md](./shell-integration.md) for the one-line
addition to your rc.

### Keychain prompts every time on macOS

The first time you `add` or `use`, click **Always Allow**. If you missed
that, open Keychain Access → search "ccswitch" → for each item:
double-click → **Access Control** tab → "Allow all applications".

### File-vault passphrase prompt won't go away

You're using the file backend (no system keyring detected). Each new
shell session asks for the passphrase once. Confirm with:

```sh
ccswitch doctor      # SecretsBackend should say "backend=file"
```

To switch to the system keyring, install / start `gnome-keyring` (Linux)
or use a graphical login session, then re-run `ccswitch`.

### Lost passphrase for an export bundle

There is no recovery path. The bundle is gone. Your local profiles are
intact in the secret store — re-export with a passphrase you'll remember
(consider using a password manager).

### `use` reports success but `claude` still uses the old account

Claude Code caches the token in memory. Restart your `claude` session
after `use`.

### Performance regression on `use`

Run with timing:

```sh
time ccswitch use <name>
```

Expected: under 100 ms. If it's much higher and the SecretsBackend is
`file`, the Argon2id KDF is doing the work — that's the design tradeoff
for the headless-fallback case.

## Filing a bug

Include:

- `ccswitch version`
- `ccswitch doctor --json`
- The exact command you ran
- What you expected, what happened

File at <https://github.com/nhtera/ccswitch/issues>. **Never** include
credential bytes, fingerprints, or the contents of `secrets.enc`.
