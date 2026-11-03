// Package claude asks Claude Code about pull requests, and for plans of
// what to show and select in the lists, and keeps the worktrees it reads
// pull requests in.
//
// It runs Claude Code itself, `claude -p`, so it uses the account the user
// is signed in with and no API key. Everything in a pull request was written
// by someone else and may carry instructions for Claude, and its worktree
// may add Claude Code settings (hooks, permissions, MCP servers). So every
// run is restricted: no settings files are read, the file tools see only
// the worktree and the skills given to it, and the built-in tools are the read-only ones the config
// names. No MCP server is started and no skill is given but those the user
// configured for the conversation (ADR-033), and no hook runs.
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
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/babarot/gh-triage/internal/logging"
	"github.com/babarot/gh-triage/internal/model"
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
	// Language is what Claude answers in about a pull request; a triage is
	// answered in the language of the request.
	Language string
	// Instructions are appended to the system prompt of every review and
	// triage.
	Instructions string
	// MCPConfig is the file of the MCP servers of a conversation, as
	// WriteMCPConfig wrote it, and MCPTools their tools that may run
	// without a prompt; empty when there are none (ADR-033).
	MCPConfig string
	MCPTools  []string
	// PluginDir is the plugin of the skills of a conversation, as
	// WritePlugin made it; empty when there are none.
	PluginDir string
	Log       *slog.Logger
}

// conversationTools are the tools of a conversation beside the built-in
// ones: the MCP servers', and Skill when there are skills.
func (c *Claude) conversationTools() []string {
	tools := slices.Clone(c.opt.MCPTools)
	if c.opt.PluginDir != "" {
		tools = append(tools, "Skill")
	}
	return tools
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

// guard is what the runs that may take the user's MCP servers and skills
// (the conversation, and Claude Code in the terminal) take whether or not
// any are configured: no hooks, of a plugin or of a skill, none of Claude
// Code's own skills, and no shell commands in a skill (ADR-033).
const guard = `{"disableAllHooks":true,"disableBundledSkills":true,"disableSkillShellExecution":true}`

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
	// PR is the pull request a conversation is about, told to its MCP
	// servers; zero for a triage.
	PR model.PRKey
	// Dir is where Claude runs: the worktree of a review, or a directory
	// away from any repository for a triage. A session is resumed only from
	// the directory it started in.
	Dir string
	// Session is the conversation; First starts it.
	Session string
	First   bool
	Prompt  string
	// Schema, when set, asks for the answer as JSON in this shape.
	Schema string
}

// Ask asks Claude about a pull request in a session kept by Claude Code,
// so it can be resumed, here or in Claude Code. progress hears of each
// tool Claude uses.
func (c *Claude) Ask(ctx context.Context, r Request, progress func(Step)) (Answer, error) {
	args := c.conversation(c.session(r, reviewInstructions+"\n\nAnswer in "+c.opt.Language+".", c.opt.Tools))
	args = append(args, "--allowedTools", strings.Join(append(slices.Clone(c.opt.Tools), c.conversationTools()...), ","))
	return c.run(ctx, c.opt.Log.With(logging.KeyClaude, r.Session), r.Dir, prEnv(r.PR), args, r.Prompt, func(s Step) {
		s.Arg = c.skillPath(s.Arg)
		progress(s)
	})
}

// skillPath is p, when it is in a skill given to Claude, as the skill's
// name and the path in it rather than where gh-triage copied it.
func (c *Claude) skillPath(p string) string {
	if c.opt.PluginDir == "" {
		return p
	}
	rel, ok := strings.CutPrefix(p, filepath.Join(c.opt.PluginDir, "skills")+string(filepath.Separator))
	if !ok {
		return p
	}
	return PluginName + ":" + filepath.ToSlash(rel)
}

// conversation adds to args, the flags of a conversation or of Claude Code
// in the terminal, the guard, the user's MCP servers and the skills.
func (c *Claude) conversation(args []string) []string {
	args = append(args, "--settings", guard)
	if c.opt.MCPConfig != "" {
		i := slices.Index(args, "--mcp-config")
		args[i+1] = c.opt.MCPConfig
	}
	if d := c.opt.PluginDir; d != "" {
		i := slices.Index(args, "--tools")
		args[i+1] = strings.Join(append(slices.Clone(c.opt.Tools), "Skill"), ",")
		args = slices.DeleteFunc(args, func(a string) bool { return a == "--disable-slash-commands" })
		// The directory too, so a skill can read its own files.
		args = append(args, "--plugin-dir", d, "--add-dir", d)
	}
	return args
}

// prEnv is the environment a conversation about k runs in: the pull
// request, for the MCP servers' ${VAR}.
func prEnv(k model.PRKey) []string {
	if k.Owner == "" {
		return nil
	}
	return append(os.Environ(),
		"GH_TRIAGE_OWNER="+k.Owner,
		"GH_TRIAGE_REPO="+k.Repo,
		"GH_TRIAGE_NUMBER="+strconv.Itoa(k.Number),
		"GH_TRIAGE_URL=https://github.com/"+k.Owner+"/"+k.Repo,
	)
}

// Triage asks Claude for a plan of what to show and select in the tab
// shown, as TriageSchema, in a session kept by Claude Code. It has no
// tools: everything Claude needs is in the prompt. Claude answers in the
// language of the request, which the user wrote.
func (c *Claude) Triage(ctx context.Context, r Request) (Answer, error) {
	r.Schema = TriageSchema
	args := c.session(r, triageInstructions, nil)
	return c.run(ctx, c.opt.Log.With(logging.KeyClaude, r.Session), r.Dir, nil, args, r.Prompt, func(Step) {})
}

// session are the arguments of a question in a session, with tools and
// instructions as the system prompt.
func (c *Claude) session(r Request, instructions string, tools []string) []string {
	args := []string{"-p", "--model", c.opt.Model, "--output-format", "stream-json", "--verbose"}
	args = append(args, c.restricted(tools)...)
	args = append(args, "--append-system-prompt", c.system(instructions))
	if r.First {
		args = append(args, "--session-id", r.Session)
	} else {
		args = append(args, "--resume", r.Session)
	}
	if r.Schema != "" {
		args = append(args, "--json-schema", r.Schema)
	}
	return args
}

// system is the system prompt: instructions, and what the config file
// adds.
func (c *Claude) system(instructions string) string {
	if strings.TrimSpace(c.opt.Instructions) == "" {
		return instructions
	}
	return instructions + "\n\n" + c.opt.Instructions
}

// Once asks a question with no tools and no session kept, from dir.
func (c *Claude) Once(ctx context.Context, dir, prompt, schema string) (Answer, error) {
	args := []string{"-p", "--model", c.opt.Model, "--output-format", "stream-json", "--verbose", "--no-session-persistence"}
	args = append(args, c.restricted(nil)...)
	if schema != "" {
		args = append(args, "--json-schema", schema)
	}
	return c.run(ctx, c.opt.Log, dir, nil, args, prompt, func(Step) {})
}

// Interactive is Claude Code resuming the session about pr in dir,
// restricted as the conversation is; it asks before each tool, as Claude
// Code does.
func (c *Claude) Interactive(pr model.PRKey, dir, session string) *exec.Cmd {
	args := c.conversation(append([]string{"--resume", session, "--model", c.opt.Model}, c.restricted(c.opt.Tools)...))
	cmd := exec.Command(c.opt.Bin, args...)
	cmd.Dir = dir
	cmd.Env = prEnv(pr)
	return cmd
}

// run runs claude in dir; env, when not nil, is its whole environment.
func (c *Claude) run(ctx context.Context, log *slog.Logger, dir string, env, args []string, prompt string, progress func(Step)) (Answer, error) {
	bin, err := exec.LookPath(c.opt.Bin)
	if err != nil {
		return Answer{}, fmt.Errorf("%s (Claude Code) is not found; claude.command sets where it is", c.opt.Bin)
	}
	start := time.Now()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	// Stopped, Claude Code is interrupted rather than killed, so it closes
	// the session it keeps, which may be resumed later.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 5 * time.Second
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
		return Answer{}, fmt.Errorf("claude: %w %s", waitErr, msg)
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

// stepArg is what a tool was used on: a path, a pattern, a URL, a skill,
// or what an MCP server was asked.
func stepArg(input json.RawMessage) string {
	var in map[string]any
	_ = json.Unmarshal(input, &in)
	for _, k := range []string{"file_path", "pattern", "url", "path", "skill", "query"} {
		if s, ok := in[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}
