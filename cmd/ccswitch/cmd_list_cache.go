package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/nhtera/ccswitch/internal/claude"
	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/nhtera/ccswitch/internal/secrets"
)

const (
	usageCacheTTL = 30 * time.Second
	usageCacheKey = "usage"
)

// usageCacheEnvelope is the on-disk shape: timestamp + key (hash of
// the profile name set, so the cache invalidates when profiles are
// added/removed) + the per-profile result blob.
//
// Reusing the same `usageRow` from cmd_list_usage.go would mean
// serializing error values, which Go's JSON can't round-trip. We
// strip the err on write and synthesize a placeholder on read for
// rows that came back empty.
type usageCacheEnvelope struct {
	Timestamp time.Time                  `json:"timestamp"`
	KeySetSha string                     `json:"key_set_sha"`
	Rows      map[string]cachedUsageRow  `json:"rows"`
}

type cachedUsageRow struct {
	// Result, when non-nil, was a successful API response. Nil rows
	// represent failures that we don't bother caching as success but
	// also don't need to refetch within TTL.
	Result *cachedUsage `json:"result,omitempty"`
}

// cachedUsage mirrors claude.UsageResult but with concrete time
// fields that JSON can encode without nesting structs.
type cachedUsage struct {
	FiveHourPct      float64   `json:"five_hour_pct,omitempty"`
	FiveHourResetsAt time.Time `json:"five_hour_resets_at,omitempty"`
	HasFiveHour      bool      `json:"has_five_hour,omitempty"`

	SevenDayPct      float64   `json:"seven_day_pct,omitempty"`
	SevenDayResetsAt time.Time `json:"seven_day_resets_at,omitempty"`
	HasSevenDay      bool      `json:"has_seven_day,omitempty"`
}

// usageCachePath returns the cache file path or an error if config
// dir resolution fails.
func usageCachePath() (string, error) {
	dir, err := secrets.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cache", usageCacheKey+".json"), nil
}

// keySetSha hashes the sorted profile-name set so the cache file
// auto-invalidates when profiles are added/removed/renamed.
func keySetSha(profiles []profile.Profile) string {
	names := make([]string, len(profiles))
	for i, p := range profiles {
		names[i] = p.Name
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		h.Write([]byte(n))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// readUsageCache returns rows in the same order as profiles, or nil
// if the cache is missing, expired, or for a different profile set.
func readUsageCache(profiles []profile.Profile) []usageRow {
	path, err := usageCachePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			// Any other error (permissions, truncation) is treated as a
			// cache miss — fall through to a fresh fetch.
		}
		return nil
	}
	var env usageCacheEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil
	}
	if env.KeySetSha != keySetSha(profiles) {
		return nil
	}
	if time.Since(env.Timestamp) > usageCacheTTL {
		return nil
	}

	rows := make([]usageRow, len(profiles))
	for i, p := range profiles {
		entry, ok := env.Rows[p.Name]
		if !ok || entry.Result == nil {
			// Treat missing entries as "no cached data" — caller can
			// either skip or fetch. We return a sentinel error so
			// callers know they need to re-fetch this row.
			rows[i] = usageRow{Err: errCacheMiss}
			continue
		}
		rows[i] = usageRow{Result: cachedUsageToResult(entry.Result)}
	}
	return rows
}

var errCacheMiss = errors.New("cache miss")

// cachedUsageToResult converts our flat on-disk shape back into the
// nested claude.UsageResult that the renderer expects.
func cachedUsageToResult(c *cachedUsage) *claude.UsageResult {
	if c == nil {
		return nil
	}
	out := &claude.UsageResult{}
	if c.HasFiveHour {
		out.FiveHour = &claude.UsageBucket{Pct: c.FiveHourPct, ResetsAt: c.FiveHourResetsAt}
	}
	if c.HasSevenDay {
		out.SevenDay = &claude.UsageBucket{Pct: c.SevenDayPct, ResetsAt: c.SevenDayResetsAt}
	}
	return out
}

// resultToCachedUsage flattens claude.UsageResult for JSON encoding.
func resultToCachedUsage(r *claude.UsageResult) *cachedUsage {
	if r == nil {
		return nil
	}
	out := &cachedUsage{}
	if r.FiveHour != nil {
		out.HasFiveHour = true
		out.FiveHourPct = r.FiveHour.Pct
		out.FiveHourResetsAt = r.FiveHour.ResetsAt
	}
	if r.SevenDay != nil {
		out.HasSevenDay = true
		out.SevenDayPct = r.SevenDay.Pct
		out.SevenDayResetsAt = r.SevenDay.ResetsAt
	}
	return out
}

// writeUsageCache persists successful rows. Rows with errors are
// recorded as nil entries so a subsequent read shows them as missing
// and re-fetches them, but the rest of the set still benefits from
// the warm cache.
func writeUsageCache(profiles []profile.Profile, rows []usageRow) {
	path, err := usageCachePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	env := usageCacheEnvelope{
		Timestamp: time.Now(),
		KeySetSha: keySetSha(profiles),
		Rows:      make(map[string]cachedUsageRow, len(profiles)),
	}
	for i, p := range profiles {
		row := cachedUsageRow{}
		if rows[i].Err == nil && rows[i].Result != nil {
			row.Result = resultToCachedUsage(rows[i].Result)
		}
		env.Rows[p.Name] = row
	}
	data, err := json.Marshal(env)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}
