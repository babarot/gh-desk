package tui

import (
	"bytes"
	"context"
	"image/color"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/listcache"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// offlineSource answers MergeInfo, which a preview's confirm asks, and
// notes every other call: an Offline TUI must make none.
type offlineSource struct {
	*fakeSource
	calls []string
}

func (s *offlineSource) note(name string) { s.calls = append(s.calls, name) }

func (s *offlineSource) Viewer(ctx context.Context) (string, error) {
	s.note("Viewer")
	return s.fakeSource.Viewer(ctx)
}
func (s *offlineSource) SearchPage(ctx context.Context, q string, first int, after string) ([]model.PR, int, string, []string, error) {
	s.note("SearchPage")
	return s.fakeSource.SearchPage(ctx, q, first, after)
}
func (s *offlineSource) Notifications(ctx context.Context, n int) ([]model.Notification, error) {
	s.note("Notifications")
	return s.fakeSource.Notifications(ctx, n)
}
func (s *offlineSource) PullRequests(ctx context.Context, keys []model.PRKey) (map[model.PRKey]model.PR, error) {
	s.note("PullRequests")
	return s.fakeSource.PullRequests(ctx, keys)
}
func (s *offlineSource) Detail(ctx context.Context, k model.PRKey) (model.Detail, error) {
	s.note("Detail")
	return s.fakeSource.Detail(ctx, k)
}
func (s *offlineSource) Diff(ctx context.Context, p model.PR) ([]byte, error) {
	s.note("Diff")
	return s.fakeSource.Diff(ctx, p)
}
func (s *offlineSource) Comment(ctx context.Context, url string) (model.Comment, error) {
	s.note("Comment")
	return s.fakeSource.Comment(ctx, url)
}
func (s *offlineSource) Bodies(ctx context.Context, ids []string) (map[string]string, error) {
	s.note("Bodies")
	return s.fakeSource.Bodies(ctx, ids)
}
func (s *offlineSource) BasedOn(ctx context.Context, bs []model.Branch) (map[model.Branch][]int, error) {
	s.note("BasedOn")
	return s.fakeSource.BasedOn(ctx, bs)
}
func (s *offlineSource) Heavy(ctx context.Context, ids []string) (map[string]model.Heavy, error) {
	s.note("Heavy")
	return s.fakeSource.Heavy(ctx, ids)
}

// offlineModel is the settings page's preview: the lists last read, and
// nothing read.
func offlineModel(lists Lists) (*Model, *offlineSource) {
	src := &offlineSource{fakeSource: fixture()}
	set := testSettings()
	set.Viewer, set.Lists, set.Offline, set.Refresh = "me", lists, true, time.Minute
	m := New(src, &fakeRunner{results: map[model.PRKey]runner.Event{}}, nil, keymap.Default(), nil, set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	run(m, m.Init())
	return m, src
}

func TestOfflineReadsNothing(t *testing.T) {
	m, src := offlineModel(cachedReview())
	s := screen(m)
	if !strings.Contains(s, "as of") || !strings.Contains(s, "Since merged") || strings.Contains(s, "reading the description") {
		t.Fatalf("the cached list is not shown as of when it was read:\n%s", s)
	}
	// Moving, the pane, another tab, S, the inbox and r.
	press(m, "j", "k", "p", "p", "S", "esc", "2", "4", "1", "r")
	if len(src.calls) != 0 {
		t.Errorf("an offline TUI read %v", src.calls)
	}
	if s := screen(m); !strings.Contains(s, "inbox (-)") {
		t.Errorf("the inbox, not in the lists last read, has a count:\n%s", s)
	}
	if !strings.Contains(status(m), "the preview reads nothing") {
		t.Errorf("r does not say the preview reads nothing: %q", status(m))
	}
	press(m, "2")
	if s := screen(m); !strings.Contains(s, "not read in the preview") || strings.Contains(s, "reading mine") {
		t.Errorf("a tab not in the lists last read does not say so:\n%s", s)
	}
	press(m, "1")
	if s := screen(m); !strings.Contains(s, "not read in the preview") {
		t.Errorf("the pane does not say the diff was not read:\n%s", s)
	}
}

func TestOfflineMergeConfirmSaysWhatItAssumes(t *testing.T) {
	m, src := offlineModel(cachedReview())
	press(m, "M")
	if m.bulk == nil {
		t.Fatal("M opened no confirm")
	}
	if s := screen(m); !strings.Contains(s, "preview: assumes the repository allows every method") {
		t.Errorf("the confirm does not say what the preview assumes:\n%s", s)
	}
	if len(src.calls) != 0 {
		t.Errorf("the confirm read %v", src.calls)
	}
}

// offlineSettings are the settings of offlineModel with change applied.
func offlineSettings(m *Model, change func(*Settings)) Settings {
	set := m.set
	set.Tabs = append([]Tab(nil), set.Tabs...)
	change(&set)
	return set
}

func apply(m *Model, set Settings) {
	_, cmd := m.Update(settingsMsg{set: set, keys: m.keys})
	run(m, cmd)
}

func TestSettingsKeepTheTabAndTheCursor(t *testing.T) {
	m, _ := offlineModel(cachedReview())
	press(m, "j")
	before, _, _ := m.currentPR()
	m.tab().hidden[key("acme", "api", 99)] = true
	press(m, "?")
	// A tab before the shown one goes, and another theme is chosen.
	apply(m, offlineSettings(m, func(s *Settings) {
		s.Tabs = append([]Tab{{Name: "new", Query: "new", Limit: 10}}, s.Tabs[1:]...)
		s.Tabs = append(s.Tabs[:1], append([]Tab{{Name: "review", Query: "review", Limit: 100}}, s.Tabs[1:]...)...)
		s.ThemeDark = "tokyonight"
	}))
	if m.tab().query != "review" || m.cur != 1 {
		t.Fatalf("the tab shown moved: %d %q", m.cur, m.tab().query)
	}
	after, _, ok := m.currentPR()
	if !ok || after.Key != before.Key {
		t.Errorf("the cursor moved from %s to %s", before.Key, after.Key)
	}
	if !m.tab().hidden[key("acme", "api", 99)] {
		t.Error("what was hidden came back")
	}
	if !m.help {
		t.Error("the ? list closed")
	}
	if m.tabs[m.inbox].query != "" || !m.tabs[m.inbox].inbox {
		t.Errorf("the inbox is not at %d", m.inbox)
	}
	if !reflect.DeepEqual(m.st, newStyles(theme("tokyonight"))) {
		t.Error("the theme did not change")
	}
}

func TestSettingsDecideTheConfirmAgain(t *testing.T) {
	m, src := offlineModel(cachedReview())
	src.info = map[model.PRKey]func(*model.MergeInfo){}
	for _, p := range src.searches["review"] {
		src.info[p.Key] = func(mi *model.MergeInfo) {
			mi.Repo.Methods = []model.MergeMethod{model.MethodMerge, model.MethodSquash, model.MethodRebase}
		}
	}
	// api#12, which the made-up GitHub answers of.
	press(m, "j", "M")
	method := func() model.MergeMethod {
		t.Helper()
		if m.bulk == nil || len(m.bulk.items) != 1 || !m.bulk.items[0].ok {
			t.Fatalf("the confirm is not of api#12: %+v", m.bulk)
		}
		return m.bulk.items[0].d.Method
	}
	if got := method(); got != model.MethodMerge {
		t.Fatalf("the confirm merges with %q", got)
	}
	apply(m, offlineSettings(m, func(s *Settings) {
		s.Plan.Methods = []model.MergeMethod{model.MethodSquash}
	}))
	if got := method(); got != model.MethodSquash {
		t.Errorf("the confirm still merges with %q", got)
	}
}

func TestSettingsWithFewerRulesKeepSWhole(t *testing.T) {
	m, _ := offlineModel(cachedReview())
	press(m, "S", "j")
	if m.picker == nil || m.picker.sel != 1 {
		t.Fatalf("S is not on the second rule: %+v", m.picker)
	}
	press(m, "space")
	apply(m, offlineSettings(m, func(s *Settings) { s.Rules = nil }))
	if m.picker == nil {
		t.Fatal("S closed")
	}
	if m.picker.sel != 0 || len(m.picker.checked) != 0 {
		t.Errorf("S holds rules that are gone: %+v", m.picker)
	}
	screen(m)
}

func TestSettingsKeepALightTerminalLight(t *testing.T) {
	m, _ := offlineModel(cachedReview())
	m.Update(tea.BackgroundColorMsg{Color: color.White})
	apply(m, offlineSettings(m, func(s *Settings) { s.ThemeDark, s.ThemeLight = "tokyonight", "github-light" }))
	if !reflect.DeepEqual(m.st, newStyles(theme("github-light"))) {
		t.Error("a light terminal took the dark theme")
	}
}

// syncBuffer is what the program draws, read while it runs.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestProgramDrawsOnTheTerminalGiven(t *testing.T) {
	set := testSettings()
	set.Viewer, set.Lists, set.Offline = "me", cachedReview(), true
	in, typed := io.Pipe()
	defer typed.Close()
	var out syncBuffer
	ctx, cancel := context.WithCancel(context.Background())
	p := NewProgram(ctx, in, &out, 120, 30, &offlineSource{fakeSource: fixture()}, &fakeRunner{results: map[model.PRKey]runner.Event{}}, nil, keymap.Default(), nil, set)
	done := make(chan error, 1)
	go func() { done <- p.Run() }()
	waitFor := func(what string) {
		t.Helper()
		for range 200 {
			if strings.Contains(out.String(), what) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("%q was not drawn:\n%s", what, out.String())
	}
	waitFor("Since merged")
	if !strings.Contains(out.String(), "\x1b[38;2;") {
		t.Error("the program does not draw in true color")
	}
	p.Apply(offlineSettings(&Model{set: set}, func(s *Settings) { s.Tabs[0].Name = "zzz" }), keymap.Default())
	waitFor("zzz")
	p.Resize(100, 20)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("a program ended by its context says %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the program did not end with its context")
	}
}

func TestSettingsOnAnEmptyTab(t *testing.T) {
	m, _ := offlineModel(cachedReview())
	same := func(s *Settings) {}
	apply(m, offlineSettings(m, same))
	press(m, "2")
	screen(m)
	apply(m, offlineSettings(m, same))
	screen(m)
	if tb := m.tab(); tb.sel < 0 || tb.offset < 0 {
		t.Errorf("an empty tab's cursor is at %d, %d", tb.sel, tb.offset)
	}
}

// An inbox shown from the lists last read, as the settings page shows it,
// starts on its first notification, not on the head above it: nothing is
// read after to move it there.
func TestCachedInboxStartsOnANotification(t *testing.T) {
	p := model.PR{Key: key("acme", "api", 12), Title: "Add retries to the client", Author: "alice", State: model.StateOpen}
	lists := &fakeLists{lists: map[string]listcache.List{}, inbox: &listcache.Inbox{Read: earlier, PRs: []model.PR{p},
		Notifs: []model.Notification{{ThreadID: "n1", Key: p.Key, Reason: "review_requested", Unread: true, Updated: earlier}}}}
	set := testSettings()
	set.Tabs[3].Sections = []plan.Section{{Name: "asked", Show: plan.ShowOpen, Rule: plan.Rule{Reasons: []string{"review_requested"}}}}
	set.Viewer, set.Lists, set.Offline = "me", lists, true
	m := New(&offlineSource{fakeSource: fixture()}, &fakeRunner{results: map[model.PRKey]runner.Event{}}, nil, keymap.Default(), nil, set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	run(m, m.Init())
	press(m, "4")
	if r, ok := m.current(); !ok || r.kind != rowPR {
		t.Errorf("the cursor is on %+v\n%s", r, screen(m))
	}
}
