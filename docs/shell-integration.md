# Shell Integration

`ccswitch use` writes per-profile environment variables to
`~/.config/ccswitch/active.env`. To make those vars take effect in your
shell, source the file once at startup.

## zsh / bash

Add this to `~/.zshrc` or `~/.bashrc`:

```sh
[ -f ~/.config/ccswitch/active.env ] && . ~/.config/ccswitch/active.env
```

The file is always present after the first `use` (even for profiles with
no env vars — it'll be empty), so the `-f` test is just defense-in-depth
for fresh installs.

## fish

```fish
if test -f ~/.config/ccswitch/active.env
    source ~/.config/ccswitch/active.env
end
```

(`active.env` uses `export KEY=VALUE` lines, which fish accepts via
`source` since fish 3.x.)

## Verification

```sh
ccswitch doctor --check-shell
```

This greps your rc files for the sourcing line. The check is opt-in via
`--check-shell` because the heuristic can false-positive on rc files that
mention `active.env` in comments — review the output if unsure.

## Why a file?

We considered exporting vars directly from the binary (e.g. `eval
"$(ccswitch use work)"`), but that conflates two concerns:

1. The env overlay should persist for *future* commands in the same shell,
   not just the one piped through `eval`.
2. `use` should be safe to run from any shell, scripts, or even a future
   menu bar app. A file gives us one source of truth.

Sourcing is a one-time setup; switches are silent after that.

## Removing the overlay

```sh
ccswitch env <name> --unset KEY
```

If `<name>` is the active profile, `active.env` is rewritten immediately.
For profiles that aren't active, the change takes effect on the next
`ccswitch use <name>`.
