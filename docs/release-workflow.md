# Release Workflow

Cutting a release is **commit → tag → push**. Everything else is wired up:
the `.github/workflows/release.yaml` workflow runs on any `v*.*.*` tag,
GoReleaser builds 5 platform archives, publishes a GitHub release, and
pushes an updated formula to [`nhtera/homebrew-tap`][tap].

## Quick path

```sh
# Working tree clean, on main, CI green.
git tag -a v1.0.1 -m "v1.0.1 — <one-line summary>"
git push origin v1.0.1
gh run watch --exit-status      # ~1.5 min
```

That's the whole flow. Users on Homebrew pick up the new version with
`brew update && brew upgrade ccswitch`.

## Versioning

Standard semver. What changed dictates the bump:

| Change | Bump | Example |
|---|---|---|
| Bug fix only, no behavior change | patch | `v1.0.0` → `v1.0.1` |
| New backwards-compatible feature | minor | `v1.0.1` → `v1.1.0` |
| Breaking change (removed flag, renamed config, on-disk format change) | major | `v1.x.x` → `v2.0.0` |
| Pre-release for testing | suffix | `v1.1.0-rc1`, `v2.0.0-beta2` |

Pre-release tags are auto-flagged via `prerelease: auto` in
`.goreleaser.yaml` — they appear on the Releases page as
"Pre-release" and **do not** become "Latest", so `brew install` keeps
serving the previous stable until you ship a clean semver tag.

## Pre-tag polish (optional)

These steps make the release nicer but aren't required:

### 1. Smoke-test the build locally

Catches `.goreleaser.yaml` mistakes without burning a tag:

```sh
brew install goreleaser            # one-time
goreleaser release --snapshot --clean --skip=publish
ls dist/                           # inspect the would-be tarballs
```

### 2. Update `CHANGELOG.md` (optional)

GoReleaser auto-generates the release notes from git log (filtered by
your `.goreleaser.yaml` `changelog.filters.exclude`), so a manually
curated `CHANGELOG.md` is optional. If you maintain one, commit it
with the release-prep commit before tagging:

```sh
$EDITOR CHANGELOG.md
git add CHANGELOG.md
git commit -m "release: prep v1.0.1"
git push
```

### 3. Annotate the tag with a real summary

The tag message shows up in `git tag -l -n5` and `git log --tags`. A
real summary helps teammates skim the history:

```sh
git tag -a v1.0.1 -m "v1.0.1 — fix usage cache key collision

- list --usage no longer shows stale data after profile rename
- doctor: warn when CLAUDE_CONFIG_DIR points outside \$HOME"
```

## What the workflow actually does

On `git push origin v*.*.*`:

1. `release.yaml` fires on `macos-latest`.
2. Checks out the tag with `fetch-depth: 0` (full history needed by
   GoReleaser to build the changelog).
3. Sets up Go 1.25 with module cache.
4. Runs `goreleaser release --clean`, which:
   - Builds 5 archives — `darwin_{amd64,arm64}`, `linux_{amd64,arm64}`,
     `windows_amd64` — with `-trimpath -s -w -X main.version=...`.
   - Generates `checksums.txt` (SHA-256, one line per artifact).
   - Creates the GitHub release with the changelog block (commit hash
     + subject line, no author email — see `changelog.use: git`).
   - Pushes `ccswitch.rb` to `nhtera/homebrew-tap` using the
     `HOMEBREW_TAP_TOKEN` secret.

Total runtime: ~1.5 minutes on a cold cache.

## When things go wrong

### Bad tag, never installed by anyone yet

```sh
git tag -d v1.0.1                        # delete locally
git push --delete origin v1.0.1          # delete remote tag
gh release delete v1.0.1 --yes           # if release lingered
# fix the code, then:
git tag -a v1.0.1 -m "..."
git push origin v1.0.1
```

### Bad tag, already shipped to Homebrew users

**Don't retag.** Homebrew pins formulas by SHA-256, so a silent retag
breaks every cached install. Bump to the next patch (`v1.0.2`) instead
and ship the fix forward.

### Workflow ran, release exists, no assets attached

Usually means a partial GoReleaser run (often: tap-token expired
mid-flight). Easiest recovery:

```sh
gh release delete v1.0.1 --yes
git push --delete origin v1.0.1
# fix the broken bit, retag, repush
```

The workflow's `--clean` flag handles whatever local state remained.

### Workflow didn't fire at all

```sh
git ls-remote --tags origin              # confirm tag actually pushed
gh workflow view release.yaml            # confirm tag pattern matches
```

The `on.push.tags` filter in `release.yaml` is `v*.*.*` — tags missing
the `v` prefix or with non-numeric suffixes won't trigger.

## Convenience: shell helper

If you cut releases often, drop this in `~/.zshrc`:

```sh
ccrelease() {
  [ -z "$1" ] && { echo "usage: ccrelease <version> [-m message]"; return 1; }
  local v="$1"; shift
  [[ "$v" != v* ]] && v="v$v"
  git tag -a "$v" "$@" && git push origin "$v"
}
```

Then: `ccrelease 1.0.1 -m "fix usage cache"`.

## Secrets & maintenance

### `HOMEBREW_TAP_TOKEN`

Used by GoReleaser to push the formula to `nhtera/homebrew-tap`.
Currently set to a personal `gh` OAuth token (broad scope but works).
Rotating to a fine-grained PAT is a one-liner once you have one:

1. Generate at <https://github.com/settings/tokens?type=beta>:
   - Repository access: `nhtera/homebrew-tap` only
   - Permissions: **Contents: Read and write**, **Metadata: Read** (auto)
2. Replace the secret:
   ```sh
   gh secret set HOMEBREW_TAP_TOKEN --repo nhtera/ccswitch --body "github_pat_…"
   ```

### Move formula to `Formula/` subdir

Cosmetic. `brew install` works either way, but the `Formula/` layout
is conventional. Add to the `brews:` block in `.goreleaser.yaml`:

```yaml
brews:
  - name: ccswitch
    directory: Formula
    repository:
      ...
```

Affects only the next release; existing `ccswitch.rb` at the tap repo
root is harmless.

### macOS Apple-signing + notarization

Comment block in `.goreleaser.yaml` points to `gon` or
`goreleaser-pro`'s `notarize.macos` block. Wire up only if Gatekeeper
friction with raw-tarball downloads becomes a real issue —
`brew install` users won't see a quarantine prompt either way (Homebrew
strips it).

## Verifying the install end-to-end

```sh
# Fresh shell, no ccswitch installed.
brew tap nhtera/tap
brew install ccswitch
ccswitch version                    # → ccswitch 1.0.1
shasum -a 256 "$(brew --prefix)/bin/ccswitch"
# Should match the checksum row in checksums.txt on the release page.
```

[tap]: https://github.com/nhtera/homebrew-tap
