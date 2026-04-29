package main

import (
	"context"

	"github.com/nhtera/ccswitch/internal/claude"
	"github.com/nhtera/ccswitch/internal/profile"
)

// findActiveProfile resolves which stored profile (if any) currently
// owns the live Claude Code credential. Tries the refresh-safe
// stable-fingerprint match first (derived from ~/.claude.json's
// oauthAccount block) so a refreshed access token doesn't drop the
// profile into "untracked" state. Falls back to the volatile blob
// fingerprint for legacy profiles captured before stable
// fingerprints existed.
//
// Returns (profile, true, fingerprint, accountInfo) on match.
// Returns ("", false, fingerprint, accountInfo) when there's a live
// credential but no profile claims it. Returns ("", false, "", nil)
// when there is no live credential at all (caller should check err
// from ReadLive separately).
func findActiveProfile(ctx context.Context, bridge *claude.Bridge, store *profile.Store) (profile.Profile, bool, string, *claude.AccountInfo) {
	blob, err := bridge.ReadLive(ctx)
	if err != nil {
		return profile.Profile{}, false, "", nil
	}
	volatileFp := bridge.Fingerprint(blob)
	info, _ := claude.ReadAccountInfo()

	if info != nil {
		if sfp := info.StableFingerprint(); sfp != "" {
			if p, ok := store.FindByStableFingerprint(sfp); ok {
				return p, true, volatileFp, info
			}
		}
	}
	if p, ok := store.FindByFingerprint(volatileFp); ok {
		return p, true, volatileFp, info
	}
	return profile.Profile{}, false, volatileFp, info
}
