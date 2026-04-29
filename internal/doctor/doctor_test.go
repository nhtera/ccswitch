package doctor

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunner_AggregatesStatuses(t *testing.T) {
	r := NewRunner()
	r.Register(
		CheckFunc{N: "alpha", Fn: func(ctx context.Context) Result { return Result{Status: StatusPass} }},
		CheckFunc{N: "beta", Fn: func(ctx context.Context) Result { return Result{Status: StatusWarn, Message: "yellow"} }},
		CheckFunc{N: "gamma", Fn: func(ctx context.Context) Result { return Result{Status: StatusFail, Message: "red"} }},
		CheckFunc{N: "delta", Fn: func(ctx context.Context) Result { return Result{Status: StatusSkip} }},
	)
	rep := r.Run(context.Background())
	if rep.Summary.Pass != 1 || rep.Summary.Warn != 1 || rep.Summary.Fail != 1 || rep.Summary.Skip != 1 {
		t.Fatalf("unexpected summary: %+v", rep.Summary)
	}
	if got, want := len(rep.Checks), 4; got != want {
		t.Fatalf("got %d results, want %d", got, want)
	}
	if rep.Checks[0].Name != "alpha" {
		t.Fatalf("expected name to be filled in from check, got %q", rep.Checks[0].Name)
	}
}

func TestPlatformCheck_KnownTarget(t *testing.T) {
	res := PlatformCheck().Run(context.Background())
	if res.Status != StatusPass {
		t.Fatalf("running tests on a known platform should PASS, got %s (%s)", res.Status, res.Message)
	}
}

func TestRenderText_IncludesFixHintOnFail(t *testing.T) {
	rep := Report{
		Checks: []Result{
			{Name: "X", Status: StatusFail, Message: "broken", FixHint: "fix it"},
			{Name: "Y", Status: StatusPass, Message: "ok", FixHint: "no-op"},
		},
		Summary: Summary{Pass: 1, Fail: 1},
	}
	var buf bytes.Buffer
	RenderText(&buf, rep, false)
	out := buf.String()
	if !strings.Contains(out, "fix: fix it") {
		t.Fatalf("expected fix hint on FAIL, got:\n%s", out)
	}
	if strings.Contains(out, "fix: no-op") {
		t.Fatalf("non-verbose should hide PASS hints, got:\n%s", out)
	}
}

func TestRenderText_VerboseIncludesPassHints(t *testing.T) {
	rep := Report{
		Checks:  []Result{{Name: "Y", Status: StatusPass, Message: "ok", FixHint: "shown"}},
		Summary: Summary{Pass: 1},
	}
	var buf bytes.Buffer
	RenderText(&buf, rep, true)
	if !strings.Contains(buf.String(), "fix: shown") {
		t.Fatalf("verbose mode should include PASS hint, got:\n%s", buf.String())
	}
}

func TestRenderJSON_ProducesValidJSON(t *testing.T) {
	rep := Report{
		Checks:  []Result{{Name: "X", Status: StatusPass, Message: "ok"}},
		Summary: Summary{Pass: 1},
	}
	var buf bytes.Buffer
	if err := RenderJSON(&buf, rep); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if !strings.Contains(buf.String(), `"pass": 1`) {
		t.Fatalf("expected JSON to mention pass count, got:\n%s", buf.String())
	}
}
