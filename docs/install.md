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
vault and prompts for a passphrase on first use. Run `ccswitch doctor` to
confirm which backend is active.

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
go build ./cmd/ccswitch
./ccswitch doctor
```

Requires Go 1.22 or newer.

## Verifying the binary

Each release includes a `checksums.txt` of SHA-256 hashes:

```sh
sha256sum -c checksums.txt --ignore-missing
```
