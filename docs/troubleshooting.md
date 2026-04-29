# Troubleshooting

Always start with:

```sh
ccswitch doctor
```

Each line is a check; non-PASS lines come with a `fix:` hint. Use
`--verbose` to also show hints on PASS lines, or `--json` for
machine-readable output.

## Common situations

### `ClaudeCredential — no live credential present`

You haven't logged into Claude Code yet on this machine, or your previous
session expired and was wiped. Run:

```sh
claude /login
```

Then `ccswitch add` (name is auto-suggested from `~/.claude.json`).

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
fine. (Active detection prefers the stable identity hash, so token
rotations don't break the marker.)

### `OrphanSecrets — N orphan keyring entry/entries`

Keyring entries that no profile in `profiles.json` knows about. Usually
left over from a manual delete or a crashed `add`. Open Keychain
Access (macOS) or Seahorse (Linux), find entries with service
`ccswitch`, and delete the ones you don't recognize. We don't auto-clean
to avoid losing data.

If you'd rather start completely fresh, `ccswitch prune` wipes
everything ccswitch owns (profiles, secrets, cache, `active.env`)
without touching your Claude Code login.

### `ShellHook — no shell rc file sources active.env` (only with `--check-shell`)

See [shell-integration.md](./shell-integration.md) for the one-line
addition to your rc.

### `list --usage` shows "usage unavailable" for a profile

- For raw API-key or SSO profiles: this is expected. The Anthropic
  quota endpoint is OAuth-only.
- For an inactive OAuth profile: the stored access token is expired
  and refreshing it would require swapping the live credential. Run
  `ccswitch use <name>` and then `claude /login` if needed; the next
  `list --usage` will succeed.
- For the active profile: ccswitch already attempts a one-shot
  delegated refresh via `claude /status` (8 s timeout). If that fails
  too, `claude /login` will rotate the token and unblock subsequent
  fetches.

The usage cache lives at `~/.config/ccswitch/cache/usage.json` and
expires after 30 s; it's also invalidated whenever the profile set
changes (rename / add / remove).

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
after `use`. (`ccswitch list` shows running CLI sessions and IDEs in the
"Running instances:" footer — handy for spotting stale sessions.)

### `next` keeps bouncing back to the same profile

Rotation is driven by `LastUsedAt` (the profile you most recently
switched to via `ccswitch use`), not by live-credential matching. If
you've never used `ccswitch use` since the last `claude /login`, run
it once — then `next` will rotate cleanly.

### Performance regression on `use`

Run with timing:

```sh
time ccswitch use <name>
```

Expected: under 100 ms. If it's much higher and the SecretsBackend is
`file`, the Argon2id KDF is doing the work — that's the design tradeoff
for the headless-fallback case.

### Want to start completely over

```sh
ccswitch prune       # removes profiles, secrets, cache, active.env
```

Leaves Claude Code untouched. After prune, log into the accounts you
want and re-run `ccswitch add` for each.

## Filing a bug

Include:

- `ccswitch version`
- `ccswitch doctor --json`
- The exact command you ran
- What you expected, what happened

File at <https://github.com/nhtera/ccswitch/issues>. **Never** include
credential bytes, fingerprints, or the contents of `secrets.enc`.
