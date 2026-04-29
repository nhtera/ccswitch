package main

import (
	"context"

	"github.com/nhtera/ccswitch/internal/claude"
	"github.com/nhtera/ccswitch/internal/profile"
)

// findActiveProfile resolves which stored profile (if any) currently
// owns the live Claude Code credential.
//
// Priority order (intentional):
//  1. Volatile blob fingerprint — matches the EXACT bytes in the
//     keychain right now, so it's the most accurate when the blob is
//     unchanged since capture.
//  2. Stable fingerprint (sha256 accountUuid|orgUuid from
//     ~/.claude.json) — fallback for the legitimate token-refresh
//     case where Claude Code rotated the access token but the
//     account identity is unchanged.
//
// Why volatile first: ~/.claude.json is owned by Claude Code and may
// be stale relative to the live keychain when an external tool
// writes a different account's blob into the keychain WITHOUT
// touching .claude.json. In that window, stable fingerprint would
// match .claude.json's old account → wrong profile. Trusting the
// actual keychain bytes first keeps us correct.
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

	if p, ok := store.FindByFingerprint(volatileFp); ok {
		return p, true, volatileFp, info
	}
	if info != nil {
		if sfp := info.StableFingerprint(); sfp != "" {
			if p, ok := store.FindByStableFingerprint(sfp); ok {
				return p, true, volatileFp, info
			}
		}
	}
	return profile.Profile{}, false, volatileFp, info
}
