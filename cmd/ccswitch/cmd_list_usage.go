package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/nhtera/ccswitch/internal/claude"
	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/nhtera/ccswitch/internal/secrets"
)

// usageRow is the per-profile result of an attempted usage fetch.
type usageRow struct {
	Result *claude.UsageResult
	Err    error
}

// fetchUsageForProfiles fans out to the Anthropic usage API in
// parallel (capped at 5 in-flight) and returns one row per profile in
// the same order as the input. Network/IO errors per row are
// captured in usageRow.Err — never propagated, so one failing profile
// doesn't kill the whole listing.
//
// Cache-aware: warm rows from disk are returned without a network
// call; only cache-miss rows trigger fetches. After fetching, the
// cache is rewritten with the union of warm + freshly-fetched rows.
//
// Delegated refresh: the active profile (whose credential blob IS the
// live keychain entry) gets one retry via `claude /status` if its
// access token returns 401. Inactive profiles can't be refreshed
// without disturbing the live keychain, so they just show "usage
// unavailable" with a hint.
func fetchUsageForProfiles(ctx context.Context, profiles []profile.Profile, secStore secrets.Store) []usageRow {
	rows := make([]usageRow, len(profiles))
	if len(profiles) == 0 {
		return rows
	}

	store, _ := profile.LoadStore()
	bridge := claude.NewDefaultBridge()
	activeName := ""
	if store != nil {
		if active, ok, _, _ := findActiveProfile(ctx, bridge, store); ok {
			activeName = active.Name
		}
	}

	cached := readUsageCache(profiles)
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	for i, p := range profiles {
		// API is OAuth-only; bare api keys / SSO can't query this
		// endpoint. Mark them with a sentinel and skip.
		if p.Type != string(claude.TypeOAuth) {
			rows[i] = usageRow{Err: errSkipNonOAuth}
			continue
		}
		// Cache hit: skip the network call.
		if cached != nil && cached[i].Err == nil && cached[i].Result != nil {
			rows[i] = cached[i]
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, p profile.Profile) {
			defer wg.Done()
			defer func() { <-sem }()
			rows[i] = fetchOneUsage(ctx, p, secStore, p.Name == activeName, bridge)
		}(i, p)
	}
	wg.Wait()
	writeUsageCache(profiles, rows)
	return rows
}

var errSkipNonOAuth = errors.New("usage available only for oauth profiles")

func fetchOneUsage(ctx context.Context, p profile.Profile, secStore secrets.Store, isActive bool, bridge *claude.Bridge) usageRow {
	blob, err := secStore.Get(ctx, profile.SecretKey(p.Name))
	if err != nil {
		return usageRow{Err: err}
	}
	token := claude.ExtractAccessToken(blob)
	if token == "" {
		return usageRow{Err: errors.New("no access token in stored credential")}
	}
	res, err := claude.FetchUsage(ctx, token)
	if err == nil {
		return usageRow{Result: res}
	}
	// Delegated refresh: only worth attempting for the active profile,
	// because `claude /status` refreshes the live keychain entry
	// (which IS the active profile's blob). For inactive profiles,
	// running it would just refresh whatever the live cred currently
	// is — a different account.
	if !errors.Is(err, claude.ErrUsageUnauthorized) || !isActive || bridge == nil {
		return usageRow{Err: err}
	}
	if rerr := claude.DelegatedRefresh(ctx, 8*time.Second); rerr != nil {
		// Refresh attempt itself failed — surface the original 401.
		return usageRow{Err: err}
	}
	freshBlob, rerr := bridge.ReadLive(ctx)
	if rerr != nil {
		return usageRow{Err: err}
	}
	freshToken := claude.ExtractAccessToken(freshBlob)
	if freshToken == "" || freshToken == token {
		// Refresh didn't actually rotate the token — treat as failure.
		return usageRow{Err: err}
	}
	// Persist the refreshed blob back to the secret store so the
	// next invocation has a fresh starting point. Best-effort: a
	// failed write doesn't poison the row.
	_ = secStore.Set(ctx, profile.SecretKey(p.Name), freshBlob)
	res, err = claude.FetchUsage(ctx, freshToken)
	if err != nil {
		return usageRow{Err: err}
	}
	return usageRow{Result: res}
}

// renderUsageRows prints the 5h/7d lines under each profile that has
// data. Falls back to "usage unavailable" on error. The intended
// layout is one block per profile: the caller prints an identity row,
// then this prints the indented usage lines beneath it.
func renderUsageRows(w io.Writer, rows []usageRow, profiles []profile.Profile) {
	now := time.Now()
	for i, r := range rows {
		if r.Err != nil {
			if errors.Is(r.Err, errSkipNonOAuth) {
				continue
			}
			hint := ""
			if errors.Is(r.Err, claude.ErrUsageUnauthorized) {
				hint = fmt.Sprintf(" (run `ccswitch use %s` after `claude /login` to refresh)", profiles[i].Name)
			}
			fmt.Fprintf(w, "       usage unavailable%s\n", hint)
			continue
		}
		if r.Result == nil {
			continue
		}
		printUsageLine(w, "5h", r.Result.FiveHour, now)
		printUsageLine(w, "7d", r.Result.SevenDay, now)
	}
}

func printUsageLine(w io.Writer, label string, b *claude.UsageBucket, now time.Time) {
	if b == nil {
		return
	}
	countdown, clock := claude.FormatReset(b.ResetsAt, now)
	switch {
	case clock == "" && countdown == "":
		fmt.Fprintf(w, "       %s: %3.0f%%\n", label, b.Pct)
	default:
		fmt.Fprintf(w, "       %s: %3.0f%%   resets %-12s  in %s\n", label, b.Pct, clock, countdown)
	}
}
