// Package doctor runs read-only diagnostics against the local environment.
//
// Each Check is a stateless function returning a Result. The Runner executes
// every registered check and returns a Report.
package doctor

import (
	"context"
	"runtime"
)

// Status of a single check outcome.
type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

// Result is the outcome of a single check.
type Result struct {
	Name     string `json:"name"`
	Status   Status `json:"status"`
	Message  string `json:"message,omitempty"`
	FixHint  string `json:"fix_hint,omitempty"`
}

// Check runs a diagnostic and returns its Result. Implementations MUST NOT
// mutate state. Implementations should respect ctx cancellation.
type Check interface {
	Name() string
	Run(ctx context.Context) Result
}

// CheckFunc adapts an inline function into a Check.
type CheckFunc struct {
	N  string
	Fn func(ctx context.Context) Result
}

func (c CheckFunc) Name() string                   { return c.N }
func (c CheckFunc) Run(ctx context.Context) Result { return c.Fn(ctx) }

// Report aggregates Results from a Runner pass.
type Report struct {
	Checks  []Result `json:"checks"`
	Summary Summary  `json:"summary"`
}

// Summary counts statuses.
type Summary struct {
	Pass int `json:"pass"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
	Skip int `json:"skip"`
}

// Runner executes a set of checks in sequence.
type Runner struct {
	checks []Check
}

// NewRunner creates a Runner with no checks registered.
func NewRunner() *Runner { return &Runner{} }

// Register adds checks to the runner.
func (r *Runner) Register(checks ...Check) { r.checks = append(r.checks, checks...) }

// Run executes every registered check and returns a populated Report.
func (r *Runner) Run(ctx context.Context) Report {
	rep := Report{}
	for _, c := range r.checks {
		res := c.Run(ctx)
		if res.Name == "" {
			res.Name = c.Name()
		}
		rep.Checks = append(rep.Checks, res)
		switch res.Status {
		case StatusPass:
			rep.Summary.Pass++
		case StatusWarn:
			rep.Summary.Warn++
		case StatusFail:
			rep.Summary.Fail++
		case StatusSkip:
			rep.Summary.Skip++
		}
	}
	return rep
}

// PlatformCheck reports the OS / arch and whether it's a supported target.
func PlatformCheck() Check {
	return CheckFunc{
		N: "Platform",
		Fn: func(ctx context.Context) Result {
			supported := map[string]bool{
				"darwin/arm64":  true,
				"darwin/amd64":  true,
				"linux/amd64":   true,
				"linux/arm64":   true,
				"windows/amd64": true,
			}
			plat := runtime.GOOS + "/" + runtime.GOARCH
			if supported[plat] {
				return Result{Status: StatusPass, Message: plat}
			}
			return Result{
				Status:  StatusFail,
				Message: plat + " is not a supported target",
				FixHint: "build from source or use a supported platform (darwin/linux/windows on amd64/arm64)",
			}
		},
	}
}
