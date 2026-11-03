package playground

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/babarot/gh-desk/internal/claude"
	"github.com/babarot/gh-desk/internal/model"
)

// Claude answers from a script: a review written in advance for each pull
// request, and plans made from a few words of the request. It reads the
// tools it pretends to use from the pull request's files.
type Claude struct{ w *World }

func (w *World) Claude() Claude { return Claude{w} }

// cost is what each answer says it cost, so the screen shows one.
const cost = 0.04

func (c Claude) Prepare(ctx context.Context, key model.PRKey, _ string) (string, error) {
	if err := c.wait(ctx, 900*time.Millisecond); err != nil {
		return "", err
	}
	return "~/.cache/gh-desk/worktrees/" + key.Owner + "/" + key.Repo + "/" + fmt.Sprint(key.Number), nil
}

// wait waits d, or until ctx is done.
func (c Claude) wait(ctx context.Context, d time.Duration) error {
	done := make(chan struct{})
	go func() { c.w.pause(d); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c Claude) Ask(ctx context.Context, r claude.Request, progress func(claude.Step)) (claude.Answer, error) {
	c.w.mu.Lock()
	p := c.w.find(r.PR)
	var files []string
	var answer, followUp string
	var draft draftReview
	if p != nil {
		files, answer, followUp, draft = slices.Clone(p.files), p.answer, p.followUp, p.draft
	}
	c.w.mu.Unlock()
	if p == nil {
		return claude.Answer{}, fmt.Errorf("%s is not in the demo", r.PR)
	}
	if r.Schema == claude.DraftSchema {
		if err := c.wait(ctx, 2500*time.Millisecond); err != nil {
			return claude.Answer{}, err
		}
		if draft.event == "" {
			draft = draftReview{model.EventComment, "Thanks for this."}
		}
		b, _ := json.Marshal(map[string]string{"event": string(draft.event), "body": draft.body})
		return claude.Answer{Text: draft.body, Structured: b, Cost: cost, Model: "claude-opus"}, nil
	}
	steps := []claude.Step{}
	for i, f := range files {
		if i == 3 {
			break
		}
		steps = append(steps, claude.Step{Tool: "Read", Arg: f})
		if i == 0 {
			steps = append(steps, claude.Step{Tool: "Grep", Arg: grepFor(f)})
		}
	}
	if !r.First {
		steps = steps[:min(1, len(steps))]
	}
	if err := c.wait(ctx, 1200*time.Millisecond); err != nil {
		return claude.Answer{}, err
	}
	for _, s := range steps {
		progress(s)
		if err := c.wait(ctx, 900*time.Millisecond); err != nil {
			return claude.Answer{}, err
		}
	}
	text := answer
	if !r.First || !isReviewQuestion(r.Prompt) {
		text = followUp
	}
	return claude.Answer{Text: text, Cost: cost, Model: "claude-opus"}, nil
}

// isReviewQuestion reports whether the prompt ends with the question a
// review starts with.
func isReviewQuestion(prompt string) bool {
	return strings.HasSuffix(strings.TrimSpace(prompt), claude.ReviewQuestion)
}

// grepFor is what Claude pretends to search for around a file.
func grepFor(file string) string {
	base := file[strings.LastIndex(file, "/")+1:]
	if i := strings.Index(base, "."); i > 0 {
		base = base[:i]
	}
	return base
}

// Interactive is Claude Code in the terminal, which the browser cannot
// run; the TUI says it could not start.
func (c Claude) Interactive(model.PRKey, string, string) *exec.Cmd {
	return exec.Command("claude", "--resume")
}

var notesKey = regexp.MustCompile(`<pull_request key="([^"]+)">`)

// Once answers the question of the release notes: a major update needs
// care.
func (c Claude) Once(ctx context.Context, _ string, prompt, schema string) (claude.Answer, error) {
	if err := c.wait(ctx, 3*time.Second); err != nil {
		return claude.Answer{}, err
	}
	if schema != claude.NotesSchema {
		return claude.Answer{Text: "", Structured: json.RawMessage(`{}`), Cost: cost}, nil
	}
	type caution struct {
		Key string `json:"key"`
		Why string `json:"why"`
	}
	got := struct {
		Cautions []caution `json:"cautions"`
	}{Cautions: []caution{}}
	for _, m := range notesKey.FindAllStringSubmatch(prompt, -1) {
		if m[1] == "acme/web#1293" {
			got.Cautions = append(got.Cautions, caution{m[1], "vite 7 needs Node.js 20.19 or later, and changes the default build target."})
		}
		if m[1] == "acme/api#485" {
			got.Cautions = append(got.Cautions, caution{m[1], "actions/checkout v5 runs on Node 24, which self-hosted runners need to have."})
		}
	}
	b, _ := json.Marshal(got)
	return claude.Answer{Structured: b, Cost: cost, Model: "claude-opus"}, nil
}

// row is a row of the list given with a triage, as much as the script
// reads of it.
type row struct {
	key    string
	group  string
	bot    bool
	ci     string
	review string
	size   int
	mine   string
	draft  bool
}

var rowField = regexp.MustCompile(`(\w+)=("(?:[^"\\]|\\.)*"|\S+)`)

func parseRows(prompt string) []row {
	_, list, _ := strings.Cut(prompt, "<list>\n")
	list, _, _ = strings.Cut(list, "</list>")
	var rows []row
	for l := range strings.SplitSeq(list, "\n") {
		key, _, _ := strings.Cut(l, " ")
		if _, err := model.ParsePRKey(key); err != nil {
			continue
		}
		r := row{key: key, bot: strings.Contains(l, " (bot)"), draft: strings.HasSuffix(l, " draft") || strings.Contains(l, " draft ")}
		for _, m := range rowField.FindAllStringSubmatch(l, -1) {
			switch m[1] {
			case "group":
				r.group = m[2]
			case "ci":
				r.ci = m[2]
			case "review":
				r.review = m[2]
			case "mine":
				r.mine = m[2]
			case "size":
				var a, d int
				if _, err := fmt.Sscanf(m[2], "+%d-%d", &a, &d); err == nil {
					r.size = a + d
				}
			}
		}
		rows = append(rows, r)
	}
	return rows
}

func requestOf(prompt string) string {
	_, req, _ := strings.Cut(prompt, "<request>\n")
	req, _, _ = strings.Cut(req, "</request>")
	return strings.ToLower(strings.TrimSpace(req))
}

// Triage makes a plan from a few words of the request: bots, small,
// failing, approve, merge. Anything else gets a reply saying what the
// script knows.
func (c Claude) Triage(ctx context.Context, r claude.Request) (claude.Answer, error) {
	if err := c.wait(ctx, 2500*time.Millisecond); err != nil {
		return claude.Answer{}, err
	}
	rows := parseRows(r.Prompt)
	req := requestOf(r.Prompt)
	has := func(words ...string) bool {
		return slices.ContainsFunc(words, func(w string) bool { return strings.Contains(req, w) })
	}
	pick := func(ok func(row) bool) []string {
		var keys []string
		for _, r := range rows {
			if ok(r) {
				keys = append(keys, r.key)
			}
		}
		return keys
	}
	var reply string
	var actions []claude.Action
	switch {
	case has("bot", "renovate", "dependabot", "dependenc", "deps", "ボット", "依存"):
		keys := pick(func(r row) bool { return r.bot && r.ci == "success" })
		if len(keys) == 0 {
			reply = "No bot pull request with passing checks is in this tab."
			break
		}
		reply = fmt.Sprintf("%d bot pull requests have passing checks. I selected them", len(keys))
		actions = []claude.Action{{Do: "clear_selection"}, {Do: "unfold", Group: "bots"}, {Do: "select", Keys: keys}}
		if has("merge", "マージ") {
			reply += " and opened the merge popup; nothing is merged until you confirm there."
			actions = append(actions, claude.Action{Do: "merge", Keys: keys})
		} else if has("approve", "承認") {
			reply += " and opened the approve popup; nothing is sent until you confirm there."
			actions = append(actions, claude.Action{Do: "approve", Keys: keys})
		} else {
			reply += "; press M to merge them, or A to approve."
		}
	case has("small", "tiny", "小さい"):
		keys := pick(func(r row) bool { return r.size > 0 && r.size <= 30 })
		reply = fmt.Sprintf("%d pull requests change 30 lines or fewer; I selected them.", len(keys))
		actions = []claude.Action{{Do: "clear_selection"}, {Do: "select", Keys: keys}}
	case has("fail", "red", "broken", "落ち", "失敗"):
		keys := pick(func(r row) bool { return r.ci == "failure" })
		if len(keys) == 0 {
			reply = "No pull request in this tab has failing checks."
			break
		}
		reply = fmt.Sprintf("%d pull requests have failing checks. The cursor is on the first, with its overview.", len(keys))
		actions = []claude.Action{{Do: "select", Keys: keys}, {Do: "cursor", Key: keys[0]}, {Do: "pane", Mode: "overview"}}
	case has("approve", "承認"):
		keys := pick(func(r row) bool { return r.ci == "success" && r.mine != "approved" && !r.draft })
		reply = fmt.Sprintf("%d pull requests have passing checks and no approval of yours. The approve popup is open; nothing is sent until you confirm.", len(keys))
		actions = []claude.Action{{Do: "clear_selection"}, {Do: "select", Keys: keys}, {Do: "approve", Keys: keys}}
	default:
		reply = "This is the demo, so I answer from a script. Try \"merge the bots with passing checks\", \"select the small ones\" or \"which ones are failing?\"."
	}
	b, _ := json.Marshal(map[string]any{"reply": reply, "actions": actionsOrEmpty(actions)})
	return claude.Answer{Text: reply, Structured: b, Cost: cost, Model: "claude-opus"}, nil
}

func actionsOrEmpty(as []claude.Action) []claude.Action {
	if as == nil {
		return []claude.Action{}
	}
	return as
}
