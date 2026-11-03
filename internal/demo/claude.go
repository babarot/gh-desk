package demo

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/babarot/gh-desk/internal/claude"
	"github.com/babarot/gh-desk/internal/model"
)

// Claude is a made-up Claude: it reads a few files of the pull request,
// one step at a time, and gives the answer written for it.
type Claude struct {
	// Dir is where the worktrees would be; nothing is written there.
	Dir string
	// Speed scales every wait, as GitHub's does.
	Speed float64
}

func (c *Claude) wait(ctx context.Context, ms int) error {
	if c.Speed <= 0 {
		return ctx.Err()
	}
	select {
	case <-time.After(time.Duration(float64(ms)*c.Speed) * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Prepare names a worktree without making one.
func (c *Claude) Prepare(ctx context.Context, k model.PRKey, head string) (string, error) {
	return filepath.Join(c.Dir, k.Owner, k.Repo, head), c.wait(ctx, 1200)
}

// Ask reads the files of the retry pull request and answers about it,
// whatever was asked: the demo asks one question.
func (c *Claude) Ask(ctx context.Context, r claude.Request, progress func(claude.Step)) (claude.Answer, error) {
	if err := c.wait(ctx, 1500); err != nil {
		return claude.Answer{}, err
	}
	for _, s := range []claude.Step{
		{Tool: "Read", Arg: "client/retry.go"},
		{Tool: "Read", Arg: "client/client.go"},
		{Tool: "Grep", Arg: "Idempotency-Key"},
		{Tool: "Read", Arg: "client/retry_test.go"},
	} {
		progress(s)
		if err := c.wait(ctx, 900); err != nil {
			return claude.Answer{}, err
		}
	}
	if r.Schema != "" {
		s, _ := json.Marshal(map[string]string{"event": string(model.EventComment), "body": draft})
		return claude.Answer{Structured: s, Cost: 0.12, Model: "claude-opus-5-5"}, nil
	}
	return claude.Answer{Text: answer, Cost: 0.09, Model: "claude-opus-5-5"}, nil
}

// Interactive is a command that ends at once: the demo has no Claude Code
// to open.
func (c *Claude) Interactive(k model.PRKey, dir, session string) *exec.Cmd {
	return exec.Command("true")
}

// Once is the check of the release notes before a merge: one update that
// needs care, whichever were asked about.
func (c *Claude) Once(ctx context.Context, dir, prompt, schema string) (claude.Answer, error) {
	if err := c.wait(ctx, 2500); err != nil {
		return claude.Answer{}, err
	}
	s, _ := json.Marshal(map[string]any{"cautions": []map[string]string{
		{"key": key("infra", 133).String(), "why": "v7 runs on Node 24, which needs runner v2.329 or later on self-hosted runners."},
	}})
	return claude.Answer{Structured: s, Model: "claude-opus-5-5"}, nil
}

// Triage is not part of the demo.
func (c *Claude) Triage(ctx context.Context, r claude.Request) (claude.Answer, error) {
	return claude.Answer{}, errors.New("triage is not part of the demo")
}

const answer = `Mostly, with one gap.

- A POST without an ` + "`Idempotency-Key`" + ` is never retried: ` + "`retryable`" + `
  checks the method first, so a create is not sent twice.
- The body is not rewound. ` + "`Do`" + ` sends the same request again, but the
  first try has read its body, so a PUT goes out empty on the second
  try. Call ` + "`req.GetBody`" + ` before each retry.
- ` + "`Retry-After`" + ` is honored for a 429, as the description says.

The tests cover the status codes, but not a request with a body.`

const draft = "Thanks, this removes a lot of loops around the client. One thing before it goes in: a retried PUT or keyed POST is sent without its body, since `Do` reuses `req` after the first try read it. Could it call `req.GetBody` before each retry, with a test that sends a body?"
