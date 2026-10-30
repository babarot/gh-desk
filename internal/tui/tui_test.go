package tui

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/model"
)

var update = flag.Bool("update", false, "rewrite the golden files")

var now = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func key(owner, repo string, n int) model.PRKey {
	return model.PRKey{Owner: owner, Repo: repo, Number: n}
}

// fixture is made-up data: no real organization's pull requests.
func fixture() *fakeSource {
	me := model.Reviewer{Login: "me"}
	team := model.Reviewer{Login: "core", Team: true}
	prs := []model.PR{
		{ID: "1", Key: key("acme", "api", 12), Title: "Add retries to the client", Author: "alice", State: model.StateOpen,
			HeadRef: "retries", HeadOID: "aaaaaaa1", BaseRef: "main", Additions: 40, Deletions: 3, ChangedFiles: 2,
			Decision: model.DecisionReviewRequired, Checks: model.CheckSuccess, Requested: []model.Reviewer{me, team},
			Updated: now.Add(-3 * time.Hour), URL: "https://github.com/acme/api/pull/12"},
		{ID: "2", Key: key("acme", "web", 7), Title: "Rework the settings page", Author: "bob", State: model.StateOpen, Draft: true,
			HeadRef: "settings", HeadOID: "bbbbbbb2", BaseRef: "main", Additions: 300, Deletions: 120, ChangedFiles: 9,
			Decision: model.DecisionReviewRequired, Checks: model.CheckPending, Requested: []model.Reviewer{team},
			Updated: now.Add(-50 * 24 * time.Hour)},
		{ID: "3", Key: key("acme", "api", 30), Title: "Update dependency octo/app to v0.13.2", Author: "renovate", AuthorIsBot: true,
			State: model.StateOpen, HeadRef: "renovate/octo-app", HeadOID: "ccccccc3", BaseRef: "main", Additions: 1, Deletions: 1, ChangedFiles: 1,
			Decision: model.DecisionReviewRequired, Checks: model.CheckFailure, Requested: []model.Reviewer{me},
			Reviews: []model.Review{{Author: "carol", State: model.ReviewApproved}}, Updated: now.Add(-20 * time.Minute)},
	}
	mine := []model.PR{
		{ID: "4", Key: key("me", "tool", 3), Title: "Document the flags", Author: "me", State: model.StateOpen,
			HeadRef: "docs", HeadOID: "ddddddd4", BaseRef: "main", Additions: 10, Deletions: 2, ChangedFiles: 1,
			Decision: model.DecisionReviewRequired, Checks: model.CheckSuccess, Updated: now.Add(-24 * time.Hour)},
	}
	notifs := []model.Notification{
		{ThreadID: "n1", Key: key("acme", "api", 12), Title: "Add retries to the client", Reason: "review_requested", Unread: true, Updated: now.Add(-3 * time.Hour)},
		{ThreadID: "n2", Key: key("acme", "api", 9), Title: "Old change", Reason: "mention", Unread: true, Updated: now.Add(-48 * time.Hour),
			LatestCommentURL: "https://api.github.com/repos/acme/api/issues/comments/1"},
	}
	return &fakeSource{
		searches: map[string][]model.PR{"review": prs, "mine": mine, "involved": nil},
		notifs:   notifs,
		extra:    map[model.PRKey]model.PR{key("acme", "api", 9): {Key: key("acme", "api", 9), Title: "Old change", State: model.StateMerged}},
		diffs: map[model.PRKey]string{
			key("acme", "api", 12): "diff --git a/client.go b/client.go\n--- a/client.go\n+++ b/client.go\n@@ -1,3 +1,4 @@ package client\n import \"net/http\"\n-var tries = 1\n+var tries = 3\n+var wait = time.Second\n" +
				"diff --git a/go.sum b/go.sum\n--- a/go.sum\n+++ b/go.sum\n@@ -1 +1 @@\n-x v1\n+x v2\n",
			key("acme", "api", 30): "diff --git a/deploy.yml b/deploy.yml\n--- a/deploy.yml\n+++ b/deploy.yml\n@@ -1 +1 @@\n-  ref: v0.12.0\n+  ref: v0.13.2\n",
		},
		details: map[model.PRKey]model.Detail{
			key("acme", "api", 12): {Body: "## What\n\nRetry [failed](https://example.com) requests.", Checks: []model.Check{{Name: "test", State: model.CheckSuccess}}},
			key("acme", "api", 30): {Body: "| Package | Update | Change | Pending |\n|---|---|---|---|\n| octo/app | minor | `v0.12.0` → `v0.13.2` | `v0.13.3` |\n"},
		},
		comments: map[string]model.Comment{"https://api.github.com/repos/acme/api/issues/comments/1": {Author: "dave", Body: "Does this cover the timeout?"}},
	}
}

type fakeSource struct {
	searches map[string][]model.PR
	notifs   []model.Notification
	extra    map[model.PRKey]model.PR
	diffs    map[model.PRKey]string
	details  map[model.PRKey]model.Detail
	comments map[string]model.Comment

	reviews []string
	done    []string
}

func (f *fakeSource) Viewer(context.Context) (string, error) { return "me", nil }

// SearchPage pages through a search two at a time, so tests cover the
// later pages.
func (f *fakeSource) SearchPage(_ context.Context, q string, first int, after string) ([]model.PR, int, string, []string, error) {
	prs := f.searches[q]
	from := 0
	if after != "" {
		fmt.Sscan(after, &from)
	}
	to := min(from+min(first, 2), len(prs))
	next := ""
	if to < len(prs) {
		next = fmt.Sprint(to)
	}
	return prs[from:to], len(prs), next, nil, nil
}
func (f *fakeSource) Notifications(context.Context, int) ([]model.Notification, error) {
	return f.notifs, nil
}
func (f *fakeSource) PullRequests(_ context.Context, keys []model.PRKey) (map[model.PRKey]model.PR, error) {
	out := map[model.PRKey]model.PR{}
	for _, k := range keys {
		if p, ok := f.extra[k]; ok {
			out[k] = p
		}
		for _, prs := range f.searches {
			for _, p := range prs {
				if p.Key == k {
					out[k] = p
				}
			}
		}
	}
	return out, nil
}
func (f *fakeSource) Detail(_ context.Context, k model.PRKey) (model.Detail, error) {
	return f.details[k], nil
}
func (f *fakeSource) Diff(_ context.Context, k model.PRKey) ([]byte, error) {
	return []byte(f.diffs[k]), nil
}
func (f *fakeSource) Comment(_ context.Context, url string) (model.Comment, error) {
	return f.comments[url], nil
}
func (f *fakeSource) Review(_ context.Context, p model.PR, ev model.ReviewEvent, body string) error {
	f.reviews = append(f.reviews, p.Key.String()+" "+string(ev)+" "+p.HeadOID+" "+body)
	return nil
}
func (f *fakeSource) MarkDone(_ context.Context, n model.Notification) error {
	f.done = append(f.done, n.Key.String())
	return nil
}
func (f *fakeSource) Browse(string) error { return nil }

// start makes the TUI over f, sized, with every tab read.
func start(t *testing.T, f *fakeSource, km keymap.Map) *Model {
	t.Helper()
	set := Settings{
		Sections: []Section{
			{Name: "review", Query: "review", Grouped: true, Folded: []string{"team", "bots"}, Limit: 100},
			{Name: "mine", Query: "mine", DraftsLast: true, Limit: 100},
			{Name: "involved", Query: "involved", Limit: 100},
		},
		InboxLimit: 100, HideClosed: true, DoneAfterReview: true, HomeOwners: []string{"acme"}, StaleDays: 30,
		PreviewHeight: 10, Collapse: []string{"go.sum"}, Theme: "mocha", Now: func() time.Time { return now },
	}
	m := New(f, km, nil, set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	run(m, m.Init())
	return m
}

// run carries out cmd and every command its messages lead to, at once.
func run(m *Model, cmd tea.Cmd) {
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		msg := c()
		switch msg := msg.(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case tea.QuitMsg:
		default:
			_, next := m.Update(msg)
			queue = append(queue, next)
		}
	}
}

// keyMsg is a key press as Bubble Tea reports one written k.
func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	}
	if c, ok := strings.CutPrefix(k, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: []rune(c)[0], Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
}

func press(m *Model, keys ...string) {
	for _, k := range keys {
		_, cmd := m.Update(keyMsg(k))
		run(m, cmd)
	}
}

func screen(m *Model) string { return ansi.Strip(m.render()) }

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/tui -update)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from %s:\n%s", name, path, got)
	}
}

func TestReviewTab(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	golden(t, "review", screen(m))
	press(m, "b", "G")
	golden(t, "review-bots", screen(m))
}

func TestPreviewDescription(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "v")
	golden(t, "preview-description", screen(m))
}

func TestPRScreen(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "enter")
	golden(t, "pr-overview", screen(m))
	press(m, "l")
	golden(t, "pr-files", screen(m))
}

func TestInbox(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	press(m, "4")
	golden(t, "inbox", screen(m))
	press(m, "a", "j")
	if s := screen(m); !strings.Contains(s, "Does this cover the timeout?") || !strings.Contains(s, "merged") {
		t.Errorf("closed notification or its comment not shown:\n%s", s)
	}
	press(m, "d")
	if len(f.done) != 1 || f.done[0] != "acme/api#9" {
		t.Errorf("done = %v", f.done)
	}
}

func TestApprove(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	press(m, "A")
	if !strings.Contains(screen(m), "send approve?") {
		t.Fatalf("no confirm popup:\n%s", screen(m))
	}
	press(m, "esc")
	if len(f.reviews) != 0 {
		t.Fatal("esc sent the review")
	}
	press(m, "A", "y")
	if len(f.reviews) != 1 || f.reviews[0] != "acme/api#12 APPROVE aaaaaaa1 " {
		t.Errorf("reviews = %q", f.reviews)
	}
	if len(f.done) != 1 || f.done[0] != "acme/api#12" {
		t.Errorf("notification not marked done: %v", f.done)
	}
	if !strings.Contains(screen(m), "me✓") {
		t.Errorf("row does not show the approval:\n%s", screen(m))
	}
}

func TestCommentWithBody(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	press(m, "c", "ctrl+s")
	if !strings.Contains(m.status, "write the review first") {
		t.Errorf("empty comment was not refused: %q", m.status)
	}
	run(m, m.paste(tea.PasteMsg{Content: "Looks good, one question."}))
	press(m, "ctrl+s", "y")
	if len(f.reviews) != 1 || !strings.HasSuffix(f.reviews[0], "COMMENT aaaaaaa1 Looks good, one question.") {
		t.Errorf("reviews = %q", f.reviews)
	}
	if m.view != viewList {
		t.Error("the writing screen stayed open")
	}
}

func TestOwnPullRequest(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	press(m, "2", "A")
	if m.confirm != nil || !strings.Contains(m.status, "cannot approve your own") {
		t.Errorf("approve of own PR not refused: %q", m.status)
	}
}

func TestReboundKeys(t *testing.T) {
	km, ps := keymap.Apply(keymap.Default(), map[string][]string{"approve": {"a"}})
	if len(ps) > 0 {
		t.Fatal(ps)
	}
	f := fixture()
	m := start(t, f, km)
	press(m, "A")
	if m.confirm != nil {
		t.Error("A still approves after approve was rebound to a")
	}
	press(m, "a")
	if m.confirm == nil {
		t.Error("a does not approve")
	}
}
