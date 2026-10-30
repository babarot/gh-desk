package tui

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-triage/internal/claude"
	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/plan"
	"github.com/babarot/gh-triage/internal/runner"
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
	browsed []string
	// info overrides what MergeInfo says of a pull request, by key.
	info map[model.PRKey]func(*model.MergeInfo)
	// noHeavy has Heavy return nothing, and searchErr fails every search.
	noHeavy   bool
	searchErr error
}

func (f *fakeSource) MergeInfo(_ context.Context, ids []string, _ bool) (map[string]model.MergeInfo, error) {
	out := map[string]model.MergeInfo{}
	for _, prs := range f.searches {
		for _, p := range prs {
			for _, id := range ids {
				if p.ID != id {
					continue
				}
				mi := model.MergeInfo{Key: p.Key, ID: p.ID, State: model.StateOpen, HeadOID: p.HeadOID, MergeState: model.MergeClean,
					Rollup: p.Checks, Checks: []model.Check{}, Files: []string{}, Reviews: p.Reviews,
					Repo: model.RepoMerge{Methods: []model.MergeMethod{model.MethodMerge}}}
				if fn := f.info[p.Key]; fn != nil {
					fn(&mi)
				}
				out[id] = mi
			}
		}
	}
	return out, nil
}

// Heavy reads the heavy fields from the pull requests as the fixture has
// them.
func (f *fakeSource) Heavy(_ context.Context, ids []string) (map[string]model.Heavy, error) {
	out := map[string]model.Heavy{}
	if f.noHeavy {
		// GitHub returned none of them.
		return out, nil
	}
	for _, prs := range f.searches {
		for _, p := range prs {
			for _, id := range ids {
				if p.ID == id {
					p.HeavyHead = p.HeadOID
					out[id] = p.Heavy()
				}
			}
		}
	}
	return out, nil
}

func (f *fakeSource) Bodies(_ context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, prs := range f.searches {
		for _, p := range prs {
			for _, id := range ids {
				if p.ID == id {
					out[id] = f.details[p.Key].Body
				}
			}
		}
	}
	return out, nil
}

// fakeRunner records the batches it is given and ends each pull request
// with the result results says, Approved or Merged by default.
type fakeRunner struct {
	kinds   []runner.Kind
	items   [][]runner.Item
	results map[model.PRKey]runner.Event
	resumed []model.Pending
}

func (r *fakeRunner) Resume(ctx context.Context, ps []model.Pending) (string, <-chan runner.Event) {
	var items []runner.Item
	for _, p := range ps {
		items = append(items, runner.Item{PR: model.PR{ID: p.ID, Key: p.Key}, Head: p.Head})
	}
	r.resumed = append(r.resumed, ps...)
	return r.Start(ctx, runner.Merge, items)
}

func (r *fakeRunner) Start(_ context.Context, kind runner.Kind, items []runner.Item) (string, <-chan runner.Event) {
	r.kinds = append(r.kinds, kind)
	r.items = append(r.items, items)
	ch := make(chan runner.Event, len(items))
	for _, it := range items {
		ev, ok := r.results[it.PR.Key]
		if !ok {
			ev = runner.Event{Result: runner.Merged, Text: "merged", Method: model.MethodMerge}
			if kind == runner.Approve {
				ev = runner.Event{Result: runner.Approved, Text: "approved", Approved: true}
			}
		}
		ev.Key = it.PR.Key
		ch <- ev
	}
	close(ch)
	return fmt.Sprintf("b%d", len(r.kinds)), ch
}

func (f *fakeSource) Viewer(context.Context) (string, error) { return "me", nil }

// SearchPage pages through a search two at a time, so tests cover the
// later pages.
func (f *fakeSource) SearchPage(_ context.Context, q string, first int, after string) ([]model.PR, int, string, []string, error) {
	if f.searchErr != nil {
		return nil, 0, "", nil, f.searchErr
	}
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
	// As the real client does, every field is read.
	for k, p := range out {
		p.HeavyHead = p.HeadOID
		out[k] = p
	}
	return out, nil
}
func (f *fakeSource) Detail(_ context.Context, k model.PRKey) (model.Detail, error) {
	return f.details[k], nil
}
func (f *fakeSource) Diff(_ context.Context, p model.PR) ([]byte, error) {
	return []byte(f.diffs[p.Key]), nil
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
func (f *fakeSource) Browse(url string) error {
	f.browsed = append(f.browsed, url)
	return nil
}

// start makes the TUI over f, sized, with every tab read.
func start(t *testing.T, f *fakeSource, km keymap.Map) *Model {
	m, _ := startWith(t, f, km)
	return m
}

func startWith(t *testing.T, f *fakeSource, km keymap.Map) (*Model, *fakeRunner) {
	t.Helper()
	r := &fakeRunner{results: map[model.PRKey]runner.Event{}}
	m := New(f, r, &fakeClaude{}, km, nil, testSettings())
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	run(m, m.Init())
	return m, r
}

// testSettings are the settings of the tests' TUI.
func testSettings() Settings {
	return Settings{
		Sections: []Section{
			{Name: "review", Query: "review", Grouped: true, Folded: []string{"team", "bots"}, Limit: 100},
			{Name: "mine", Query: "mine", DraftsLast: true, Limit: 100},
			{Name: "involved", Query: "involved", Limit: 100},
		},
		InboxLimit: 100, HideClosed: true, DoneAfterReview: true, HomeOwners: []string{"acme"}, StaleDays: 30,
		PreviewHeight: 10, Collapse: []string{"go.sum"}, Theme: "mocha", Now: func() time.Time { return now },
		Plan: plan.Settings{Methods: []model.MergeMethod{model.MethodMerge}, AutoMerge: true, UpdateBranch: "humans",
			AttentionPaths: []string{".github/workflows/*"}},
		Rules:   []plan.Rule{{Name: "octo bumps", Author: "renovate", Packages: []string{"octo/app"}}},
		CanWait: true,
	}
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
		case nil, tickMsg, toastMsg, spinMsg, freshMsg:
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
	m, r := startWith(t, f, keymap.Default())
	press(m, "A")
	if !strings.Contains(screen(m), "approve this pull request?") {
		t.Fatalf("no confirm popup:\n%s", screen(m))
	}
	press(m, "esc")
	if len(r.items) != 0 {
		t.Fatal("esc started the approve")
	}
	press(m, "A", "y")
	if len(r.items) != 1 || r.kinds[0] != runner.Approve || r.items[0][0].Head != "aaaaaaa1" {
		t.Errorf("batches = %+v", r.items)
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
	if m.bulk != nil || !strings.Contains(m.status, "cannot approve your own") {
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
	if m.bulk != nil {
		t.Error("A still approves after approve was rebound to a")
	}
	press(m, "a")
	if m.bulk == nil {
		t.Error("a does not approve")
	}
}

func TestSafeRemovesEscapes(t *testing.T) {
	in := "ok\x1b]0;owned\x07 \x1b[2Jtitle\r\tend\nnext"
	if got := safe(in); got != "ok title    end\nnext" {
		t.Errorf("safe = %q", got)
	}
}

func TestTitleEscapesNeverReachTheScreen(t *testing.T) {
	f := fixture()
	f.searches["review"][0].Title = "evil \x1b]0;pwned\x07title"
	m := start(t, f, keymap.Default())
	if s := m.render(); strings.Contains(s, "pwned\x07") || strings.Contains(s, "\x1b]0;") {
		t.Error("an escape sequence from a title reached the screen")
	}
}

func TestOneReviewAtATime(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	press(m, "c")
	run(m, m.paste(tea.PasteMsg{Content: "once"}))
	// y without running the command: the review is still on its way.
	press(m, "ctrl+s")
	_, cmd := m.Update(keyMsg("y"))
	press(m, "ctrl+s")
	if m.confirm != nil || !strings.Contains(m.status, "being sent") {
		t.Errorf("a second review was asked for while one was on its way: %q", m.status)
	}
	run(m, cmd)
	if len(f.reviews) != 1 {
		t.Errorf("reviews = %q", f.reviews)
	}
}

func TestReviewOfAnotherPRKeepsTheDraft(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "c")
	m.handleData(reviewMsg{pr: model.PR{Key: key("acme", "web", 7)}, event: model.EventApprove})
	if m.view != viewCompose || m.compose == nil {
		t.Error("a review of another pull request closed the draft")
	}
}

type failingDone struct{ *fakeSource }

func (f failingDone) MarkDone(context.Context, model.Notification) error {
	return fmt.Errorf("network down")
}

func TestFailedDoneComesBack(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	m.src = failingDone{fixture()}
	press(m, "4", "d")
	if m.inboxTab().hidden[key("acme", "api", 12)] {
		t.Error("a notification whose done failed stayed hidden")
	}
	if _, ok := m.unread[key("acme", "api", 12)]; !ok {
		t.Error("its unread mark did not come back")
	}
}

// The cursor's row keeps its background behind every colored cell.
func TestCursorRowBackground(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	probe := m.st.sel.Render("\x00")
	set := probe[:strings.IndexByte(probe, 0)]
	for _, l := range strings.Split(m.render(), "\n") {
		if !strings.Contains(ansi.Strip(l), "▎ ● api#12") {
			continue
		}
		parts := strings.Split(l, "\x1b[m")
		for _, p := range parts[:len(parts)-1] {
			if !strings.HasPrefix(p, set) && !strings.Contains(p, set) {
				t.Fatalf("a part of the cursor's row lost the background: %q", p)
			}
		}
		return
	}
	t.Fatal("cursor row not found")
}

func TestBulkMerge(t *testing.T) {
	f := fixture()
	f.info = map[model.PRKey]func(*model.MergeInfo){
		key("acme", "api", 30): func(mi *model.MergeInfo) { mi.MergeState = model.MergeDirty },
	}
	m, r := startWith(t, f, keymap.Default())
	press(m, "b", "space", "j", "j", "space")
	if len(m.selected) != 2 {
		t.Fatalf("selected = %v", m.selected)
	}
	press(m, "M")
	golden(t, "bulk-merge", screen(m))
	press(m, "y")
	if len(r.items) != 1 || len(r.items[0]) != 1 || r.items[0][0].PR.Key != key("acme", "api", 12) {
		t.Fatalf("batches = %+v", r.items)
	}
	if len(m.selected) != 0 {
		t.Errorf("selection left after merge: %v", m.selected)
	}
	golden(t, "bulk-progress", screen(m))
	if m.unseen() != 1 || !strings.Contains(m.notes[0].title, "merged 1, 1 skipped") {
		t.Errorf("notes = %+v", m.notes)
	}
	if got := m.notes[0].items[0].text; got != "merged (merge commit)" {
		t.Errorf("the note says %q", got)
	}
	press(m, "R")
	if len(m.progress) != 0 || len(m.rows(m.tab())) == 0 {
		t.Errorf("R did not clear the results: %v", m.progress)
	}
	for _, p := range m.tab().prs {
		if p.Key == key("acme", "api", 12) && m.merged[p.Key] {
			t.Error("the merged pull request is still listed")
		}
	}
}

func TestHeadMovedIsNotSent(t *testing.T) {
	f := fixture()
	f.info = map[model.PRKey]func(*model.MergeInfo){
		key("acme", "api", 12): func(mi *model.MergeInfo) { mi.HeadOID = "pushed-later" },
	}
	m, r := startWith(t, f, keymap.Default())
	press(m, "M")
	if !strings.Contains(screen(m), "changed since it was read") {
		t.Errorf("popup does not say the head moved:\n%s", screen(m))
	}
	press(m, "y")
	if len(r.items) != 0 {
		t.Errorf("a moved head was sent: %+v", r.items)
	}
}

func TestNotificationsJump(t *testing.T) {
	m, r := startWith(t, fixture(), keymap.Default())
	r.results[key("acme", "api", 12)] = runner.Event{Result: runner.Stopped, Text: "blocked by another rule"}
	press(m, "M", "y", "2", "n")
	if !strings.Contains(screen(m), "blocked by another rule") {
		t.Fatalf("notification lacks the result:\n%s", screen(m))
	}
	press(m, "j", "enter")
	if m.cur != 0 || m.notesAt != nil {
		t.Errorf("did not jump: tab %d", m.cur)
	}
	if r, ok := m.current(); !ok || r.pr.Key != key("acme", "api", 12) {
		t.Errorf("cursor on %+v", r.pr.Key)
	}
}

func TestRunningIsNotStartedAgain(t *testing.T) {
	m, r := startWith(t, fixture(), keymap.Default())
	m.progress[key("acme", "api", 12)] = progress{text: "waiting for checks", result: runner.Running}
	press(m, "M")
	if !strings.Contains(screen(m), "already running") {
		t.Errorf("popup does not say it is running:\n%s", screen(m))
	}
	press(m, "y")
	if len(r.items) != 0 {
		t.Errorf("started again: %+v", r.items)
	}
}

func TestFoldGroupByKey(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	if !m.tab().folded[plan.GroupBots] {
		t.Fatal("the bots group is not folded at start")
	}
	for r, _ := m.current(); r.kind != rowGroup || r.group != plan.GroupBots; r, _ = m.current() {
		press(m, "j")
	}
	press(m, "l")
	if m.tab().folded[plan.GroupBots] {
		t.Fatal("l did not unfold the group")
	}
	press(m, "j")
	if r, _ := m.current(); r.kind != rowPR {
		t.Fatalf("row = %+v, want a pull request in the group", r)
	}
	press(m, "h")
	if m.tab().folded[plan.GroupBots] {
		t.Fatal("h on a pull request folded its group")
	}
	press(m, "k", "h")
	if !m.tab().folded[plan.GroupBots] {
		t.Fatal("h did not fold the group")
	}
	press(m, "right", "left")
	if !m.tab().folded[plan.GroupBots] {
		t.Fatal("left did not fold the group")
	}
}

func TestSelectByRule(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "S")
	golden(t, "select-rule", screen(m))
	press(m, "j", "enter")
	if len(m.selected) != 1 || !m.selected[key("acme", "api", 30)] {
		t.Fatalf("selected = %v", m.selected)
	}
	if m.tab().folded[plan.GroupBots] {
		t.Error("the bots group stayed folded")
	}
	if r, ok := m.current(); !ok || r.pr.Key != key("acme", "api", 30) {
		t.Errorf("cursor on %v", r.pr.Key)
	}
}

func TestResumeAtStart(t *testing.T) {
	f := fixture()
	set := Settings{Sections: []Section{{Name: "review", Query: "review", Grouped: true, Limit: 100}}, InboxLimit: 100,
		Theme: "mocha", Now: func() time.Time { return now }, CanWait: true,
		Resume: []model.Pending{{Key: key("acme", "api", 12), ID: "1", Head: "aaaaaaa1", Author: "alice"}}}
	r := &fakeRunner{results: map[model.PRKey]runner.Event{}}
	m := New(f, r, &fakeClaude{}, keymap.Default(), nil, set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	run(m, m.Init())
	if len(r.resumed) != 1 || m.progress[key("acme", "api", 12)].result != runner.Merged {
		t.Errorf("resumed %v, progress %+v", r.resumed, m.progress)
	}
	if m.unseen() != 1 {
		t.Errorf("no note for the resumed batch: %+v", m.notes)
	}
}

func TestNoWaitingWithoutTheLock(t *testing.T) {
	f := fixture()
	f.info = map[model.PRKey]func(*model.MergeInfo){
		key("acme", "api", 12): func(mi *model.MergeInfo) { mi.MergeState = model.MergeBlocked },
	}
	m, r := startWith(t, f, keymap.Default())
	m.set.CanWait = false
	press(m, "M")
	if !strings.Contains(screen(m), "another gh-triage") {
		t.Errorf("popup does not say why:\n%s", screen(m))
	}
	press(m, "y")
	if len(r.items) != 0 {
		t.Errorf("a waiting merge started without the lock: %+v", r.items)
	}
}

// fakeClaude answers every question at once and keeps what it was asked.
type fakeClaude struct {
	asked    []claude.Request
	prepared []model.PRKey
	// notes are the prompts Once was given.
	notes []string
	// fail makes the next question fail.
	fail bool
}

func (c *fakeClaude) Prepare(_ context.Context, k model.PRKey, head string) (string, error) {
	c.prepared = append(c.prepared, k)
	return "/cache/worktrees/" + k.String(), nil
}

func (c *fakeClaude) Ask(_ context.Context, r claude.Request, progress func(claude.Step)) (claude.Answer, error) {
	c.asked = append(c.asked, r)
	if c.fail {
		c.fail = false
		return claude.Answer{}, fmt.Errorf("claude: rate limited")
	}
	progress(claude.Step{Tool: "Read", Arg: "main.go"})
	if r.Schema != "" {
		return claude.Answer{Structured: []byte(`{"event":"APPROVE","body":"Looks good: the retry is bounded."}`), Cost: 0.1, Model: "claude-opus-5-5"}, nil
	}
	return claude.Answer{Text: fmt.Sprintf("Answer %d: the retry loop has no limit.", len(c.asked)), Cost: 0.4, Model: "claude-opus-5-5"}, nil
}

func (c *fakeClaude) Interactive(dir, session string) *exec.Cmd { return exec.Command("true") }

func (c *fakeClaude) Once(_ context.Context, _, prompt, _ string) (claude.Answer, error) {
	c.notes = append(c.notes, prompt)
	return claude.Answer{Structured: []byte(`{"cautions":[{"key":"acme/api#30","why":"drops support for config v1"}]}`)}, nil
}

func TestClaudeConversation(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	fc := m.claude.(*fakeClaude)
	press(m, "C")
	if m.view != viewChat || len(fc.asked) != 1 {
		t.Fatalf("view %v, asked %d", m.view, len(fc.asked))
	}
	first := fc.asked[0]
	if !first.First || !strings.Contains(first.Prompt, "<diff>") || first.Dir == "" {
		t.Errorf("first request %+v", first)
	}
	golden(t, "claude", screen(m))

	press(m, "i", "w", "h", "y", "?", "enter")
	if len(fc.asked) != 2 {
		t.Fatalf("asked %d", len(fc.asked))
	}
	if r := fc.asked[1]; r.First || r.Session != first.Session || r.Prompt != "why?" || r.Dir != first.Dir {
		t.Errorf("second request %+v", r)
	}
	if len(fc.prepared) != 1 {
		t.Errorf("the worktree was made %d times", len(fc.prepared))
	}

	// Leaving and coming back keeps the conversation.
	press(m, "esc")
	if m.view != viewList {
		t.Fatalf("view %v", m.view)
	}
	press(m, "C")
	if len(fc.asked) != 2 || !strings.Contains(screen(m), "Answer 2") {
		t.Errorf("the conversation was not kept:\n%s", screen(m))
	}
}

func TestClaudeDraft(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	press(m, "C", "D")
	if m.view != viewCompose || m.compose == nil {
		t.Fatalf("view %v", m.view)
	}
	if kinds[m.compose.kind] != model.EventApprove || m.compose.ta.Value() != "Looks good: the retry is bounded." {
		t.Errorf("draft kind %v body %q", kinds[m.compose.kind], m.compose.ta.Value())
	}
	press(m, "esc")
	if m.view != viewChat {
		t.Errorf("discarding the draft went to %v", m.view)
	}
}

func TestReleaseNotesMarkBotPullRequests(t *testing.T) {
	f := fixture()
	f.info = map[model.PRKey]func(*model.MergeInfo){
		key("acme", "api", 30): func(mi *model.MergeInfo) { mi.Rollup = model.CheckSuccess },
	}
	m, r := startWith(t, f, keymap.Default())
	m.set.ReleaseNotes = true
	fc := m.claude.(*fakeClaude)
	press(m, "b", "space", "j", "j", "space", "M")
	if len(fc.notes) != 1 || !strings.Contains(fc.notes[0], "acme/api#30") || strings.Contains(fc.notes[0], "acme/api#12") {
		t.Fatalf("asked about %q", fc.notes)
	}
	if s := screen(m); !strings.Contains(s, "⚑ drops support for config v1") || !strings.Contains(s, "1 marked ⚑") {
		t.Errorf("no mark:\n%s", s)
	}
	// Marked, not left out: what is merged is still the user's choice.
	press(m, "esc", "M")
	if len(fc.notes) != 1 {
		t.Errorf("asked again for the same heads: %d", len(fc.notes))
	}
	press(m, "y")
	if len(r.items) != 1 || len(r.items[0]) != 2 {
		t.Errorf("batches = %+v", r.items)
	}
}

// A failed first question does not leave the session ID taken: the next
// one starts a new session.
func TestClaudeFirstFailureStartsAgain(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	fc := m.claude.(*fakeClaude)
	fc.fail = true
	press(m, "C")
	if !strings.Contains(screen(m), "rate limited") {
		t.Fatalf("no error shown:\n%s", screen(m))
	}
	press(m, "i", "o", "k", "enter")
	if len(fc.asked) != 2 {
		t.Fatalf("asked %d", len(fc.asked))
	}
	if r := fc.asked[1]; !r.First || r.Session == fc.asked[0].Session {
		t.Errorf("retry %+v after %+v", r, fc.asked[0])
	}
}

func TestDescriptionWraps(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	prose := strings.Repeat("長い段落は端で折り返す。", 5)
	table := "| " + strings.Repeat("column ", 10) + "|"
	code := strings.Repeat("x", 40)
	body := prose + "\n" + table + "\n```\n" + code + "\n```"
	var got []string
	for _, l := range m.bodyLines(body, 30) {
		got = append(got, ansi.Strip(l.text))
	}
	var wrapped string
	n := 0
	for ; n < len(got) && got[n] != table; n++ {
		if w := ansi.StringWidth(got[n]); w > 30 {
			t.Errorf("line %q is %d cells wide", got[n], w)
		}
		wrapped += got[n]
	}
	if n < 2 || wrapped != prose {
		t.Errorf("prose became %q", got[:n])
	}
	if want := []string{table, "```", code, "```"}; !slices.Equal(got[n:], want) {
		t.Errorf("table and code became %q, want them uncut", got[n:])
	}
}
