package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/claude"
	"github.com/babarot/gh-desk/internal/diff"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/chat"
	"github.com/babarot/gh-desk/internal/tui/keymap"
	"github.com/babarot/gh-desk/internal/tui/statusline"
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
			HeadRef: "retries", HeadRepo: "acme/api", HeadOID: "aaaaaaa1", BaseRef: "main", Additions: 40, Deletions: 3, ChangedFiles: 2,
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
	posted  []string
	done    []string
	browsed []string
	// info overrides what MergeInfo says of a pull request, by key.
	info map[model.PRKey]func(*model.MergeInfo)
	// infoReads are how many pull requests each read of MergeInfo asked
	// for.
	infoReads []int
	// based are the pull requests based on each branch.
	based map[model.Branch][]int
	// noHeavy has Heavy return nothing, and searchErr fails every search.
	noHeavy   bool
	searchErr error
}

func (f *fakeSource) MergeInfo(_ context.Context, ids []string, _ bool) (map[string]model.MergeInfo, error) {
	out := map[string]model.MergeInfo{}
	f.infoReads = append(f.infoReads, len(ids))
	extra := slices.Collect(maps.Values(f.extra))
	for _, prs := range append(slices.Collect(maps.Values(f.searches)), extra) {
		for _, p := range prs {
			for _, id := range ids {
				if p.ID != id {
					continue
				}
				mi := model.MergeInfo{Key: p.Key, ID: p.ID, State: model.StateOpen, HeadOID: p.HeadOID, MergeState: model.MergeClean,
					Rollup: p.Checks, Checks: []model.Check{}, Files: []string{}, Reviews: p.Reviews,
					Repo:     model.RepoMerge{Methods: []model.MergeMethod{model.MethodMerge}},
					CanClose: true, CanWrite: true, HeadRefID: "ref-" + p.ID,
					Head: model.Branch{Owner: p.Key.Owner, Repo: p.Key.Repo, Name: p.HeadRef}}
				if p.State != "" {
					mi.State = p.State
				}
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

func (f *fakeSource) BasedOn(_ context.Context, bs []model.Branch) (map[model.Branch][]int, error) {
	out := map[model.Branch][]int{}
	for _, b := range bs {
		if ns := f.based[b]; len(ns) > 0 {
			out[b] = ns
		}
	}
	return out, nil
}

// fakeRunner records the batches it is given and ends each pull request
// with the result results says, Approved or Merged by default.
type fakeRunner struct {
	kinds   []runner.Kind
	reviews []runner.ReviewText
	items   [][]runner.Item
	results map[model.PRKey]runner.Event
	resumed []model.Pending
	closes  []runner.CloseText
}

// StartClose closes each pull request, saying first that the comment was
// taken when there is one, as the runner does.
func (r *fakeRunner) StartClose(_ context.Context, ct runner.CloseText, items []runner.Item) (string, <-chan runner.Event) {
	r.closes = append(r.closes, ct)
	r.kinds = append(r.kinds, runner.Close)
	r.items = append(r.items, items)
	ch := make(chan runner.Event, 2*len(items))
	for _, it := range items {
		if ct.Body != "" {
			ch <- runner.Event{Key: it.PR.Key, Result: runner.Running, Text: "commented", Commented: true}
		}
		ev, ok := r.results[it.PR.Key]
		if !ok {
			ev = runner.Event{Result: runner.Closed, Text: "closed", Closed: true}
		}
		ev.Key = it.PR.Key
		ch <- ev
	}
	close(ch)
	return fmt.Sprintf("b%d", len(r.kinds)), ch
}

func (r *fakeRunner) Resume(ctx context.Context, ps []model.Pending) (string, <-chan runner.Event) {
	var items []runner.Item
	for _, p := range ps {
		items = append(items, runner.Item{PR: model.PR{ID: p.ID, Key: p.Key}, Head: p.Head})
	}
	r.resumed = append(r.resumed, ps...)
	return r.Start(ctx, runner.Merge, items)
}

func (r *fakeRunner) StartReview(_ context.Context, rv runner.ReviewText, items []runner.Item) (string, <-chan runner.Event) {
	r.reviews = append(r.reviews, rv)
	return r.start(runner.Review, rv, items)
}

func (r *fakeRunner) Start(_ context.Context, kind runner.Kind, items []runner.Item) (string, <-chan runner.Event) {
	return r.start(kind, runner.ReviewText{}, items)
}

func (r *fakeRunner) start(kind runner.Kind, rv runner.ReviewText, items []runner.Item) (string, <-chan runner.Event) {
	r.kinds = append(r.kinds, kind)
	r.items = append(r.items, items)
	ch := make(chan runner.Event, len(items))
	for _, it := range items {
		ev, ok := r.results[it.PR.Key]
		if !ok {
			ev = runner.Event{Result: runner.Merged, Text: "merged", Method: model.MethodMerge}
			switch {
			case kind == runner.Review && rv.Event == model.EventApprove:
				ev = runner.Event{Result: runner.Approved, Text: "approved", Approved: true}
			case kind == runner.Review:
				ev = runner.Event{Result: runner.Reviewed, Text: "sent"}
			case kind == runner.Ready:
				ev = runner.Event{Result: runner.Readied, Text: "ready for review"}
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
		if _, err := fmt.Sscan(after, &from); err != nil {
			return nil, 0, "", nil, fmt.Errorf("fake cursor %q: %w", after, err)
		}
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
func (f *fakeSource) PostComment(_ context.Context, p model.PR, body string) error {
	f.posted = append(f.posted, p.Key.String()+" "+body)
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
		Tabs: []Tab{
			{Name: "review", Query: "review", Groups: plan.Groups, Folded: []plan.Group{plan.GroupTeam, plan.GroupBots}, Limit: 100},
			{Name: "mine", Query: "mine", DraftsLast: true, Limit: 100},
			{Name: "involved", Query: "involved", Limit: 100},
			{Name: "inbox", Inbox: true, Limit: 100},
		},
		MarkDone: true, HomeOwners: []string{"acme"}, StaleDays: 30,
		PreviewHeight: 10, Collapse: []string{"go.sum"}, ThemeDark: "default", ThemeLight: "default", Now: func() time.Time { return now },
		Plan: plan.Settings{Methods: []model.MergeMethod{model.MethodMerge}, AutoMerge: true, UpdateBranch: "humans",
			AttentionPaths: []string{".github/workflows/*"}},
		Rules:   []plan.Rule{{Name: "octo bumps", Authors: []string{"renovate"}, Packages: []string{"octo/app"}}},
		CanWait: true, Language: "English",
		// Clawd's scenes are the first theme's, with the first of each part.
		Pick: func(int) int { return 0 },
		// Ticks only redraw; tests read the screen when they like.
		Tick: func(time.Duration, tea.Msg) tea.Cmd { return nil },
	}
}

// status is the status line as shown, without colors.
func status(m *Model) string { return ansi.Strip(m.statusText()) }

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
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
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

func screen(m *Model) string { s, _ := m.render(); return ansi.Strip(s) }

// golden compares what m draws with testdata/name.golden: the characters on
// the screen, then the colors and attributes of each run of them, so a change
// of color fails as a change of text does.
func golden(t *testing.T, name string, m *Model) {
	t.Helper()
	view, _ := m.render()
	got := normalize(t, view)
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
	if !sameView(got, string(want)) {
		t.Errorf("%s differs from %s (run go test ./internal/tui -update to accept):\n%s", name, path, got)
	}
}

func TestReviewTab(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	golden(t, "review", m)
	press(m, "b", "G")
	golden(t, "review-bots", m)
}

// groupHeads are the groups whose heads t's rows show, in order.
func groupHeads(m *Model) []plan.Group {
	var gs []plan.Group
	for _, r := range m.rows(m.tab()) {
		if r.kind == rowGroup {
			gs = append(gs, r.group)
		}
	}
	return gs
}

func TestGroupsOnlyWithRows(t *testing.T) {
	// The review tab has no pull request without a request: other is not
	// shown, in the list or the header's counts.
	m := start(t, fixture(), keymap.Default())
	if got, want := groupHeads(m), []plan.Group{plan.GroupYou, plan.GroupTeam, plan.GroupBots}; !slices.Equal(got, want) {
		t.Fatalf("groups = %v, want %v", got, want)
	}
	if s := screen(m); strings.Contains(s, "other") {
		t.Errorf("other is shown with no rows:\n%s", s)
	}

	// A filter that leaves only the bot's pull request leaves only its group.
	press(m, "/", "o", "c", "t", "o", "enter")
	if got := groupHeads(m); !slices.Equal(got, []plan.Group{plan.GroupBots}) {
		t.Fatalf("filtered groups = %v, want bots only", got)
	}
	// A filter that matches nothing leaves no rows, not empty heads.
	press(m, "/", "z", "z", "z", "enter")
	if rs := m.rows(m.tab()); len(rs) != 0 {
		t.Fatalf("rows = %d, want none", len(rs))
	}
	if s := screen(m); !strings.Contains(s, "nothing here") || strings.Contains(s, "requested from") {
		t.Errorf("screen with nothing filtered in:\n%s", s)
	}
	if _, _, ok := m.currentPR(); ok {
		t.Error("the cursor is on a pull request with no rows")
	}
	press(m, "esc")
	if got := groupHeads(m); len(got) != 3 {
		t.Fatalf("groups after the filter = %v, want three", got)
	}
}

func TestFilterKeptOnReentry(t *testing.T) {
	// An accepted filter is there to edit when / is pressed again.
	m := start(t, fixture(), keymap.Default())
	press(m, "/", "o", "c", "enter", "/", "t", "o")
	if m.filter != "octo" || !m.filtering {
		t.Fatalf("filter = %q, filtering = %v; want %q and filtering", m.filter, m.filtering, "octo")
	}
	press(m, "esc")
	if m.filter != "" || m.filtering {
		t.Fatalf("after esc filter = %q, filtering = %v; want cleared", m.filter, m.filtering)
	}
}

func TestFilterEraseWord(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "/", "a", " ", "b", "c", " ", " ", "ctrl+w")
	if m.filter != "a " {
		t.Fatalf("filter = %q, want %q", m.filter, "a ")
	}
	press(m, "ctrl+w")
	if m.filter != "" {
		t.Fatalf("filter = %q, want empty", m.filter)
	}
	press(m, "ctrl+w")
	if m.filter != "" || !m.filtering {
		t.Fatalf("filter = %q, filtering = %v; want empty and still filtering", m.filter, m.filtering)
	}
}

func TestEraseWord(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"abc", ""},
		{"a bc  ", "a "},
		{"repo:babarot/gh-desk", "repo:babarot/"},
		{"repo:babarot/", "repo:"},
		{"repo:", ""},
		{"is:open author:me", "is:open author:"},
		{"a / ", ""},
		{"レビュー\u3000待ち", "レビュー\u3000"},
	} {
		if got := eraseWord(tc.in); got != tc.want {
			t.Errorf("eraseWord(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestGroupedTabWithoutRequests(t *testing.T) {
	// A grouped tab of one's own pull requests has no requests to the
	// viewer or a team: its rows are all under other.
	set := testSettings()
	set.Tabs[1].Groups = plan.Groups
	m := New(fixture(), &fakeRunner{results: map[model.PRKey]runner.Event{}}, &fakeClaude{}, keymap.Default(), nil, set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	run(m, m.Init())
	press(m, "2")
	if got := groupHeads(m); !slices.Equal(got, []plan.Group{plan.GroupOther}) {
		t.Fatalf("groups = %v, want other only\n%s", got, screen(m))
	}
	s := screen(m)
	for _, want := range []string{"▾ other 1", "1 other"} {
		if !strings.Contains(s, want) {
			t.Errorf("screen does not show %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "to a team") || strings.Contains(s, "to you") {
		t.Errorf("screen counts groups with no rows:\n%s", s)
	}
}

func TestPreviewDiff(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "v")
	golden(t, "preview-diff", m)
}

func TestPreviewDiffFoldsFiles(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "v")
	// go.sum matches the collapse patterns, so it starts folded; z folds
	// the file whose header is in sight first, and leaves go.sum alone.
	if s := screen(m); !strings.Contains(s, "▾ client.go") || !strings.Contains(s, "▸ go.sum") {
		t.Fatalf("want client.go open and go.sum folded:\n%s", s)
	}
	press(m, "z")
	if s := screen(m); !strings.Contains(s, "▸ client.go") || strings.Contains(s, "var tries = 3") || !strings.Contains(s, "▸ go.sum") {
		t.Fatalf("z did not fold client.go alone:\n%s", s)
	}
	// A click on a file's header folds or unfolds it.
	x, y := lineOf(t, m, "▸ go.sum")
	mouseAt(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if s := screen(m); !strings.Contains(s, "▾ go.sum") || !strings.Contains(s, "+x v2") {
		t.Fatalf("a click did not unfold go.sum:\n%s", s)
	}
}

func TestPreviewDiffFileTabs(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "v")
	if s := screen(m); !strings.Contains(s, " all (2)   client.go   go.sum ") {
		t.Fatalf("no file tabs:\n%s", s)
	}
	// ] shows the next file alone, unfolded even when it matches the
	// collapse patterns, and z does not fold it.
	press(m, "]", "]")
	if s := screen(m); !strings.Contains(s, "▾ go.sum") || !strings.Contains(s, "+x v2") || strings.Contains(s, "client.go  +2") {
		t.Fatalf("] did not show go.sum alone:\n%s", s)
	}
	press(m, "z")
	if s := screen(m); !strings.Contains(s, "+x v2") {
		t.Fatalf("z folded a file shown in its own tab:\n%s", s)
	}
	press(m, "[", "[", "[")
	if s := screen(m); !strings.Contains(s, "▾ client.go") || !strings.Contains(s, "▸ go.sum") {
		t.Fatalf("[ did not go back to all:\n%s", s)
	}
	// A click on a tab shows it.
	x, y := lineOf(t, m, " client.go   go.sum ")
	mouseAt(m, tea.MouseClickMsg{X: x + 1, Y: y, Button: tea.MouseLeft})
	if s := screen(m); !strings.Contains(s, "▾ client.go") || strings.Contains(s, "go.sum  +1") {
		t.Fatalf("a click did not show client.go alone:\n%s", s)
	}
}

// On a pull request, l and h step the pane along overview, all of the diff
// and each file, and stop at either end.
func TestPreviewStepsTabsSideways(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	tab := func() string {
		t.Helper()
		p, _, ok := m.currentPR()
		if !ok {
			t.Fatal("no pull request under the cursor")
		}
		if m.pane.mode == paneOverview {
			return "overview"
		}
		if f := m.pane.file[p.Key]; f != "" {
			return f
		}
		return "all"
	}
	steps := []struct{ key, want string }{
		{"h", "overview"},
		{"l", "all"},
		{"l", "client.go"},
		{"right", "go.sum"},
		{"l", "go.sum"},
		{"h", "client.go"},
		{"left", "all"},
		{"h", "overview"},
	}
	for i, s := range steps {
		press(m, s.key)
		if got := tab(); got != s.want {
			t.Fatalf("step %d (%s): tab = %s; want %s", i, s.key, got, s.want)
		}
	}
	// From the overview, l opens the diff on all, whichever file was left.
	press(m, "v", "]")
	press(m, "v", "l")
	if got := tab(); got != "all" {
		t.Fatalf("l from the overview: tab = %s; want all", got)
	}
	// With the pane hidden, h and l do nothing.
	press(m, "p", "h")
	if got := tab(); got != "all" {
		t.Fatalf("h with the pane hidden: tab = %s; want all", got)
	}
}

func TestFileTabNames(t *testing.T) {
	var files []diff.File
	for _, p := range []string{"api/Dockerfile", "web/Dockerfile", "compose.yaml", "a/x/main.go", "b/x/main.go"} {
		files = append(files, diff.File{NewPath: p})
	}
	got := fileTabNames(files)
	want := []string{"api/Dockerfile", "web/Dockerfile", "compose.yaml", "a/x/main.go", "b/x/main.go"}
	if !slices.Equal(got, want) {
		t.Errorf("fileTabNames = %q; want %q", got, want)
	}
}

func TestPreviewOpensOnDiff(t *testing.T) {
	set := testSettings()
	set.OpenOnDiff = true
	m := New(fixture(), &fakeRunner{results: map[model.PRKey]runner.Event{}}, &fakeClaude{}, keymap.Default(), nil, set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	run(m, m.Init())
	if m.pane.mode != paneDiff {
		t.Fatalf("mode = %v; want the diff", m.pane.mode)
	}
}

// Enter on a pull request opens the menu of what can be done to it; an
// action runs by its key there as on the list, and one that cannot run
// says why and leaves the menu open.
func TestActionMenu(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "enter")
	if m.menu == nil {
		t.Fatal("enter did not open the menu")
	}
	golden(t, "action-menu", m)
	press(m, "esc")
	if m.menu != nil {
		t.Fatal("esc did not close the menu")
	}

	// merge is the sixth.
	press(m, "enter", "j", "j", "j", "j", "j", "enter")
	if m.menu != nil || m.bulk == nil || m.bulk.kind != runner.Merge {
		t.Fatalf("choosing merge did not open the merge popup: menu %v, bulk %v", m.menu, m.bulk)
	}
	press(m, "esc")

	press(m, "enter", "c")
	if m.compose == nil || m.menu != nil {
		t.Fatal("c in the menu did not start a comment")
	}
}

// Under a popup the screen is drawn in the theme's faint color alone, so
// only the popup keeps its colors.
func TestPopupDimsTheScreen(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	header := func() string { s, _ := m.render(); return strings.Split(s, "\n")[0] }
	before := header()
	if before == m.st.dim.Render(ansi.Strip(before)) {
		t.Fatalf("the header is faint before any popup: %q", before)
	}
	press(m, "enter")
	if got, want := header(), m.st.dim.Render(ansi.Strip(before)); got != want {
		t.Errorf("the header under the menu is not dimmed:\n got %q\nwant %q", got, want)
	}
}

// With a selection the menu acts on all of it: approve and merge count
// what they would act on, and actions on one pull request cannot run.
func TestActionMenuSelection(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	// space on #12, then on the team group under it, which selects #7.
	press(m, "space", "space", "k", "enter")
	golden(t, "action-menu-selection", m)
	press(m, "i")
	if m.menu == nil || len(m.chats) != 0 || func() bool { k, _ := m.shownStatus(); return k != statusline.Refusal }() {
		t.Fatalf("asking claude on a selection ran: menu %v", m.menu)
	}
	press(m, "R")
	if m.menu != nil || m.compose == nil || len(m.compose.prs) != 2 {
		t.Fatalf("review did not open for the selection: %+v", m.compose)
	}
	press(m, "esc", "enter", "A")
	if m.menu != nil || m.bulk == nil || m.bulk.kind != runner.Review {
		t.Fatalf("approve did not open the approve popup for the selection: %v", m.bulk)
	}
}

func TestInbox(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	press(m, "4")
	golden(t, "inbox", m)
	// The tab counts the unread notifications its rows hold: the one on a
	// merged pull request only while it is shown.
	if s := screen(m); !strings.Contains(s, "inbox (1)") {
		t.Errorf("the hidden merged notification is counted:\n%s", s)
	}
	press(m, "a", "j")
	if s := screen(m); !strings.Contains(s, "Does this cover the timeout?") || !strings.Contains(s, "merged") {
		t.Errorf("closed notification or its comment not shown:\n%s", s)
	}
	if s := screen(m); !strings.Contains(s, "inbox (2)") {
		t.Errorf("the shown merged notification is not counted:\n%s", s)
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
	if len(r.items) != 1 || r.kinds[0] != runner.Review || r.items[0][0].Head != "aaaaaaa1" {
		t.Errorf("batches = %+v", r.items)
	}
	if len(f.done) != 1 || f.done[0] != "acme/api#12" {
		t.Errorf("notification not marked done: %v", f.done)
	}
	if !strings.Contains(screen(m), "me✓") {
		t.Errorf("row does not show the approval:\n%s", screen(m))
	}
}

// c comments on the conversation: no review is sent, so the review asked
// of the viewer stays, and the notification is not marked done.
func TestCommentWithBody(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	press(m, "c", "tab", "ctrl+s")
	if !strings.Contains(status(m), "write the comment first") {
		t.Errorf("empty comment was not refused: %q", status(m))
	}
	run(m, m.paste(tea.PasteMsg{Content: "Does this retry on a 429?"}))
	press(m, "ctrl+s")
	if !strings.Contains(screen(m), "a review asked of you stays asked") {
		t.Errorf("the confirm popup does not tell a comment from a review:\n%s", screen(m))
	}
	press(m, "y")
	if len(f.posted) != 1 || f.posted[0] != "acme/api#12 Does this retry on a 429?" || len(f.reviews) != 0 {
		t.Errorf("posted = %q, reviews = %q", f.posted, f.reviews)
	}
	if m.compose != nil {
		t.Error("the popup stayed open")
	}
	if len(f.done) != 0 {
		t.Errorf("a comment marked the notification done: %v", f.done)
	}
	if p, _ := m.knownPR(key("acme", "api", 12)); len(p.Reviews) != 0 {
		t.Errorf("a comment shows as a review: %+v", p.Reviews)
	}
	// Shown in the pane without reading it again (ADR-067).
	cs := m.cache.details[key("acme", "api", 12)].val.Comments
	if len(cs) != 1 || cs[0].Author != "me" || cs[0].Body != "Does this retry on a 429?" {
		t.Errorf("the pane's comments = %+v", cs)
	}
}

// c with a selection sends the same comment to each selected pull
// request. The popup closes at once and the status line counts the
// comment through them; none stays selected, and the retry key sends the
// comment again to those it failed on.
func TestCommentOnSelection(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	m.src = &failingComment{fakeSource: f, on: key("acme", "web", 7)}
	// space on #12, then on the team group under it, which selects #7.
	press(m, "space", "space", "k", "c")
	run(m, m.paste(tea.PasteMsg{Content: "@dependabot rebase"}))
	press(m, "ctrl+s")
	if s := screen(m); !strings.Contains(s, "comment on 2 pull requests") || !strings.Contains(s, "acme/api#12  acme/web#7") {
		t.Fatalf("the confirm popup does not name both:\n%s", s)
	}
	// y without running the command: the comment is on its way.
	_, cmd := m.Update(keyMsg("y"))
	if m.compose != nil || !strings.Contains(screen(m), "sending the comment… 1/2") {
		t.Fatalf("the popup did not close for the comment to go behind it:\n%s", screen(m))
	}
	press(m, "c")
	run(m, m.paste(tea.PasteMsg{Content: "again"}))
	press(m, "ctrl+s")
	if m.confirm != nil || !strings.Contains(status(m), "being sent") {
		t.Errorf("a second comment was let through while one was on its way: %q", status(m))
	}
	press(m, "esc")
	// #7 read, so a comment shown on it would be seen.
	m.cache.details[key("acme", "web", 7)] = &entry[model.Detail]{}
	run(m, cmd)
	if len(f.posted) != 1 || f.posted[0] != "acme/api#12 @dependabot rebase" {
		t.Errorf("posted = %q", f.posted)
	}
	if cs := m.cache.details[key("acme", "web", 7)].val.Comments; len(cs) != 0 {
		t.Errorf("a comment GitHub refused is shown: %+v", cs)
	}
	if cs := m.cache.details[key("acme", "api", 12)].val.Comments; len(cs) != 1 {
		t.Errorf("the comment sent is not shown: %+v", cs)
	}
	if !strings.Contains(status(m), "failed on 1 of 2") || len(m.selected) != 0 {
		t.Fatalf("selection = %v, status = %q", m.selected, status(m))
	}
	press(m, ".")
	if m.confirm == nil || len(m.confirm.prs) != 1 || m.confirm.body != "@dependabot rebase" {
		t.Fatalf("the retry key does not offer the comment again: %+v", m.confirm)
	}
	press(m, "esc")
	if m.retry == nil {
		t.Fatal("esc lost what is left to try again")
	}
	press(m, ".")
	m.src = f
	press(m, "y")
	if len(f.posted) != 2 || f.posted[1] != "acme/web#7 @dependabot rebase" || m.retry != nil {
		t.Errorf("posted = %q, retry = %+v", f.posted, m.retry)
	}
}

type failingComment struct {
	*fakeSource
	on model.PRKey
}

func (f *failingComment) PostComment(ctx context.Context, p model.PR, body string) error {
	if p.Key == f.on {
		return fmt.Errorf("locked")
	}
	return f.fakeSource.PostComment(ctx, p, body)
}

// R writes a review, and the confirm popup of a batch asks about it: the
// review is sent by the runner on the head the popup read. esc there
// comes back to the review as it was written.
func TestReviewWithBody(t *testing.T) {
	f := fixture()
	m, r := startWith(t, f, keymap.Default())
	press(m, "R", "ctrl+s")
	if !strings.Contains(status(m), "write the review comment first") {
		t.Errorf("empty review was not refused: %q", status(m))
	}
	run(m, m.paste(tea.PasteMsg{Content: "Looks good, one question."}))
	press(m, "ctrl+s")
	if m.bulk == nil || m.bulk.kind != runner.Review || m.bulk.review.Body != "Looks good, one question." {
		t.Fatalf("the confirm popup did not open: %+v", m.bulk)
	}
	golden(t, "review-confirm", m)
	press(m, "esc")
	if m.bulk != nil || m.compose == nil || m.compose.ta.Value() != "Looks good, one question." {
		t.Fatalf("esc lost the review: %+v", m.compose)
	}
	press(m, "ctrl+s", "y")
	if len(r.reviews) != 1 || r.reviews[0] != (runner.ReviewText{Event: model.EventComment, Body: "Looks good, one question."}) ||
		len(r.items[0]) != 1 || r.items[0][0].Head != "aaaaaaa1" || len(f.reviews) != 0 || len(f.posted) != 0 {
		t.Errorf("reviews = %+v, items = %+v, sent directly = %q", r.reviews, r.items, f.reviews)
	}
	if m.compose != nil {
		t.Error("the popup stayed open")
	}
	if p, _ := m.knownPR(key("acme", "api", 12)); len(p.Reviews) == 0 || p.Reviews[len(p.Reviews)-1].State != model.ReviewCommented {
		t.Errorf("the row does not show the review: %+v", p.Reviews)
	}
}

// R on a selection sends the same review to each; none stays selected,
// and the retry key sends it to those the runner did not send it to.
func TestReviewOnSelection(t *testing.T) {
	m, r := startWith(t, fixture(), keymap.Default())
	r.results[key("acme", "web", 7)] = runner.Event{Result: runner.Stopped, Text: "head changed · confirm again"}
	press(m, "space", "space", "k", "R", "tab", "tab")
	run(m, m.paste(tea.PasteMsg{Content: "Please split this."}))
	press(m, "ctrl+s", "y")
	if len(r.reviews) != 1 || r.reviews[0].Event != model.EventRequestChanges || len(r.items[0]) != 2 {
		t.Fatalf("reviews = %+v, items = %+v", r.reviews, r.items)
	}
	if len(m.selected) != 0 {
		t.Fatalf("selection = %v, want none", m.selected)
	}
	press(m, ".")
	if m.bulk == nil || len(m.bulk.items) != 1 || m.bulk.items[0].pr.Key != key("acme", "web", 7) ||
		m.bulk.review.Event != model.EventRequestChanges || m.bulk.review.Body != "Please split this." {
		t.Fatalf("the retry key does not offer the review not sent: %+v", m.bulk)
	}
}

func TestOwnPullRequest(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	press(m, "2", "A")
	if m.bulk != nil || !strings.Contains(status(m), "cannot approve your own") {
		t.Errorf("approve of own PR not refused: %q", status(m))
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
	if s, _ := m.render(); strings.Contains(s, "pwned\x07") || strings.Contains(s, "\x1b]0;") {
		t.Error("an escape sequence from a title reached the screen")
	}
}

// R writes a review in a popup over the list, which stays in sight
// dimmed; esc puts it away and leaves the list as it was. c's popup has no
// kinds.
func TestComposePopup(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "c")
	run(m, m.paste(tea.PasteMsg{Content: "Does this retry on a 429?"}))
	golden(t, "comment", m)
	press(m, "esc", "R")
	run(m, m.paste(tea.PasteMsg{Content: "Does this retry on a 429?"}))
	golden(t, "compose", m)
	press(m, "esc")
	if m.compose != nil {
		t.Errorf("esc left compose %v", m.compose)
	}
}

// A review confirmed and not yet sent leaves its pull request out of the
// next one's popup, so it is not sent twice.
func TestOneReviewAtATime(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "R")
	run(m, m.paste(tea.PasteMsg{Content: "once"}))
	press(m, "ctrl+s")
	// y without running the command: the review is still on its way.
	m.Update(keyMsg("y"))
	press(m, "R")
	run(m, m.paste(tea.PasteMsg{Content: "twice"}))
	press(m, "ctrl+s")
	if m.bulk == nil || len(m.bulk.items) != 1 || !m.bulk.items[0].running || m.bulk.items[0].goes() {
		t.Fatalf("a second review was let through while one was on its way: %+v", m.bulk)
	}
	// Nothing to do: the review written stays.
	press(m, "y")
	if m.compose == nil || m.compose.ta.Value() != "twice" {
		t.Errorf("the review was lost with nothing to send: %+v", m.compose)
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
	s, _ := m.render()
	for l := range strings.SplitSeq(s, "\n") {
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
	golden(t, "bulk-merge", m)
	press(m, "y")
	if len(r.items) != 1 || len(r.items[0]) != 1 || r.items[0][0].PR.Key != key("acme", "api", 12) {
		t.Fatalf("batches = %+v", r.items)
	}
	if len(m.selected) != 0 {
		t.Errorf("selection left after merge: %v", m.selected)
	}
	golden(t, "bulk-progress", m)
	if m.unseen() != 1 || !strings.Contains(m.notes[0].title, "merged 1, 1 skipped") {
		t.Errorf("notes = %+v", m.notes)
	}
	if got := m.notes[0].items[0].text; got != "merged (merge commit)" {
		t.Errorf("the note says %q", got)
	}
	press(m, "r")
	if len(m.progress) != 0 || len(m.rows(m.tab())) == 0 {
		t.Errorf("r did not clear the results: %v", m.progress)
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

func TestMarkReady(t *testing.T) {
	f := fixture()
	draft := key("acme", "web", 7)
	f.info = map[model.PRKey]func(*model.MergeInfo){draft: func(mi *model.MergeInfo) { mi.Draft = true }}
	m, r := startWith(t, f, keymap.Default())
	m.tab().folded[plan.GroupTeam] = false
	for range 10 {
		if row, ok := m.current(); ok && row.pr.Key == draft {
			break
		}
		press(m, "j")
	}
	press(m, "D")
	if !strings.Contains(screen(m), "mark ready this pull request?") {
		t.Fatalf("no popup:\n%s", screen(m))
	}
	press(m, "y")
	if len(r.kinds) != 1 || r.kinds[0] != runner.Ready || r.items[0][0].PR.Key != draft {
		t.Fatalf("batches = %v %+v", r.kinds, r.items)
	}
	if p, _ := m.knownPR(draft); p.Draft {
		t.Error("the row still says draft")
	}
	if !strings.Contains(m.notes[0].title, "marked ready 1") {
		t.Errorf("notes = %+v", m.notes)
	}
}

func TestMarkReadySkipsNonDraft(t *testing.T) {
	m, r := startWith(t, fixture(), keymap.Default())
	press(m, "D")
	if !strings.Contains(screen(m), "not a draft") {
		t.Errorf("popup does not say why:\n%s", screen(m))
	}
	press(m, "y")
	if len(r.items) != 0 {
		t.Errorf("sent: %+v", r.items)
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
	golden(t, "select-rule", m)
	press(m, "j", "enter")
	if len(m.selected) != 1 || m.selected[key("acme", "api", 30)] != byHand {
		t.Fatalf("selected = %v", m.selected)
	}
	if m.tab().folded[plan.GroupBots] {
		t.Error("the bots group stayed folded")
	}
	if r, ok := m.current(); !ok || r.pr.Key != key("acme", "api", 30) {
		t.Errorf("cursor on %v", r.pr.Key)
	}
}

// Rules checked in the popup of S select what they all match; with none
// checked, the rule under the cursor does.
func TestSelectByRulesTogether(t *testing.T) {
	set := testSettings()
	set.Rules = []plan.Rule{{Name: "small", MaxLines: 100}, {Name: "bots", Bot: new(true)}}
	m := startSet(t, set, 130)
	press(m, "S", "j", "space", "j", "space")
	if !strings.Contains(screen(m), "matches · small + bots · 1 PRs") {
		t.Errorf("the matches do not name the rules put together:\n%s", screen(m))
	}
	// The cursor on another rule does not change what is chosen.
	press(m, "k", "k", "enter")
	if len(m.selected) != 1 || !m.isSelected(key("acme", "api", 30)) {
		t.Fatalf("selected = %v, want only the small bot pull request", m.selected)
	}
	if got := status(m); !strings.Contains(got, "small + bots: selected 1") {
		t.Errorf("status = %q", got)
	}
	// A check taken off again leaves the rule under the cursor.
	press(m, "S", "j", "space", "space", "enter")
	if len(m.selected) != 2 || !m.isSelected(key("acme", "api", 12)) {
		t.Errorf("selected = %v, want both small pull requests", m.selected)
	}
	// Rules that share nothing select nothing, and say so.
	set.Rules = append(set.Rules, plan.Rule{Name: "alice", Authors: []string{"alice"}})
	m = startSet(t, set, 130)
	press(m, "S", "j", "j", "space", "j", "space", "enter")
	if len(m.selected) != 0 || !strings.Contains(status(m), "bots + alice: nothing matches in this tab") {
		t.Errorf("selected = %v, status = %q", m.selected, status(m))
	}
}

// Adding keeps the selection and adds what the rules match, so rules put
// together and then added make (A AND B) OR C.
func TestAddByRule(t *testing.T) {
	set := testSettings()
	set.Rules = []plan.Rule{{Name: "small", MaxLines: 100}, {Name: "bots", Bot: new(true)}, {Name: "alice", Authors: []string{"alice"}}}
	m := startSet(t, set, 130)
	press(m, "S", "j", "space", "j", "space", "enter")
	if len(m.selected) != 1 || !m.isSelected(key("acme", "api", 30)) {
		t.Fatalf("selected = %v, want the small bot pull request", m.selected)
	}
	press(m, "S", "j", "j", "j", "a")
	if len(m.selected) != 2 || !m.isSelected(key("acme", "api", 12)) || !m.isSelected(key("acme", "api", 30)) {
		t.Fatalf("selected = %v, want alice's added to the bot's", m.selected)
	}
	if got := status(m); !strings.Contains(got, "alice: added 1, selected 2") {
		t.Errorf("status = %q", got)
	}
	if r, ok := m.current(); !ok || r.pr.Key != key("acme", "api", 12) {
		t.Errorf("cursor on %v, want the pull request added", r.pr.Key)
	}
	// What is already selected is not counted again.
	press(m, "S", "j", "a")
	if len(m.selected) != 2 || !strings.Contains(status(m), "small: added 0, selected 2") {
		t.Errorf("selected = %v, status = %q", m.selected, status(m))
	}
}

// shownKeys are the pull requests the tab's rows show.
func shownKeys(m *Model) []model.PRKey {
	var out []model.PRKey
	t := m.tab()
	for _, r := range m.rowsOf(t, m.filter, t.ruleFilter, nil) {
		if r.kind == rowPR {
			out = append(out, r.pr.Key)
		}
	}
	return out
}

// f in the popup of S narrows the tab's rows to what the rules chosen
// match, with the filter typed, and esc takes both away.
func TestFilterByRule(t *testing.T) {
	f := fixture()
	big := f.searches["review"][2]
	big.ID, big.Key, big.Additions, big.Deletions = "9", key("acme", "api", 31), 800, 200
	f.searches["review"] = append(f.searches["review"], big)
	m := start(t, f, keymap.Default())
	m.set.Rules = []plan.Rule{{Name: "small", MaxLines: 100}, {Name: "bots", Bot: new(true)}}
	m.selected[key("acme", "api", 12)] = byHand
	press(m, "S", "j", "space", "j", "space", "f")
	if got := shownKeys(m); len(got) != 1 || got[0] != key("acme", "api", 30) {
		t.Fatalf("shown = %v, want the small bot pull request", got)
	}
	if !strings.Contains(screen(m), "rule: small + bots · 1 PRs") {
		t.Errorf("no line names the rules:\n%s", screen(m))
	}
	if len(m.selected) != 1 || !m.isSelected(key("acme", "api", 12)) {
		t.Errorf("selected = %v; filtering changed the selection", m.selected)
	}
	// The other tabs are not narrowed.
	if m.tabs[1].ruleFilter != nil {
		t.Error("another tab was narrowed")
	}
	// A group's header selects only the rows shown, not the big bot
	// pull request the rules hide.
	press(m, "esc", "S", "j", "space", "f")
	for range 10 {
		if r, _ := m.current(); r.kind == rowGroup && r.group == plan.GroupBots {
			break
		}
		press(m, "j")
	}
	press(m, "space")
	if !m.isSelected(key("acme", "api", 30)) || m.isSelected(key("acme", "api", 31)) {
		t.Errorf("selected = %v, want only the shown bot pull request", m.selected)
	}
	// With a filter typed, a row must pass both.
	press(m, "esc", "/", "r", "e", "t", "r", "enter")
	if got := shownKeys(m); len(got) != 1 || got[0] != key("acme", "api", 12) {
		t.Errorf("shown = %v, want the small one with retr in its title", got)
	}
	// esc clears the selection first, then both filters.
	press(m, "space", "esc")
	if len(m.selected) != 0 || m.filter == "" {
		t.Fatalf("selected = %v, filter = %q", m.selected, m.filter)
	}
	press(m, "esc")
	if m.filter != "" || m.tab().ruleFilter != nil || len(shownKeys(m)) < 3 {
		t.Errorf("filter = %q, rules = %v, shown = %v", m.filter, m.tab().ruleFilter, shownKeys(m))
	}
}

// A tab narrowed to a rule on the packages reads the bodies of the pull
// requests a reading again brings, so a new one that matches is shown.
func TestFilterByRuleReadsNewBodies(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	press(m, "S", "j", "f")
	if got := shownKeys(m); len(got) != 1 || got[0] != key("acme", "api", 30) {
		t.Fatalf("shown = %v, want the octo bump", got)
	}
	next := f.searches["review"][2]
	next.ID, next.Key, next.HeadOID = "9", key("acme", "api", 33), "eeeeeee9"
	f.searches["review"] = append(f.searches["review"], next)
	f.details[next.Key] = f.details[key("acme", "api", 30)]
	press(m, "r")
	if got := shownKeys(m); len(got) != 2 {
		t.Errorf("shown = %v, want the new octo bump too", got)
	}
}

// The inbox narrowed to a rule on the packages reads the bodies of the
// pull requests a reading again brings, as a search tab does.
func TestFilterByRuleReadsNewBodiesInTheInbox(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	in := m.inboxTab()
	in.ruleFilter = []plan.Rule{m.set.Rules[0]}
	p := f.searches["review"][2]
	delete(m.cache.bodies, p.Key)
	delete(m.cache.details, p.Key)
	in.notifs = append(in.notifs, model.Notification{ThreadID: "n9", Key: p.Key, Title: p.Title, Reason: "review_requested", Updated: now})
	prs := map[model.PRKey]model.PR{p.Key: p}
	maps.Copy(prs, in.inboxPRs)
	_, cmd := m.Update(inboxPRsMsg{gen: in.gen, prs: prs})
	run(m, cmd)
	if _, ok := m.cache.bodies[p.Key]; !ok {
		t.Error("the body of the inbox's new bot pull request was not read")
	}
}

// A rule on the days since an update is matched again as time passes: a
// row narrowed to it leaves when it grows too old, and a selection made
// with it stays.
func TestFilterByRuleOnDays(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	m.set.Rules = []plan.Rule{{Name: "recent", UpdatedWithinDays: 1}}
	press(m, "S", "j", "enter", "S", "j", "f")
	if got := shownKeys(m); len(got) != 2 {
		t.Fatalf("shown = %v, want the two updated within a day", got)
	}
	selected := len(m.selected)
	m.set.Now = func() time.Time { return now.Add(22 * time.Hour) }
	if got := shownKeys(m); len(got) != 1 || got[0] != key("acme", "api", 30) {
		t.Errorf("shown = %v, want only the one updated 20 minutes before", got)
	}
	if len(m.selected) != selected {
		t.Errorf("selected = %v; time passing changed the selection", m.selected)
	}
}

func TestResumeAtStart(t *testing.T) {
	f := fixture()
	set := Settings{Tabs: []Tab{{Name: "review", Query: "review", Groups: plan.Groups, Limit: 100}, {Name: "inbox", Inbox: true, Limit: 100}},
		ThemeDark: "default", ThemeLight: "default", Now: func() time.Time { return now }, CanWait: true, Tick: func(time.Duration, tea.Msg) tea.Cmd { return nil },
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
	if !strings.Contains(screen(m), "another gh-desk") {
		t.Errorf("popup does not say why:\n%s", screen(m))
	}
	press(m, "y")
	if len(r.items) != 0 {
		t.Errorf("a waiting merge started without the lock: %+v", r.items)
	}
}

// fakeLock is the lock another gh-desk holds until free is set.
type fakeLock struct {
	free    bool
	pending []model.Pending
	takes   int
}

func (l *fakeLock) Take() ([]model.Pending, bool, error) {
	l.takes++
	if !l.free {
		return nil, false, nil
	}
	return l.pending, true, nil
}

func (l *fakeLock) Holder() (int, string, bool) { return 4242, "~/src/acme/gh-desk", true }

// moveTo moves the cursor down to k.
func moveTo(t *testing.T, m *Model, k model.PRKey) {
	t.Helper()
	for range 50 {
		if r, ok := m.current(); ok && r.kind == rowPR && r.pr.Key == k {
			return
		}
		press(m, "j")
	}
	t.Fatalf("no row of %s", k)
}

func TestLockedRowAndCursor(t *testing.T) {
	f := fixture()
	f.info = map[model.PRKey]func(*model.MergeInfo){
		key("acme", "api", 12): func(mi *model.MergeInfo) { mi.MergeState = model.MergeBlocked },
	}
	m, _ := startWith(t, f, keymap.Default())
	m.set.CanWait = false
	m.set.Lock = &fakeLock{}
	run(m, m.takeLock())
	k := key("acme", "api", 12)
	moveTo(t, m, k)
	press(m, "M")
	if s := screen(m); !strings.Contains(s, "skip · locked by pid 4242 · would wait") || !strings.Contains(s, "~/src/acme/gh-desk") {
		t.Fatalf("popup does not say what holds it:\n%s", s)
	}
	press(m, "y")
	if pg := m.progress[k]; pg.text != "– locked by pid 4242" || !pg.locked {
		t.Fatalf("row = %+v", pg)
	}
	// The cursor is on it: the status line has the whole of it.
	if !strings.Contains(status(m), "acme/api#12 skipped: it would wait for checks") || !strings.Contains(status(m), "press M again") {
		t.Fatalf("status = %q", status(m))
	}
	press(m, "j")
	if status(m) != "" {
		t.Errorf("status kept after leaving the row: %q", status(m))
	}
	press(m, "k")
	if !strings.Contains(status(m), "acme/api#12 skipped") {
		t.Errorf("status on coming back = %q", status(m))
	}
	// Another message is not cleared by moving.
	m.say("opened in the browser")
	press(m, "j")
	if status(m) != "opened in the browser" {
		t.Errorf("moving cleared another message: %q", status(m))
	}
}

// Tries that would overlap are one: a second try while one is on its way
// could take the lock without the merges to go on with.
func TestTakeLockOnce(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	m.set.CanWait = false
	l := &fakeLock{}
	m.set.Lock = l
	first := m.takeLock()
	if first == nil || m.takeLock() != nil {
		t.Fatal("a second try started while the first was on its way")
	}
	run(m, first)
	if m.takeLock() == nil {
		t.Error("no try after the first came back")
	}
}

func TestTakeLockOver(t *testing.T) {
	f := fixture()
	f.info = map[model.PRKey]func(*model.MergeInfo){
		key("acme", "api", 12): func(mi *model.MergeInfo) { mi.MergeState = model.MergeBlocked },
	}
	m, r := startWith(t, f, keymap.Default())
	m.set.CanWait = false
	waited := model.Pending{Key: key("acme", "web", 7), ID: "9", Head: "h9", Author: "alice"}
	l := &fakeLock{pending: []model.Pending{waited}}
	m.set.Lock = l
	moveTo(t, m, key("acme", "api", 12))
	press(m, "M")
	if !strings.Contains(screen(m), "skip · locked") {
		t.Fatalf("popup with the lock held:\n%s", screen(m))
	}
	press(m, "esc")
	// The other gh-desk quits.
	l.free = true
	press(m, "M")
	if s := screen(m); strings.Contains(s, "skip · locked") || !strings.Contains(s, "The other gh-desk quit") {
		t.Fatalf("popup after it quit:\n%s", s)
	}
	if len(r.resumed) != 1 || r.resumed[0].Key != waited.Key {
		t.Errorf("resumed %+v", r.resumed)
	}
	press(m, "y")
	if n := len(r.items); n == 0 || len(r.items[n-1]) != 1 || r.items[n-1][0].PR.Key != key("acme", "api", 12) {
		t.Errorf("the merge did not start: %+v", r.items)
	}
	takes := l.takes
	press(m, "M")
	press(m, "esc")
	if l.takes != takes {
		t.Error("tried the lock again after taking it")
	}
}

// fakeClaude answers every question at once and keeps what it was asked.
type fakeClaude struct {
	asked    []claude.Request
	prepared []model.PRKey
	// notes are the prompts Once was given.
	notes []string
	// fail makes the next question fail, and wait makes a question wait
	// until it is stopped.
	fail bool
	wait bool
	// prepareErr makes Prepare fail with it.
	prepareErr error
	// plan is what Triage answers, and triaged the requests it was given.
	plan    string
	triaged []claude.Request
}

func (c *fakeClaude) Prepare(_ context.Context, k model.PRKey, head string) (string, error) {
	c.prepared = append(c.prepared, k)
	if c.prepareErr != nil {
		return "", c.prepareErr
	}
	return "/cache/worktrees/" + k.String(), nil
}

func (c *fakeClaude) Ask(ctx context.Context, r claude.Request, progress func(claude.Step)) (claude.Answer, error) {
	c.asked = append(c.asked, r)
	if c.wait {
		<-ctx.Done()
		return claude.Answer{}, ctx.Err()
	}
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

func (c *fakeClaude) Interactive(_ model.PRKey, dir, session string) *exec.Cmd {
	return exec.Command("true")
}

func (c *fakeClaude) Triage(_ context.Context, r claude.Request) (claude.Answer, error) {
	c.triaged = append(c.triaged, r)
	return claude.Answer{Structured: []byte(c.plan)}, nil
}

func (c *fakeClaude) Once(_ context.Context, _, prompt, _ string) (claude.Answer, error) {
	c.notes = append(c.notes, prompt)
	return claude.Answer{Structured: []byte(`{"cautions":[{"key":"acme/api#30","why":"drops support for config v1"}]}`)}, nil
}

func TestClaudeConversation(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	fc := fakeClaudeOf(t, m)
	press(m, "i")
	// Opened, nothing is asked; the worktree is made meanwhile.
	if !m.claudeShown() || len(fc.asked) != 0 || len(fc.prepared) != 1 {
		t.Fatalf("claude shown %v, asked %d, prepared %d", m.claudeShown(), len(fc.asked), len(fc.prepared))
	}
	golden(t, "claude_empty", m)
	press(m, "tab", "enter")
	if len(fc.asked) != 1 {
		t.Fatalf("asked %d", len(fc.asked))
	}
	first := fc.asked[0]
	if !first.First || !strings.Contains(first.Prompt, "<diff>") || !strings.HasSuffix(first.Prompt, "\n"+claude.ReviewQuestion) || first.Dir == "" {
		t.Errorf("first request %+v", first)
	}
	golden(t, "claude", m)

	press(m, "w", "h", "y", "?", "enter")
	if len(fc.asked) != 2 {
		t.Fatalf("asked %d", len(fc.asked))
	}
	if r := fc.asked[1]; r.First || r.Session != first.Session || r.Prompt != "why?" || r.Dir != first.Dir {
		t.Errorf("second request %+v", r)
	}
	// The worktree is put at the head shown before each question.
	if len(fc.prepared) != 3 {
		t.Errorf("the worktree was prepared %d times", len(fc.prepared))
	}

	// Leaving and coming back keeps the conversation.
	press(m, "esc")
	if m.claudeOpen {
		t.Fatal("esc did not go back to the preview")
	}
	press(m, "i")
	if len(fc.asked) != 2 || !strings.Contains(screen(m), "Answer 2") {
		t.Errorf("the conversation was not kept:\n%s", screen(m))
	}
}

// The text box offers a review until one is asked, and tab fills it in
// only into an empty box.
func TestClaudeSuggestsAReview(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	fc := fakeClaudeOf(t, m)
	press(m, "i", "w", "tab")
	if c := m.chat(); !m.typingChat() || c.input.Value() != "w" {
		t.Fatalf("typing %v, box %q", m.typingChat(), c.input.Value())
	}
	press(m, "backspace", "tab", "enter")
	if len(fc.asked) != 1 || !strings.HasSuffix(fc.asked[0].Prompt, "\n"+claude.ReviewQuestion) {
		t.Fatalf("asked %+v", fc.asked)
	}
	press(m, "tab")
	if c := m.chat(); c.input.Value() != "" || c.input.Placeholder == claude.ReviewQuestion || strings.Contains(screen(m), "fill in the suggestion") {
		t.Errorf("a review is offered again: box %q, placeholder %q", c.input.Value(), c.input.Placeholder)
	}
}

// While Claude answers, the claude pane takes every key but help: the list
// does not move under it, backspace goes back while Claude goes on, and a
// click on the list goes back to it.
func TestClaudeWaitingTakesTheKeys(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	press(m, "i")
	c := m.chat()
	chat.NewState(nil, logging.Discard(), func() time.Time { return now }, noStatus{}).Begin(c.Conversation, "And then?")
	stopped := false
	c.cancel = func() { stopped = true }
	sel := m.tab().sel
	press(m, "j", "q", "tab")
	if m.tab().sel != sel || !m.claudeShown() || m.typingChat() {
		t.Fatalf("the list moved, or the pane went or took typing: sel %d, shown %v", m.tab().sel, m.claudeShown())
	}
	if s := screen(m); strings.Contains(s, "ask claude") || !strings.Contains(s, "backspace back, claude goes on") {
		t.Errorf("the pane shows a text box while Claude answers:\n%s", s)
	}
	press(m, "backspace")
	if stopped || m.claudeOpen || !c.Running() {
		t.Errorf("backspace stopped %v, left the pane open %v", stopped, m.claudeOpen)
	}
}

// A pull request is on the screen when its row is in the part of the list
// drawn: not scrolled out, nor filtered out.
func TestOnScreen(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	press(m, "b")
	m.pane.on = false
	// Three lines of rows: the head of the first group, api#12, the head
	// of the second.
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 7})
	api12, api30 := key("acme", "api", 12), key("acme", "api", 30)
	screen(m)
	if !m.onScreen(api12) || m.onScreen(api30) {
		t.Errorf("at the top: api#12 %v, api#30 %v", m.onScreen(api12), m.onScreen(api30))
	}
	press(m, "G")
	screen(m)
	if m.onScreen(api12) || !m.onScreen(api30) {
		t.Errorf("at the bottom: api#12 %v, api#30 %v", m.onScreen(api12), m.onScreen(api30))
	}
	m.filter = "retries"
	screen(m)
	if !m.onScreen(api12) || m.onScreen(api30) {
		t.Errorf("filtered: api#12 %v, api#30 %v", m.onScreen(api12), m.onScreen(api30))
	}
}

// A pull request selected keeps its ✓ while Claude answers about it; its
// name still shows that Claude does.
func TestClaudeAnsweringOnASelectedRow(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	press(m, "i")
	c := m.chat()
	chat.NewState(nil, logging.Discard(), func() time.Time { return now }, noStatus{}).Begin(c.Conversation, "And then?")
	// space selects, and moves the cursor down.
	press(m, "esc", "space")
	if s := screen(m); !strings.Contains(s, "  ✓ api#12") {
		t.Errorf("the selection's mark gave way:\n%s", s)
	}
}

// esc stops what Claude is answering and leaves the pane open; with
// nothing on its way it goes back to the preview, as backspace does.
func TestClaudeStoppedByEsc(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	press(m, "i")
	c := m.chat()
	k := c.PR().Key
	chat.NewState(nil, logging.Discard(), func() time.Time { return now }, noStatus{}).Begin(c.Conversation, "And then?")
	stopped := false
	c.cancel = func() { stopped = true }
	if s := screen(m); !strings.Contains(s, "esc stop") {
		t.Errorf("the key line does not say esc stops:\n%s", s)
	}
	press(m, "esc")
	if !stopped || !m.claudeShown() || !strings.Contains(screen(m), "stopping…") {
		t.Fatalf("stopped %v, shown %v:\n%s", stopped, m.claudeShown(), screen(m))
	}
	m.Update(chatAnswerMsg{key: k, err: context.Canceled})
	ts := c.Turns()
	if last := ts[len(ts)-1]; !last.Stopped || last.Err {
		t.Errorf("the last turn is %+v; want it stopped", last)
	}
	if s := screen(m); !strings.Contains(s, "⏺ stopped") || strings.Contains(s, "claude failed") {
		t.Errorf("the stop is not shown as one:\n%s", s)
	}
	press(m, "esc")
	if m.claudeOpen {
		t.Error("esc with nothing on its way did not go back to the preview")
	}
}

// A question stopped on its way still sends its end, which frees the
// conversation for the next one.
func TestClaudeStopFreesTheConversation(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	fc := fakeClaudeOf(t, m)
	press(m, "i")
	c := m.chat()
	fc.wait = true
	cmd := m.ask(c, "And then?", "And then?", false)
	press(m, "esc")
	run(m, cmd)
	ts := c.Turns()
	if c.Running() || c.cancel != nil || len(ts) != 2 || !ts[1].Stopped {
		t.Fatalf("running %v, turns %+v", c.Running(), ts)
	}
	fc.wait = false
	press(m, "tab", "enter")
	if len(fc.asked) != 2 || !strings.Contains(screen(m), "Answer 2") {
		t.Errorf("the next question was not asked; asked %d:\n%s", len(fc.asked), screen(m))
	}
}

// While Claude answers, the chat turns a spinner with the time waited;
// elsewhere the status line says so, and an answer that came there marks
// the row until the conversation is opened.
func TestClaudeAnsweringElsewhere(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	press(m, "i")
	c := m.chat()
	k := c.PR().Key
	// A question asked 75 seconds ago, still on its way.
	chat.NewState(nil, logging.Discard(), func() time.Time { return now.Add(-75 * time.Second) }, noStatus{}).Begin(c.Conversation, "And then?")
	if s := screen(m); !strings.Contains(s, "thinking… 1m15s") {
		t.Errorf("no time waited:\n%s", s)
	}
	if !m.reading() {
		t.Error("the spinner does not turn while Claude answers")
	}

	// Back on the list, the row tells, and the status line does not.
	press(m, "esc")
	frame := m.claudeFrame()
	if s := screen(m); !strings.Contains(s, "▎ "+frame+" api#12") || strings.Contains(s, "Claude is answering") {
		t.Errorf("the row does not tell Claude is answering, or the status line does too:\n%s", s)
	}
	// Folded, the head of its group tells.
	m.tab().folded[plan.GroupYou] = true
	if s := screen(m); !strings.Contains(s, "requested from you 1 "+frame) || strings.Contains(s, "Claude is answering") {
		t.Errorf("the folded group does not tell:\n%s", s)
	}
	// On a tab without it, only the status line can.
	press(m, "3")
	if s := screen(m); !strings.Contains(s, "Claude is answering on "+k.String()+"…") {
		t.Errorf("the status line does not tell:\n%s", s)
	}
	press(m, "1")
	m.Update(chatAnswerMsg{key: k, answer: claude.Answer{Text: "Later answer."}})
	if s := screen(m); !c.unseen || !strings.Contains(s, "requested from you 1 ✦") {
		t.Errorf("the answer that came elsewhere is not marked on the folded group:\n%s", s)
	}
	m.tab().folded[plan.GroupYou] = false
	if strings.Count(screen(m), "✦ api#12") != 1 {
		t.Errorf("the answer that came elsewhere is not marked:\n%s", screen(m))
	}
	// It is kept in the notifications too, leading to its pull request.
	if m.unseen() != 1 || m.notes[0].title != "Claude answered on "+k.String() || m.notes[0].items[0].key != k {
		t.Errorf("notes = %+v", m.notes)
	}
	press(m, "i")
	if c.unseen {
		t.Error("opening the conversation kept the mark")
	}
}

func TestClaudeDraft(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	press(m, "i", "ctrl+r")
	if m.compose == nil {
		t.Fatal("R did not open the draft")
	}
	if kinds[m.compose.kind] != model.EventApprove || m.compose.ta.Value() != "Looks good: the retry is bounded." {
		t.Errorf("draft kind %v body %q", kinds[m.compose.kind], m.compose.ta.Value())
	}
	press(m, "esc")
	if !m.claudeShown() {
		t.Error("discarding the draft closed the claude pane")
	}
}

// Claude's draft is a review of its conversation's pull request alone,
// whatever is selected when it comes, and waits while another review is
// being written.
func TestClaudeDraftIsOfItsPullRequest(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "i", "esc")
	c := m.chats[key("acme", "api", 12)]
	if c == nil {
		t.Fatal("no conversation")
	}
	m.selected[key("acme", "api", 12)] = byHand
	m.selected[key("acme", "web", 7)] = byHand
	d := claude.Draft{Event: model.EventApprove, Body: "Looks good."}
	run(m, m.draftCompose(c, d))
	if m.compose == nil || len(m.compose.prs) != 1 || m.compose.pr.Key != key("acme", "api", 12) {
		t.Fatalf("the draft is not of its pull request alone: %+v", m.compose)
	}
	m.compose.ta.SetValue("my own words")
	run(m, m.draftCompose(c, d))
	if m.compose.ta.Value() != "my own words" || c.draft == nil {
		t.Errorf("a draft took the place of the review being written: %q, kept %v", m.compose.ta.Value(), c.draft)
	}
}

func TestReleaseNotesMarkBotPullRequests(t *testing.T) {
	f := fixture()
	f.info = map[model.PRKey]func(*model.MergeInfo){
		key("acme", "api", 30): func(mi *model.MergeInfo) { mi.Rollup = model.CheckSuccess },
	}
	m, r := startWith(t, f, keymap.Default())
	m.set.ReleaseNotes = true
	fc := fakeClaudeOf(t, m)
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
	fc := fakeClaudeOf(t, m)
	fc.fail = true
	press(m, "i", "tab", "enter")
	if !strings.Contains(screen(m), "rate limited") {
		t.Fatalf("no error shown:\n%s", screen(m))
	}
	press(m, "o", "k", "enter")
	if len(fc.asked) != 2 {
		t.Fatalf("asked %d", len(fc.asked))
	}
	if r := fc.asked[1]; !r.First || r.Session == fc.asked[0].Session {
		t.Errorf("retry %+v after %+v", r, fc.asked[0])
	}
}

func TestDescriptionWraps(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	m.set.PlainDescription = true
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

func TestTabShowsItsBoundKey(t *testing.T) {
	km, ps := keymap.Apply(keymap.Default(), map[string][]string{"tab_2": {"w"}})
	if len(ps) > 0 {
		t.Fatal(ps)
	}
	m := start(t, fixture(), km)
	header, _, _ := strings.Cut(screen(m), "\n")
	if !strings.Contains(header, "w mine") || strings.Contains(header, "2 mine") {
		t.Errorf("header = %q, want the mine tab marked w", header)
	}
}

// A popup keeps what is left and right of it, colored or wide.
func TestPlaceKeepsTheSides(t *testing.T) {
	for _, c := range []struct {
		base, over string
		x          int
		want       string
	}{
		{"abcdefgh", "XY", 3, "abcXYfgh"},
		{"\x1b[31mabcdefgh\x1b[m", "XY", 3, "abcXYfgh"},
		{"ab", "XY", 4, "ab  XY"},
		// A wide character cut by either edge leaves a gap.
		{"aあいうえ", "XY", 2, "a XY うえ"},
		{"あいうえ", "XY", 2, "あXYうえ"},
	} {
		got := place(c.base, c.over, c.x)
		if ansi.Strip(got) != c.want || ansi.StringWidth(got) != ansi.StringWidth(c.want) {
			t.Errorf("place(%q, %q, %d) = %q, want %q", c.base, c.over, c.x, ansi.Strip(got), c.want)
		}
	}
}

func TestFileTabLineKeepsTheTabShownInSight(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	var files []diff.File
	for _, p := range []string{"compose.prod.yaml", "compose.yaml", "renovate.json", "Dockerfile", "README.md"} {
		files = append(files, diff.File{NewPath: p})
	}
	s, spans := m.fileTabLine(files, 4, 40)
	s = ansi.Strip(s)
	if !strings.HasPrefix(s, "‹ ") || !strings.Contains(s, " README.md ") || ansi.StringWidth(s) > 40 {
		t.Fatalf("line = %q; want the tabs before README.md cut so that it fits in 40 cells", s)
	}
	last := spans[len(spans)-1]
	if last.tab != 4 || string([]rune(s)[last.x:last.x+last.w]) != " README.md " {
		t.Errorf("last span = %+v on %q; want README.md", last, s)
	}
}

// A long name, title and step do not push what is done off the popup:
// it goes under the pull request, and the name keeps its number.
func TestBulkRowsFit(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	long := model.PR{Key: key("octo-org", "platform-bootstrap-templates", 1287),
		Title: "chore(deps): update octo-org/reusable-workflows action to v4.12.0"}
	rows := []bulkRow{
		{pr: model.PR{Key: key("acme", "api", 12), Title: "Add retries to the client"}, mark: "✓", title: "Add retries to the client", what: []string{"approve →", "merge now", "(squash)"}},
		{pr: long, mark: "✓", title: long.Title, what: []string{"approve →", "wait for checks →", "merge", "(merge commit)"},
			notes: []bulkNote{{"⚑", "v4 drops the node16 runtime; jobs that still pin node16 stop running after this update", m.st.peach}}},
	}
	const inner = 90
	got := m.layoutBulk(rows, inner, false)
	var plain []string
	for _, l := range got {
		p := ansi.Strip(l)
		if w := ansi.StringWidth(p); w > inner {
			t.Errorf("%d cells, over %d: %q", w, inner, p)
		}
		plain = append(plain, p)
	}
	want := []string{
		"✓ acme/api#12                          Add retries to the client",
		"  └ approve → merge now (squash)",
		"✓ octo-org/platform-bootstrap-te…#1287 chore(deps): update octo-org/reusable-workflows ac…",
		"  ├ approve → wait for checks → merge (merge commit)",
		"  └ ⚑ v4 drops the node16 runtime; jobs that still pin node16 stop running after this",
		"      update",
	}
	if strings.Join(plain, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(plain, "\n"), strings.Join(want, "\n"))
	}
}

// On a narrow popup, the steps under a pull request are broken between
// steps, never cut: a merge as an admin and an unusual method stay in
// sight (ADR-075).
func TestBulkStepsBreakOnANarrowPopup(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	rows := []bulkRow{
		{pr: model.PR{Key: key("me", "desk", 88), Title: "Let the log viewer follow new lines"}, mark: "✓", title: "Let the log viewer follow new lines",
			what: m.arrowed([]string{"ready", "wait for checks", "merge as admin if blocked"})},
		{pr: model.PR{Key: key("me", "blog", 21), Title: "Add an archive page"}, mark: "✓", title: "Add an archive page",
			what:  append(m.arrowed([]string{"update the branch", "wait for checks", "merge"}), "(squash)"),
			notes: []bulkNote{{"⚠", "changes .github/workflows/ci.yaml", m.st.yellow}}},
	}
	const inner = 36
	var plain []string
	for _, l := range m.layoutBulk(rows, inner, false) {
		p := ansi.Strip(l)
		if w := ansi.StringWidth(p); w > inner {
			t.Errorf("%d cells, over %d: %q", w, inner, p)
		}
		plain = append(plain, p)
	}
	want := []string{
		"✓ me/desk#88 Let the log viewer fol…",
		"  └ ready → wait for checks →",
		"    merge as admin if blocked",
		"✓ me/blog#21 Add an archive page",
		"  ├ update the branch →",
		"  │ wait for checks → merge (squash)",
		"  └ ⚠ changes",
		"      .github/workflows/ci.yaml",
	}
	if strings.Join(plain, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(plain, "\n"), strings.Join(want, "\n"))
	}
}

// Enter answers a confirm popup as its yes does.
func TestEnterConfirms(t *testing.T) {
	m, r := startWith(t, fixture(), keymap.Default())
	press(m, "A", "enter")
	if len(r.items) != 1 || r.kinds[0] != runner.Review {
		t.Errorf("enter did not confirm the approve: batches = %+v", r.items)
	}
}

// r says it is refreshing while the reading lasts, and no longer; what is
// said meanwhile goes first, and the clock's reading says nothing.
func TestRefreshStatus(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	_, cmd := m.Update(keyMsg("r"))
	if s := screen(m); !strings.Contains(s, "refreshing…") {
		t.Fatalf("no refreshing line while reading:\n%s", s)
	}
	m.say("opened in the browser")
	if s := screen(m); strings.Contains(s, "refreshing…") || !strings.Contains(s, "opened in the browser") {
		t.Errorf("the refreshing line hid another message:\n%s", s)
	}
	clock := now.Add(statusline.InfoFor)
	m.set.Now = func() time.Time { return clock }
	run(m, cmd)
	if m.refreshing != nil || strings.Contains(screen(m), "refreshing") {
		t.Errorf("the refreshing line stayed after the reading:\n%s", screen(m))
	}
	_, cmd = m.Update(tickMsg{})
	if strings.Contains(screen(m), "refreshing") {
		t.Errorf("the clock's reading said refreshing:\n%s", screen(m))
	}
	run(m, cmd)
}

// Reading the lists again is logged once it ends, by r and by the clock,
// with how long it took, so the log tells what the list was as of.
func TestRefreshIsLogged(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	var buf bytes.Buffer
	m.log = logging.New(&buf, slog.LevelInfo)
	clock := now
	m.set.Now = func() time.Time { return clock }
	_, cmd := m.Update(keyMsg("r"))
	clock = clock.Add(2 * time.Second)
	run(m, cmd)
	_, cmd = m.Update(tickMsg{})
	run(m, cmd)
	var got []string
	for line := range strings.Lines(buf.String()) {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatal(err)
		}
		if rec["msg"] == "refresh" {
			if _, ok := rec["excluded"]; !ok {
				t.Errorf("refresh logged without how many rows the rules leave out: %v", rec)
			}
			got = append(got, fmt.Sprint(rec[logging.KeyOp], " by ", rec["by"], " ", rec[logging.KeyDurationMS], "ms"))
		}
	}
	if want := []string{"refresh by r 2000ms", "refresh by clock 0ms"}; !slices.Equal(got, want) {
		t.Errorf("refreshes logged = %q, want %q", got, want)
	}
}

// A batch keeps each pull request's steps from the runner's events: the
// forecast is replaced at each decision, a step begun ends the one
// before, a wait that changes what it waits on is a step of its own, and
// a forecast leaves the row's progress as it was.
func TestBatchTracksSteps(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	clock := now
	m.set.Now = func() time.Time { return clock }
	k := key("acme", "api", 12)
	m.batches["b1"] = &batchState{kind: runner.Merge, goes: map[model.PRKey]bool{k: true}, done: map[model.PRKey]progress{}}
	ch := make(chan runner.Event)
	feed := func(after time.Duration, ev runner.Event) {
		clock = clock.Add(after)
		ev.Key = k
		m.event(eventMsg{batch: "b1", ev: ev, ok: true, ch: ch})
	}
	feed(0, runner.Event{Forecast: true, Plan: []runner.Step{runner.StepReady, runner.StepApprove, runner.StepMerge}})
	feed(0, runner.Event{Step: runner.StepReady, Text: "marking ready"})
	feed(time.Second, runner.Event{Step: runner.StepApprove, Text: "approving"})
	feed(2*time.Second, runner.Event{Forecast: true, Plan: []runner.Step{runner.StepWait, runner.StepMerge}})
	if got := m.progress[k].text; got != "approving" {
		t.Errorf("a forecast changed the row's progress to %q", got)
	}
	feed(0, runner.Event{Step: runner.StepWaitApproval, Text: "waiting for approval checks", Checks: []string{"policy-bot"}})
	feed(4*time.Second, runner.Event{Step: runner.StepWaitChecks, Text: "waiting for checks", Checks: []string{"build", "lint"}})
	l := m.batches["b1"].lanes[k]
	var done []string
	for _, s := range l.done {
		done = append(done, fmt.Sprint(s.step, " ", s.took))
	}
	if want := []string{"ready 1s", "approve 2s", "wait-approval-checks 4s"}; !slices.Equal(done, want) {
		t.Errorf("done = %q, want %q", done, want)
	}
	if l.cur == nil || l.cur.step != runner.StepWaitChecks || !slices.Equal(l.checks, []string{"build", "lint"}) {
		t.Errorf("on %+v waiting for %q", l.cur, l.checks)
	}
	if !slices.Equal(l.next, []runner.Step{runner.StepMerge}) {
		t.Errorf("ahead = %q, want the merge only", l.next)
	}
	feed(3*time.Second, runner.Event{Step: runner.StepMerge, Text: "merging"})
	feed(time.Second, runner.Event{Result: runner.Merged, Text: "merged"})
	if l.cur != nil || len(l.next) != 0 || len(l.done) != 5 || l.done[4].took != time.Second {
		t.Errorf("after the merge: done %+v, on %+v, ahead %q", l.done, l.cur, l.next)
	}
}

// esc at the top of the list does not quit; it says q does.
func TestBackDoesNotQuitAtTop(t *testing.T) {
	m := startSet(t, testSettings(), 100)
	_, cmd := m.Update(keyMsg("esc"))
	if quits(cmd) {
		t.Error("esc at the top of the list quit")
	}
	if s := screen(m); !strings.Contains(s, "esc: nothing to clear; q quits") {
		t.Errorf("esc at the top did not say how to quit:\n%s", s)
	}
	_, cmd = m.Update(keyMsg("q"))
	if !quits(cmd) {
		t.Error("q did not quit")
	}
}

// quits reports whether cmd, or a command it batches, quits.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case tea.QuitMsg:
		return true
	case tea.BatchMsg:
		return slices.ContainsFunc(msg, quits)
	}
	return false
}

// The inbox reads every field of its pull requests at once, past the reads
// of heavy fields; the single checks of one only in the inbox are read
// too, a batch at a time, so that a rule on the checks can tell it.
func TestInboxReadsTheSingleChecksOfItsOwn(t *testing.T) {
	f := fixture()
	for i := range checksBatch + 1 {
		k := key("acme", "deploy", 100+i)
		f.extra[k] = model.PR{ID: fmt.Sprintf("d%d", i), Key: k, Title: "Wait for a deploy", State: model.StateOpen,
			Checks: model.CheckPending, HeadOID: fmt.Sprintf("e%d", i)}
		f.notifs = append(f.notifs, model.Notification{ThreadID: fmt.Sprintf("d%d", i), Key: k, Title: "Wait for a deploy", Reason: "subscribed",
			Updated: now.Add(-time.Hour)})
	}
	set := testSettings()
	set.Plan.ApprovalChecks = []string{"deploy/*"}
	m := New(f, &fakeRunner{results: map[model.PRKey]runner.Event{}}, &fakeClaude{}, keymap.Default(), nil, set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	run(m, m.Init())
	for i := range checksBatch + 1 {
		if _, ok := m.cache.info[key("acme", "deploy", 100+i)]; !ok {
			t.Fatalf("the single checks of acme/deploy#%d are not read; reads: %v", 100+i, f.infoReads)
		}
	}
	for _, n := range f.infoReads {
		if n > checksBatch {
			t.Errorf("a read asked for %d pull requests, more than %d", n, checksBatch)
		}
	}
}

// Close is reached from the menu, with no key of its own. Its popup holds
// the check and the text box from the start; keys are the popup's until i
// puts the focus in the box, and the comment comes back after esc.
func TestClose(t *testing.T) {
	f := fixture()
	m, r := startWith(t, f, keymap.Default())
	if ks := m.keys.Keys("close"); len(ks) != 0 {
		t.Fatalf("close has keys %v", ks)
	}
	openClose := func() {
		t.Helper()
		press(m, "enter")
		for i, it := range m.menuItems() {
			if it.action == "close" {
				m.menu.sel = i
			}
		}
		press(m, "enter")
		if m.bulk == nil || m.bulk.close == nil {
			t.Fatal("choosing close did not open its popup")
		}
	}
	openClose()
	golden(t, "close-confirm", m)
	// y is the popup's key while the box has no focus: typing it would
	// confirm, so the test types with the focus in the box only.
	press(m, "x", "i", "L", "G", "T", "M")
	if got := m.bulk.close.ta.Value(); got != "LGTM" || !m.bulk.close.del {
		t.Fatalf("comment = %q, delete = %v", got, m.bulk.close.del)
	}
	if _, cur := m.render(); cur == nil {
		t.Error("the text box with the focus gave the terminal no cursor")
	}
	golden(t, "close-writing", m)
	press(m, "esc")
	if m.bulk == nil || m.bulk.close.editing {
		t.Fatal("esc in the box did not give the keys back to the popup")
	}
	if _, cur := m.render(); cur != nil {
		t.Error("a cursor is shown while the box has no focus")
	}
	run(m, m.paste(tea.PasteMsg{Content: "pasted"}))
	if got := m.bulk.close.ta.Value(); got != "LGTM" {
		t.Errorf("a paste outside the box went into it: %q", got)
	}
	// Closed without confirming, the comment comes back.
	press(m, "esc")
	openClose()
	if got := m.bulk.close.ta.Value(); got != "LGTM" {
		t.Fatalf("the comment did not come back: %q", got)
	}
	m.bulk.close.del = true
	press(m, "y")
	if len(r.closes) != 1 || r.closes[0] != (runner.CloseText{Body: "LGTM", DeleteBranch: true}) {
		t.Fatalf("closes = %+v", r.closes)
	}
	p, _ := m.knownPR(key("acme", "api", 12))
	if p.State != model.StateClosed {
		t.Errorf("the row does not show it closed: %s", p.State)
	}
	if m.unsentClose != "" {
		t.Errorf("the comment was kept after it was sent: %q", m.unsentClose)
	}
}

// A head branch that is in a fork, gone, or the base of an open pull
// request is kept, and the popup says so before the user says yes.
func TestCloseKeepsBranches(t *testing.T) {
	f := fixture()
	f.based = map[model.Branch][]int{{Owner: "acme", Repo: "api", Name: "retries"}: {41}}
	f.info = map[model.PRKey]func(*model.MergeInfo){
		key("acme", "api", 30): func(mi *model.MergeInfo) { mi.HeadInFork = true },
	}
	m := start(t, f, keymap.Default())
	press(m, "b", "space", "j", "j", "space")
	press(m, "enter")
	for i, it := range m.menuItems() {
		if it.action == "close" {
			m.menu.sel = i
		}
	}
	press(m, "enter", "x")
	got := map[model.PRKey]string{}
	for _, it := range m.bulk.items {
		got[it.pr.Key] = it.closing.Keep
	}
	if got[key("acme", "api", 12)] != "base of #41" || got[key("acme", "api", 30)] != "in a fork" {
		t.Errorf("kept = %v", got)
	}
	if s := screen(m); !strings.Contains(s, "branch kept: base of #41") || !strings.Contains(s, "0 of 2 can be deleted") {
		t.Errorf("the popup does not say which branches it keeps:\n%s", s)
	}
}

// A comment GitHub took is added to the pane's comments without reading
// the pull request again; a read on its way is thrown away (ADR-067).
func TestSentCommentShown(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	k := key("acme", "api", 12)
	if e := m.cache.details[k]; e == nil || e.loading {
		t.Fatalf("the detail was not read: %+v", e)
	}
	run(m, m.addSentComment(k, "Superseded by #40."))
	cs := m.cache.details[k].val.Comments
	if len(cs) != 1 || cs[0].Author != "me" || cs[0].Body != "Superseded by #40." {
		t.Fatalf("comments = %+v", cs)
	}
	p, _ := m.knownPR(k)
	var pane []string
	for _, l := range m.descLines(p, 80) {
		pane = append(pane, ansi.Strip(l.text))
	}
	if all := strings.Join(pane, "\n"); !strings.Contains(all, "comments (1)") || !strings.Contains(all, "Superseded by #40.") {
		t.Errorf("the pane does not show the comment:\n%s", all)
	}
	m.cache.details[k] = &entry[model.Detail]{loading: true}
	stale := m.cache.details[k]
	m.addSentComment(k, "again")
	if m.cache.details[k] == stale {
		t.Error("a read on its way was kept")
	}
}
