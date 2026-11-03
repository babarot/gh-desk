package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/listcache"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// fakeLists is a cache in memory.
type fakeLists struct {
	lists map[string]listcache.List
	inbox *listcache.Inbox
	puts  []listcache.List
}

func (f *fakeLists) List(q string) (listcache.List, bool) { l, ok := f.lists[q]; return l, ok }
func (f *fakeLists) Inbox() (listcache.Inbox, bool) {
	if f.inbox == nil {
		return listcache.Inbox{}, false
	}
	return *f.inbox, true
}
func (f *fakeLists) PutList(l listcache.List) error {
	f.puts = append(f.puts, l)
	f.lists[l.Query] = l
	return nil
}
func (f *fakeLists) PutInbox(in listcache.Inbox) error { f.inbox = &in; return nil }

var earlier = now.Add(-18 * time.Minute)

// cachedReview is the review tab as an earlier launch read it: one pull
// request that has since gone, and api#12 at an older head.
func cachedReview() *fakeLists {
	old := model.PR{ID: "9", Key: key("acme", "api", 5), Title: "Since merged", Author: "erin", State: model.StateOpen,
		HeadOID: "9999999", Requested: []model.Reviewer{{Login: "me"}}, Updated: earlier, Additions: 7, HeavyHead: "9999999"}
	moved := model.PR{ID: "1", Key: key("acme", "api", 12), Title: "Add retries to the client", Author: "alice", State: model.StateOpen,
		HeadOID: "0000000", Requested: []model.Reviewer{{Login: "me"}}, Updated: earlier, Additions: 1, HeavyHead: "0000000"}
	return &fakeLists{lists: map[string]listcache.List{"review": {Query: "review", Total: 2, Read: earlier, PRs: []model.PR{old, moved}}}}
}

func newModel(f *fakeSource, lists Lists) *Model {
	set := testSettings()
	set.Viewer, set.Lists = "me", lists
	m := New(f, &fakeRunner{results: map[model.PRKey]runner.Event{}}, &fakeClaude{}, keymap.Default(), nil, set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	return m
}

func TestCachedListsShowAtOnce(t *testing.T) {
	lists := cachedReview()
	m := newModel(fixture(), lists)
	// Before anything is read.
	if s := screen(m); !strings.Contains(s, "Since merged") || !strings.Contains(s, "as of") {
		t.Fatalf("the cached list is not shown:\n%s", s)
	}
	run(m, m.Init())
	s := screen(m)
	if strings.Contains(s, "Since merged") || strings.Contains(s, "as of") || !strings.Contains(s, "+40") {
		t.Errorf("the list was not read again:\n%s", s)
	}
	if len(lists.puts) == 0 || len(lists.lists["review"].PRs) != 3 {
		t.Errorf("the new list was not kept: %+v", lists.puts)
	}
	for _, p := range lists.lists["review"].PRs {
		if !p.HeavyRead() {
			t.Errorf("%s was kept without its heavy fields", p.Key)
		}
	}
	if !m.fresh[key("acme", "api", 30)].After(now) {
		t.Error("a pull request the reading added is not marked")
	}
}

// A reading again keeps the rows until every page is in.
func TestReadingAgainSwapsAtOnce(t *testing.T) {
	m := newModel(fixture(), cachedReview())
	cmd := m.loadTab(0)
	next := m.handleData(cmd()) // the first page of two (the fake pages by 2)
	if got := len(m.tabs[0].prs); got != 2 || m.tabs[0].prs[0].Title != "Since merged" {
		t.Fatalf("the rows changed before the last page: %d", got)
	}
	run(m, next)
	if got := len(m.tabs[0].prs); got != 3 {
		t.Errorf("rows after the last page: %d", got)
	}
}

// Heavy fields of an older reading, or of another head, are not put in a
// row.
func TestStaleHeavyIsDropped(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	t0 := m.tabs[0]
	m.heavyBusy++
	m.handleData(heavyMsg{job: heavyJob{tab: 0, gen: t0.gen - 1}, got: map[string]model.Heavy{"1": {HeadOID: "aaaaaaa1", Additions: 999}}})
	m.heavyBusy++
	m.handleData(heavyMsg{job: heavyJob{tab: 0, gen: t0.gen}, got: map[string]model.Heavy{"1": {HeadOID: "older", Additions: 998}}})
	for _, p := range t0.prs {
		if p.Additions >= 998 {
			t.Errorf("stale heavy fields were put in %s: %+v", p.Key, p)
		}
	}
}

// Until heavy fields are read, a row shows a spinner where they go, and a
// rule on the size or the checks does not pick it.
func TestUnreadHeavyFields(t *testing.T) {
	f := fixture()
	f.noHeavy = true
	m := newModel(f, nil)
	run(m, m.Init())
	if !strings.Contains(screen(m), "alice           "+m.spinner()) {
		t.Errorf("no spinner in the heavy columns:\n%s", screen(m))
	}
	for _, r := range []plan.Rule{{Name: "few files", MaxFiles: 2}, {Name: "few lines", MaxLines: 500}} {
		if got := m.ruleMatches(r); len(got) != 0 {
			t.Errorf("%s picked unread pull requests: %v", r.Name, got)
		}
	}
	// While heavy reads are on their way, the tab spins and S waits.
	m.tabs[0].heavyLeft = 1
	if !strings.Contains(screen(m), "1 review ("+m.spinner()+")") {
		t.Errorf("no spinner on the tab:\n%s", screen(m))
	}
	m.picker = &pick{}
	if m.pickReady() {
		t.Error("S is ready before the heavy fields")
	}
}

// A reading again that fails keeps the rows, as of when they were read.
func TestFailedReadingKeepsTheRows(t *testing.T) {
	f := fixture()
	f.searchErr = errString("secondary rate limit")
	m := newModel(f, cachedReview())
	run(m, m.Init())
	s := screen(m)
	if !strings.Contains(s, "Since merged") || !strings.Contains(s, "as of") || !strings.Contains(s, "could not update") {
		t.Errorf("the rows or the reason are gone:\n%s", s)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

// An empty list is kept too, so the next launch does not show an old one.
func TestEmptyListIsKept(t *testing.T) {
	lists := cachedReview()
	m := newModel(fixture(), lists)
	run(m, m.Init())
	if l, ok := lists.lists["involved"]; !ok || len(l.PRs) != 0 {
		t.Errorf("involved kept = %v, %+v", ok, l)
	}
}

// The cursor moved on the cached list stays on its pull request when the
// reading again is put in place.
func TestCursorStaysAfterTheSwap(t *testing.T) {
	m := newModel(fixture(), cachedReview())
	press(m, "j")
	r, _ := m.current()
	if r.pr.Key != key("acme", "api", 12) {
		t.Fatalf("cursor on %v", r.pr.Key)
	}
	run(m, m.Init())
	if r, _ := m.current(); r.pr.Key != key("acme", "api", 12) {
		t.Errorf("cursor moved to %v", r.pr.Key)
	}
}

// The merge popup lists its pull requests at once, reads them again five at
// a time, fills in each as it comes, and takes y only when all are read.
func TestPopupFillsInAsItReads(t *testing.T) {
	m, r := startWith(t, fixture(), keymap.Default())
	var prs []model.PR
	for _, tt := range m.tabs {
		prs = append(prs, tt.prs...)
	}
	for i := len(prs); i < 7; i++ {
		prs = append(prs, model.PR{ID: fmt.Sprintf("x%d", i), Key: key("acme", "extra", i), Title: fmt.Sprintf("Extra %d", i), HeadOID: "e"})
	}
	cmd := m.startBulkFor(runner.Merge, runner.ReviewText{}, prs)
	parts := flatten(cmd)
	if len(parts) != 2 {
		t.Fatalf("%d reads, want 2 of 5 and 2", len(parts))
	}
	if s := screen(m); !strings.Contains(s, "Add retries to the client") || !strings.Contains(s, "reading…") || !strings.Contains(s, "▱▱▱▱▱▱▱ 0 of 7 read") {
		t.Errorf("rows are not listed before they are read:\n%s", s)
	}
	m.Update(parts[0]())
	if s := screen(m); !strings.Contains(s, "5 of 7 read") || !strings.Contains(s, "merge now") {
		t.Errorf("the first five did not fill in:\n%s", s)
	}
	press(m, "y")
	if len(r.items) != 0 || m.bulk == nil {
		t.Fatalf("y went through before every pull request was read: %v", r.items)
	}
	m.Update(parts[1]())
	if m.bulk.loading || !strings.Contains(screen(m), "? y yes") {
		t.Errorf("not ready after the last read:\n%s", screen(m))
	}
}

// A read that brings back several pull requests at once fills the bar a
// cell at a time, and the popup asks only once it has been full a moment.
func TestPopupBarFillsCellByCell(t *testing.T) {
	m, r := startWith(t, fixture(), keymap.Default())
	m.set.BarStep = BarStep
	clock := now
	m.set.Now = func() time.Time { return clock }
	prs := m.tabs[0].prs[:3]
	parts := flatten(m.startBulkFor(runner.Merge, runner.ReviewText{}, prs))
	if len(parts) != 1 {
		t.Fatalf("%d reads, want 1", len(parts))
	}
	m.Update(parts[0]())
	for i, want := range []string{"▰▱▱ 1 of 3 read", "▰▰▱ 2 of 3 read", "▰▰▰ 3 of 3 read"} {
		clock = now.Add(time.Duration(i) * BarStep)
		if s := screen(m); !strings.Contains(s, want) {
			t.Errorf("after %d steps, no %q:\n%s", i, want, s)
		}
	}
	press(m, "y")
	if len(r.items) != 0 {
		t.Fatalf("y went through while the bar was filling: %v", r.items)
	}
	clock = now.Add((2 + barHold) * BarStep)
	if s := screen(m); !strings.Contains(s, "? y yes") {
		t.Errorf("not asking once the bar was shown full:\n%s", s)
	}
	press(m, "y")
	if len(r.items) != 1 {
		t.Errorf("batches = %v", r.items)
	}
}

// flatten is the commands of a batch, each to run alone.
func flatten(cmd tea.Cmd) []tea.Cmd {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Cmd
		for _, c := range b {
			out = append(out, flatten(c)...)
		}
		return out
	}
	return []tea.Cmd{func() tea.Msg { return msg }}
}
