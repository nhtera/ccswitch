# Install

## macOS (recommended)

```sh
brew install nhtera/tap/ccswitch
ccswitch version
ccswitch doctor          # should report all PASS / WARN
```

The first time you `add` or `use`, macOS shows a Keychain prompt
("`ccswitch` wants to use your confidential information stored in your
keychain"). Click **Always Allow** so future runs don't re-prompt.

If you skip Homebrew and grab the binary from Releases, run:

```sh
xattr -d com.apple.quarantine ccswitch     # if Gatekeeper complains
```

## Linux

```sh
curl -L https://github.com/nhtera/ccswitch/releases/latest/download/ccswitch_linux_amd64.tar.gz \
  | tar -xz
sudo install ccswitch /usr/local/bin/
```

`ccswitch` uses libsecret via `gnome-keyring` or `kwallet`. Headless / SSH
sessions usually have neither — `ccswitch` falls back to an encrypted file
vault (AES-256-GCM + Argon2id) and prompts for a passphrase on first use.
Run `ccswitch doctor` to confirm which backend is active.

## Windows

```powershell
# After downloading ccswitch_windows_amd64.zip
Expand-Archive ccswitch_windows_amd64.zip
.\ccswitch.exe version
```

`ccswitch` uses the Windows Credential Manager. Tested less thoroughly than
macOS in v1 — please file issues if anything is rough.

## Development build

```sh
git clone https://github.com/nhtera/ccswitch
cd ccswitch
make build           # → ./bin/ccswitch
./bin/ccswitch doctor
```

Requires Go 1.25 or newer.

Common make targets (`make help` lists them all):

| Target | What it does |
|---|---|
| `make build` | Compile to `./bin/ccswitch` |
| `make install` | `go install` into `$GOBIN` (`~/go/bin` by default) |
| `make run ARGS="..."` | `go run` with arguments, e.g. `make run ARGS="list --usage"` |
| `make sandbox ARGS="..."` | Run with an isolated `CCSWITCH_CONFIG_DIR` (mktemp) — no risk to real data |
| `make test` / `make test-v` | Run tests (`-v` for verbose) |
| `make vet` / `make fmt` / `make tidy` | Static checks, formatting, `go mod tidy` |
| `make dev` | `vet` + `test` + `build` |
| `make release` | Stripped, trimpath release build (`CGO_ENABLED=0`) |
| `make clean` | Remove `bin/` |

Version metadata is injected via `-ldflags '-X main.version=$(VERSION)'`,
where `VERSION` comes from `git describe --tags --always --dirty`.

## Verifying the binary

Each release includes a `checksums.txt` of SHA-256 hashes:

```sh
sha256sum -c checksums.txt --ignore-missing
```

## Uninstall / reset

```sh
ccswitch prune       # wipe ALL ccswitch data (profiles, secrets, cache)
```

`prune` removes the config directory, every per-profile keychain entry,
and the local cache. It does **not** touch your Claude Code login —
`claude /status` still works after a prune.
