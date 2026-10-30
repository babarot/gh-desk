// Package claude asks Claude Code about pull requests and keeps the
// worktrees it reads them in.
//
// It runs Claude Code itself, `claude -p`, so it uses the account the user
// is signed in with and no API key. Everything in a pull request was written
// by someone else and may carry instructions for Claude, and its worktree
// may add Claude Code settings (hooks, permissions, MCP servers). So every
// run is restricted: no settings files are read, no MCP servers are started,
// the file tools see only the worktree, and the tools are the read-only ones
// the config names.
package claude

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/babarot/gh-triage/internal/logging"
)

// Tools are the tools Claude may be given; each only reads.
var Tools = []string{"Read", "Grep", "Glob", "WebFetch"}

// Options configure Claude.
type Options struct {
	// Bin is the claude command; empty is "claude" on PATH.
	Bin   string
	Model string
	// Tools are among Tools; the caller has checked them.
	Tools []string
	// Instructions are appended to the system prompt of every review.
	Instructions string
	Log          *slog.Logger
}

// Claude runs Claude Code.
type Claude struct {
	opt Options
}

// New returns a Claude.
func New(opt Options) *Claude {
	if opt.Bin == "" {
		opt.Bin = "claude"
	}
	return &Claude{opt: opt}
}

// Step is a tool Claude used, and on what.
type Step struct {
	Tool string
	Arg  string
}

// Answer is what Claude said, what it cost as claude reports it (in US
// dollars), and the model that answered.
type Answer struct {
	Text string
	// Structured is the answer in the shape a schema asked for.
	Structured json.RawMessage
	Cost       float64
	Model      string
}

// NewSession returns a new session ID.
func NewSession() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// restricted are the flags every run takes, interactive or not.
func (c *Claude) restricted(tools []string) []string {
	return []string{
		// No settings files (a worktree's .claude/settings.json and its
		// hooks), file tools confined to the worktree, and no tool that
		// runs commands.
		"--restricted",
		// No MCP servers, a worktree's .mcp.json included.
		"--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`,
		"--tools", strings.Join(tools, ","),
		"--disable-slash-commands",
	}
}

// Request is one question in a session.
type Request struct {
	// Dir is the worktree Claude works in.
	Dir string
	// Session is the conversation; First starts it.
	Session string
	First   bool
	Prompt  string
	// Schema, when set, asks for the answer as JSON in this shape.
	Schema string
}

// Ask asks Claude in a session kept by Claude Code, so it can be resumed,
// here or in Claude Code. progress hears of each tool Claude uses.
func (c *Claude) Ask(ctx context.Context, r Request, progress func(Step)) (Answer, error) {
	args := []string{"-p", "--model", c.opt.Model, "--output-format", "stream-json", "--verbose"}
	args = append(args, c.restricted(c.opt.Tools)...)
	args = append(args, "--allowedTools", strings.Join(c.opt.Tools, ","),
		"--append-system-prompt", reviewInstructions+c.extra())
	if r.First {
		args = append(args, "--session-id", r.Session)
	} else {
		args = append(args, "--resume", r.Session)
	}
	if r.Schema != "" {
		args = append(args, "--json-schema", r.Schema)
	}
	log := c.opt.Log.With(logging.KeyClaude, r.Session)
	return c.run(ctx, log, r.Dir, args, r.Prompt, progress)
}

func (c *Claude) extra() string {
	if strings.TrimSpace(c.opt.Instructions) == "" {
		return ""
	}
	return "\n\n" + c.opt.Instructions
}

// Once asks a question with no tools and no session kept, from dir.
func (c *Claude) Once(ctx context.Context, dir, prompt, schema string) (Answer, error) {
	args := []string{"-p", "--model", c.opt.Model, "--output-format", "stream-json", "--verbose", "--no-session-persistence"}
	args = append(args, c.restricted(nil)...)
	if schema != "" {
		args = append(args, "--json-schema", schema)
	}
	return c.run(ctx, c.opt.Log, dir, args, prompt, func(Step) {})
}

// Interactive is Claude Code resuming the session in dir, restricted as
// the runs are; it asks before each tool, as Claude Code does.
func (c *Claude) Interactive(dir, session string) *exec.Cmd {
	args := append([]string{"--resume", session, "--model", c.opt.Model}, c.restricted(c.opt.Tools)...)
	cmd := exec.Command(c.opt.Bin, args...)
	cmd.Dir = dir
	return cmd
}

func (c *Claude) run(ctx context.Context, log *slog.Logger, dir string, args []string, prompt string, progress func(Step)) (Answer, error) {
	bin, err := exec.LookPath(c.opt.Bin)
	if err != nil {
		return Answer{}, errors.New("claude (Claude Code) is not on PATH")
	}
	start := time.Now()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	// The prompt goes in on stdin: a diff can be longer than a command
	// line may be.
	cmd.Stdin = strings.NewReader(prompt)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return Answer{}, err
	}
	if err := cmd.Start(); err != nil {
		return Answer{}, err
	}
	answer, readErr := readStream(out, func(s Step) {
		log.Debug("claude step", "tool", s.Tool)
		progress(s)
	})
	// Read to the end, so claude is not stuck writing.
	_, _ = io.Copy(io.Discard, out)
	waitErr := cmd.Wait()
	elapsed := time.Since(start).Milliseconds()
	switch {
	case ctx.Err() != nil:
		log.Info("claude stopped", logging.KeyDurationMS, elapsed)
		return Answer{}, ctx.Err()
	case readErr != nil:
		log.Warn("claude failed", "err", readErr.Error(), logging.KeyDurationMS, elapsed)
		return Answer{}, readErr
	case waitErr != nil:
		msg := strings.TrimSpace(stderr.String())
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = msg[i+1:]
		}
		log.Warn("claude failed", "err", waitErr.Error(), logging.KeyDurationMS, elapsed)
		return Answer{}, fmt.Errorf("claude: %v %s", waitErr, msg)
	}
	log.Info("claude answered", "model", answer.Model, "cost_usd", answer.Cost, logging.KeyDurationMS, elapsed)
	return answer, nil
}

// event is the part of a stream-json line that is read.
type event struct {
	Type       string                     `json:"type"`
	IsError    bool                       `json:"is_error"`
	Result     string                     `json:"result"`
	Cost       float64                    `json:"total_cost_usd"`
	ModelUsage map[string]json.RawMessage `json:"modelUsage"`
	Message    struct {
		Content []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	} `json:"message"`
	Structured json.RawMessage `json:"structured_output"`
}

// readStream follows claude's stream-json output to its result, reporting
// each tool used.
func readStream(r io.Reader, progress func(Step)) (Answer, error) {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var ev event
			if json.Unmarshal(line, &ev) == nil {
				switch ev.Type {
				case "assistant":
					for _, c := range ev.Message.Content {
						if c.Type == "tool_use" && c.Name != "StructuredOutput" {
							progress(Step{Tool: c.Name, Arg: stepArg(c.Input)})
						}
					}
				case "result":
					if ev.IsError {
						return Answer{}, fmt.Errorf("claude: %s", ev.Result)
					}
					a := Answer{Text: strings.TrimSpace(ev.Result), Cost: ev.Cost}
					if len(ev.Structured) > 0 && string(ev.Structured) != "null" {
						a.Structured = ev.Structured
					}
					// The model that cost the most, when several worked on it.
					top := -1.0
					for name, raw := range ev.ModelUsage {
						var u struct {
							Cost float64 `json:"costUSD"`
						}
						_ = json.Unmarshal(raw, &u)
						if u.Cost > top || u.Cost == top && name < a.Model {
							a.Model, top = name, u.Cost
						}
					}
					return a, nil
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return Answer{}, errors.New("claude ended without an answer")
			}
			return Answer{}, err
		}
	}
}

// stepArg is what a tool was used on: a path, a pattern, a URL.
func stepArg(input json.RawMessage) string {
	var in map[string]any
	_ = json.Unmarshal(input, &in)
	for _, k := range []string{"file_path", "pattern", "url", "path"} {
		if s, ok := in[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}
