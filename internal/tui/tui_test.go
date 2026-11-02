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
	"github.com/babarot/gh-triage/internal/diff"
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
	posted  []string
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
			switch kind {
			case runner.Approve:
				ev = runner.Event{Result: runner.Approved, Text: "approved", Approved: true}
			case runner.Ready:
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
		Rules:   []plan.Rule{{Name: "octo bumps", Author: "renovate", Packages: []string{"octo/app"}}},
		CanWait: true, Language: "English",
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
	golden(t, "preview-diff", screen(m))
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
	golden(t, "action-menu", screen(m))
	press(m, "esc")
	if m.menu != nil || m.view != viewList {
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
		t.Fatalf("c in the menu did not start a comment: view %v", m.view)
	}
}

// Under a popup the screen is drawn in the theme's faint color alone, so
// only the popup keeps its colors.
func TestPopupDimsTheScreen(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	header := func() string { return strings.Split(m.render(), "\n")[0] }
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
	golden(t, "action-menu-selection", screen(m))
	press(m, "R")
	if m.menu == nil || m.compose != nil || !m.statusErr {
		t.Fatalf("review on a selection ran: menu %v, compose %v", m.menu, m.compose)
	}
	press(m, "A")
	if m.menu != nil || m.bulk == nil || m.bulk.kind != runner.Approve {
		t.Fatalf("approve did not open the approve popup for the selection: %v", m.bulk)
	}
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

// c comments on the conversation: no review is sent, so the review asked
// of the viewer stays, and the notification is not marked done.
func TestCommentWithBody(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	press(m, "c", "tab", "ctrl+s")
	if !strings.Contains(m.status, "write the comment first") {
		t.Errorf("empty comment was not refused: %q", m.status)
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
}

// c with a selection sends the same comment to each selected pull
// request. The popup closes at once and the status line counts the
// comment through them; those it failed on are selected again, and c
// starts with the comment for them.
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
	if m.confirm != nil || !strings.Contains(m.status, "being sent") {
		t.Errorf("a second comment was let through while one was on its way: %q", m.status)
	}
	press(m, "esc")
	run(m, cmd)
	if len(f.posted) != 1 || f.posted[0] != "acme/api#12 @dependabot rebase" {
		t.Errorf("posted = %q", f.posted)
	}
	if !strings.Contains(m.status, "failed on 1 of 2") || len(m.selected) != 1 || !m.isSelected(key("acme", "web", 7)) {
		t.Fatalf("the one it failed on is not selected again: %v, %q", m.selected, m.status)
	}
	press(m, "c")
	if m.compose == nil || len(m.compose.prs) != 1 || m.compose.ta.Value() != "@dependabot rebase" {
		t.Fatalf("c does not write the comment again for the one it failed on: %+v", m.compose)
	}
	press(m, "esc", "R")
	if m.compose != nil || !strings.Contains(m.status, "one pull request at a time") {
		t.Errorf("R on a selection opened: %q", m.status)
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

// R writes a review: a comment in it is a review of the head seen.
func TestReviewWithBody(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	press(m, "R", "ctrl+s")
	if !strings.Contains(m.status, "write the review comment first") {
		t.Errorf("empty review was not refused: %q", m.status)
	}
	run(m, m.paste(tea.PasteMsg{Content: "Looks good, one question."}))
	press(m, "ctrl+s", "y")
	if len(f.reviews) != 1 || !strings.HasSuffix(f.reviews[0], "COMMENT aaaaaaa1 Looks good, one question.") || len(f.posted) != 0 {
		t.Errorf("reviews = %q, posted = %q", f.reviews, f.posted)
	}
	if m.compose != nil {
		t.Error("the popup stayed open")
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

// R writes a review in a popup over the list, which stays in sight
// dimmed; esc puts it away and leaves the list as it was. c's popup has no
// kinds.
func TestComposePopup(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "c")
	run(m, m.paste(tea.PasteMsg{Content: "Does this retry on a 429?"}))
	golden(t, "comment", screen(m))
	press(m, "esc", "R")
	run(m, m.paste(tea.PasteMsg{Content: "Does this retry on a 429?"}))
	golden(t, "compose", screen(m))
	press(m, "esc")
	if m.compose != nil || m.view != viewList {
		t.Errorf("esc left compose %v, view %v", m.compose, m.view)
	}
}

func TestOneReviewAtATime(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	press(m, "R")
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
	if m.compose == nil {
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
	for l := range strings.SplitSeq(m.render(), "\n") {
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
	golden(t, "select-rule", screen(m))
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

func TestResumeAtStart(t *testing.T) {
	f := fixture()
	set := Settings{Tabs: []Tab{{Name: "review", Query: "review", Groups: plan.Groups, Limit: 100}, {Name: "inbox", Inbox: true, Limit: 100}},
		ThemeDark: "default", ThemeLight: "default", Now: func() time.Time { return now }, CanWait: true,
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

// fakeLock is the lock another gh-triage holds until free is set.
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

func (l *fakeLock) Holder() (int, string, bool) { return 4242, "~/src/acme/gh-triage", true }

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
	if s := screen(m); !strings.Contains(s, "skip · locked by pid 4242 · would wait") || !strings.Contains(s, "~/src/acme/gh-triage") {
		t.Fatalf("popup does not say what holds it:\n%s", s)
	}
	press(m, "y")
	if pg := m.progress[k]; pg.text != "– locked by pid 4242" || !pg.locked {
		t.Fatalf("row = %+v", pg)
	}
	// The cursor is on it: the status line has the whole of it.
	if !strings.Contains(m.status, "acme/api#12 skipped: it would wait for checks") || !strings.Contains(m.status, "press M again") {
		t.Fatalf("status = %q", m.status)
	}
	press(m, "j")
	if m.status != "" {
		t.Errorf("status kept after leaving the row: %q", m.status)
	}
	press(m, "k")
	if !strings.Contains(m.status, "acme/api#12 skipped") {
		t.Errorf("status on coming back = %q", m.status)
	}
	// Another message is not cleared by moving.
	m.setStatus("refreshing", false)
	press(m, "j")
	if m.status != "refreshing" {
		t.Errorf("moving cleared another message: %q", m.status)
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
	// The other gh-triage quits.
	l.free = true
	press(m, "M")
	if s := screen(m); strings.Contains(s, "skip · locked") || !strings.Contains(s, "The other gh-triage quit") {
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
	// fail makes the next question fail.
	fail bool
	// plan is what Triage answers, and triaged the requests it was given.
	plan    string
	triaged []claude.Request
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
	press(m, "i")
	if len(fc.asked) != 2 || !strings.Contains(screen(m), "Answer 2") {
		t.Errorf("the conversation was not kept:\n%s", screen(m))
	}
}

// While Claude answers, the chat turns a spinner with the time waited;
// elsewhere the status line says so, and an answer that came there marks
// the row until the conversation is opened.
func TestClaudeAnsweringElsewhere(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	press(m, "i")
	k := m.chatKey
	c := m.chat()
	c.running, c.since = true, now.Add(-75*time.Second)
	if s := screen(m); !strings.Contains(s, "thinking… 1m15s") {
		t.Errorf("no time waited:\n%s", s)
	}
	if !m.reading() {
		t.Error("the spinner does not turn while Claude answers")
	}

	press(m, "esc")
	if s := screen(m); !strings.Contains(s, "Claude is answering on "+k.String()+"…") {
		t.Errorf("the list does not say Claude is answering:\n%s", s)
	}
	m.Update(chatAnswerMsg{key: k, answer: claude.Answer{Text: "Later answer."}})
	if !c.unseen || strings.Count(screen(m), "✦") != 1 {
		t.Errorf("the answer that came elsewhere is not marked:\n%s", screen(m))
	}
	press(m, "i")
	if c.unseen {
		t.Error("opening the conversation kept the mark")
	}
}

func TestClaudeDraft(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	press(m, "i", "R")
	if m.compose == nil {
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
	press(m, "i")
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
		{pr: model.PR{Key: key("acme", "api", 12), Title: "Add retries to the client"}, mark: "✓", title: "Add retries to the client", what: "approve → merge now (squash)"},
		{pr: long, mark: "✓", title: long.Title, what: "approve → wait for checks, then merge (merge commit)",
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
		"  ├ approve → wait for checks, then merge (merge commit)",
		"  └ ⚑ v4 drops the node16 runtime; jobs that still pin node16 stop running after this",
		"      update",
	}
	if strings.Join(plain, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(plain, "\n"), strings.Join(want, "\n"))
	}
}

// Enter answers a confirm popup as its yes does.
func TestEnterConfirms(t *testing.T) {
	m, r := startWith(t, fixture(), keymap.Default())
	press(m, "A", "enter")
	if len(r.items) != 1 || r.kinds[0] != runner.Approve {
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
	m.setStatus("opened in the browser", false)
	if s := screen(m); strings.Contains(s, "refreshing…") || !strings.Contains(s, "opened in the browser") {
		t.Errorf("the refreshing line hid another message:\n%s", s)
	}
	m.setStatus("", false)
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
