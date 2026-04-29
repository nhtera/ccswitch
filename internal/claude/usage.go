package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// Usage API constants. Mirrors the same Anthropic endpoint and beta
// header that the official Claude Code CLI hits — calling on the
// user's own behalf with their own access token, not telemetry.
const (
	usageAPIURL    = "https://api.anthropic.com/api/oauth/usage"
	oauthBetaValue = "oauth-2025-04-20"
	usageTimeout   = 5 * time.Second
)

// ErrUsageUnauthorized indicates the access token was rejected (401).
// Callers can distinguish "token expired" from generic failures so the
// CLI can suggest a remedy ("run ccswitch use <name> to refresh").
var ErrUsageUnauthorized = errors.New("claude: usage API rejected access token")

// UsageBucket is one rate-limit window's utilization snapshot.
type UsageBucket struct {
	// Pct is the percentage of quota consumed in this window (0-100).
	Pct float64
	// ResetsAt is when the window rolls over. Zero value if API
	// didn't include a timestamp (rare).
	ResetsAt time.Time
}

// UsageResult is the parsed response of /api/oauth/usage. Either field
// may be nil if the API didn't include that window.
type UsageResult struct {
	FiveHour *UsageBucket
	SevenDay *UsageBucket
}

// usageWire is the raw JSON shape Anthropic returns.
type usageWire struct {
	FiveHour *struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"five_hour"`
	SevenDay *struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"seven_day"`
}

// FetchUsage queries the Anthropic usage API for the given OAuth access
// token. The HTTP client is constructed inline so tests can swap it via
// FetchUsageVia. Returns a 5-second timeout regardless of caller ctx.
func FetchUsage(ctx context.Context, accessToken string) (*UsageResult, error) {
	return FetchUsageVia(ctx, http.DefaultClient, accessToken)
}

// FetchUsageVia exposes the underlying HTTP client for dependency
// injection in tests.
func FetchUsageVia(ctx context.Context, client *http.Client, accessToken string) (*UsageResult, error) {
	if accessToken == "" {
		return nil, errors.New("claude: empty access token")
	}
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(ctx, usageTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usageAPIURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("anthropic-beta", oauthBetaValue)
	req.Header.Set("User-Agent", "ccswitch")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUsageUnauthorized
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("claude: usage API status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var wire usageWire
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return nil, fmt.Errorf("claude: decode usage response: %w", err)
	}

	out := &UsageResult{}
	if wire.FiveHour != nil {
		out.FiveHour = &UsageBucket{
			Pct:      wire.FiveHour.Utilization,
			ResetsAt: parseTimestamp(wire.FiveHour.ResetsAt),
		}
	}
	if wire.SevenDay != nil {
		out.SevenDay = &UsageBucket{
			Pct:      wire.SevenDay.Utilization,
			ResetsAt: parseTimestamp(wire.SevenDay.ResetsAt),
		}
	}
	return out, nil
}

// ExtractAccessToken pulls claudeAiOauth.accessToken out of a
// credential blob without reconstructing the envelope. Returns "" if
// the blob is a bare API key, malformed JSON, or missing the field.
func ExtractAccessToken(blob []byte) string {
	var outer struct {
		ClaudeAiOauth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(blob, &outer); err != nil {
		return ""
	}
	return outer.ClaudeAiOauth.AccessToken
}

// ErrClaudeCLIMissing means the `claude` binary isn't on PATH, so
// delegated refresh is unavailable. Callers should fall back to
// "usage unavailable, run `claude /login` to refresh".
var ErrClaudeCLIMissing = errors.New("claude: `claude` binary not on PATH for delegated refresh")

// DelegatedRefresh asks the official Claude CLI to refresh the live
// keychain credential by running `claude /status`. The CLI handles
// its own OAuth refresh internally as a side effect. We don't parse
// stdout — success is just "the command ran without timing out". On
// macOS the live keychain entry will then contain a freshened access
// token that callers can read via Bridge.ReadLive.
//
// Delegating to the CLI is safer than POSTing to the OAuth token
// endpoint with a hard-coded client_id, since we never impersonate
// Claude Code's own credentials.
func DelegatedRefresh(ctx context.Context, timeout time.Duration) error {
	if _, err := exec.LookPath("claude"); err != nil {
		return ErrClaudeCLIMissing
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(rctx, "claude", "/status")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("claude /status: %w", err)
	}
	return nil
}

// FormatReset returns a human-friendly (countdown, clock) pair:
//
//	countdown: "2d 1h", "3h 53m", "12m"
//	clock:     same-day → "22:00", other-day → "May 1 22:00"
//
// Both strings are empty if resetsAt is the zero value.
func FormatReset(resetsAt time.Time, now time.Time) (countdown, clock string) {
	if resetsAt.IsZero() {
		return "", ""
	}
	remaining := resetsAt.Sub(now)
	if remaining < 0 {
		remaining = 0
	}
	totalSec := int(remaining.Seconds())
	days := totalSec / 86400
	hours := (totalSec % 86400) / 3600
	minutes := (totalSec % 3600) / 60
	switch {
	case days > 0:
		countdown = fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		countdown = fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		countdown = fmt.Sprintf("%dm", minutes)
	}

	resetLocal := resetsAt.Local()
	nowLocal := now.Local()
	if resetLocal.Year() == nowLocal.Year() &&
		resetLocal.YearDay() == nowLocal.YearDay() {
		clock = resetLocal.Format("15:04")
	} else {
		clock = resetLocal.Format("Jan 2 15:04")
	}
	return countdown, clock
}

func parseTimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
