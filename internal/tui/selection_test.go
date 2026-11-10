package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// d on a selection puts away every selected pull request of the tab, and
// they leave the selection, so a later batch does not take what is out of
// sight.
func TestHideSelection(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	unfoldBots(m)
	press(m, "V")
	n := len(m.selected)
	if n < 2 {
		t.Fatalf("selected = %v", m.selected)
	}
	press(m, "d")
	if len(m.selected) != 0 {
		t.Errorf("hidden pull requests stayed selected: %v", m.selected)
	}
	if len(m.tab().hidden) != n {
		t.Errorf("hidden = %v, want %d", m.tab().hidden, n)
	}
	press(m, "M")
	if m.bulk != nil {
		for _, it := range m.bulk.items {
			if m.tab().hidden[it.pr.Key] {
				t.Errorf("a merge took %s, which was hidden", it.pr.Key)
			}
		}
	}
}

// In the inbox, d on a selection marks each notification done.
func TestDoneSelection(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	press(m, "4", "a", "V", "d")
	if len(f.done) != 2 {
		t.Fatalf("done = %v", f.done)
	}
	if len(m.selected) != 0 {
		t.Errorf("selection = %v", m.selected)
	}
}

// What is selected in another tab stays selected when d puts away this
// tab's.
func TestHideSelectionKeepsOtherTabs(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	unfoldBots(m)
	press(m, "V")
	n := len(m.selected)
	if !m.isSelected(key("acme", "api", 12)) {
		t.Fatalf("selected = %v", m.selected)
	}
	press(m, "4", "d")
	if len(f.done) != 1 || f.done[0] != "acme/api#12" {
		t.Fatalf("done = %v", f.done)
	}
	if len(m.selected) != n-1 || m.isSelected(key("acme", "api", 12)) {
		t.Errorf("selection = %v, want the other %d", m.selected, n-1)
	}
}

// o on a selection opens each selected pull request.
func TestBrowseSelection(t *testing.T) {
	f := fixture()
	for _, prs := range f.searches {
		for i := range prs {
			prs[i].URL = "https://github.com/" + prs[i].Key.Owner + "/" + prs[i].Key.Repo + "/pull/" + strconv.Itoa(prs[i].Key.Number)
		}
	}
	m := start(t, f, keymap.Default())
	unfoldBots(m)
	press(m, "V")
	var want []string
	for _, p := range m.menuTargets() {
		if p.URL != "" {
			want = append(want, p.URL)
		}
	}
	if len(want) < 2 {
		t.Fatalf("selected = %v", m.selected)
	}
	press(m, "o")
	if !slices.Equal(f.browsed, want) {
		t.Errorf("browsed %v, want %v", f.browsed, want)
	}
}

// y copies the URL of the pull request under the cursor or, with a
// selection, of each selected one, a line each; the selection stays.
func TestCopyURL(t *testing.T) {
	f := fixture()
	for _, prs := range f.searches {
		for i := range prs {
			prs[i].URL = "https://github.com/" + prs[i].Key.Owner + "/" + prs[i].Key.Repo + "/pull/" + strconv.Itoa(prs[i].Key.Number)
		}
	}
	m := start(t, f, keymap.Default())
	unfoldBots(m)
	p, _, ok := m.currentPR()
	if !ok {
		t.Fatal("no pull request under the cursor")
	}
	if got := clipboard(t, m, "y"); got != p.URL {
		t.Errorf("copied %q, want %q", got, p.URL)
	}
	if status(m) != "copied "+p.URL {
		t.Errorf("status = %q", status(m))
	}

	press(m, "V")
	n := len(m.selected)
	var want []string
	for _, p := range m.menuTargets() {
		want = append(want, p.URL)
	}
	if len(want) < 2 {
		t.Fatalf("selected = %v", m.selected)
	}
	if got := clipboard(t, m, "y"); got != strings.Join(want, "\n") {
		t.Errorf("copied %q, want %q", got, strings.Join(want, "\n"))
	}
	if status(m) != fmt.Sprintf("copied %d URLs", len(want)) {
		t.Errorf("status = %q", status(m))
	}
	if len(m.selected) != n {
		t.Errorf("selection = %v, want it kept", m.selected)
	}
}

// clipboard presses k and returns what it sets the clipboard to.
func clipboard(t *testing.T, m *Model, k string) string {
	t.Helper()
	_, cmd := m.Update(keyMsg(k))
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			queue = append(queue, msg...)
		default:
			// Bubble Tea does not export the message that sets the clipboard.
			if fmt.Sprintf("%T", msg) == "tea.setClipboardMsg" {
				return fmt.Sprint(msg)
			}
		}
	}
	t.Fatalf("%s set no clipboard", k)
	return ""
}

// i talks about one pull request; with a selection it is refused.
func TestAskClaudeRefusesSelection(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "V", "i")
	if len(m.chats) != 0 {
		t.Error("a chat opened on a selection")
	}
}

// A review batch of a plan of Claude's acts on the pull requests it
// names: one it did not send leaves the user's selection as it was.
func TestClaudeReviewKeepsTheSelection(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	m.selected[key("acme", "web", 7)] = byHand
	k := key("acme", "api", 12)
	bs := &batchState{kind: runner.Review, review: runner.ReviewText{Event: model.EventApprove}, byClaude: true,
		goes: map[model.PRKey]bool{k: true}, order: []model.PRKey{k},
		done: map[model.PRKey]progress{k: {result: runner.Stopped}}}
	run(m, m.noteBatch(bs))
	if len(m.selected) != 1 || !m.isSelected(key("acme", "web", 7)) || m.retry != nil {
		t.Errorf("selection = %v, retry = %v", m.selected, m.retry)
	}
}

// d puts away only what the tab shows: a selected notification the filter
// leaves out is neither marked done nor taken out of the selection.
func TestDoneSkipsWhatIsNotShown(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	press(m, "4", "a", "V")
	if len(m.selected) != 2 {
		t.Fatalf("selected = %v", m.selected)
	}
	press(m, "/", "r", "e", "t", "r", "i", "e", "s", "enter", "d")
	if len(f.done) != 1 || f.done[0] != "acme/api#12" {
		t.Errorf("done = %v, want only the one shown", f.done)
	}
	if len(m.selected) != 1 || !m.isSelected(key("acme", "api", 9)) {
		t.Errorf("selection = %v, want the one not shown", m.selected)
	}
}

// A folded group is shown by its header: d on its selected pull requests
// puts them away.
func TestHideFoldedGroup(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	rs := m.rows(m.tab())
	at := -1
	for i, r := range rs {
		if r.kind == rowGroup && m.tab().sections[r.section].Group == plan.GroupBots {
			at = i
		}
	}
	if at < 0 || !m.tab().folded[string(plan.GroupBots)] {
		t.Fatalf("no folded bots group in %+v", rs)
	}
	m.tab().sel = at
	press(m, "space", "d")
	if !m.tab().hidden[key("acme", "api", 30)] {
		t.Errorf("the folded group's pull request was not hidden: %v", m.tab().hidden)
	}
}

// A done that failed leaves nothing selected: the retry key marks those
// done again, alone.
func TestFailedDoneTriedAgain(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	m.src = failingDone{f}
	press(m, "4", "a", "V", "d")
	if len(m.selected) != 0 || m.retry == nil || len(m.retry.notes) != 2 {
		t.Fatalf("selection = %v, retry = %+v", m.selected, m.retry)
	}
	m.src = f
	press(m, ".")
	if m.retry != nil || len(f.done) != 2 {
		t.Errorf("retry = %+v, done = %v", m.retry, f.done)
	}
}

// An action confirmed takes its pull requests out of the selection, and u
// brings them back for the next one, so A, u, M merges what was approved.
func TestApproveThenRestoreThenMerge(t *testing.T) {
	m, r := startWith(t, fixture(), keymap.Default())
	press(m, "space", "space", "k", "A", "y")
	if len(r.reviews) != 1 || len(r.items[0]) != 2 {
		t.Fatalf("reviews = %+v, items = %+v", r.reviews, r.items)
	}
	if len(m.selected) != 0 {
		t.Fatalf("selection left after approve: %v", m.selected)
	}
	press(m, "u")
	if len(m.selected) != 2 {
		t.Fatalf("u did not bring both back: %v", m.selected)
	}
	press(m, "M")
	if m.bulk == nil || len(m.bulk.items) != 2 {
		t.Errorf("the merge popup does not list both: %+v", m.bulk)
	}
}

// u clears the selection, and again brings it back; a popup closed
// without confirming spends nothing.
func TestClearAndCancelKeepTheLastSelection(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "V", "A", "esc")
	n := len(m.selected)
	if n == 0 {
		t.Fatal("esc in the popup took the selection away")
	}
	press(m, "u")
	if len(m.selected) != 0 {
		t.Fatalf("u left %v", m.selected)
	}
	press(m, "u")
	if len(m.selected) != n {
		t.Errorf("u brought back %v, want %d", m.selected, n)
	}
}

// What was put away since does not come back, so nothing out of sight is
// acted on by a later key (ADR-042).
func TestRestoreLeavesOutWhatWasPutAway(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "space", "space", "k", "u")
	k := m.lastSelected[0]
	for _, tb := range m.tabs {
		tb.hidden[k] = true
	}
	press(m, "u")
	if m.isSelected(k) || len(m.selected) == 0 {
		t.Errorf("selection = %v, with %v put away", m.selected, k)
	}
}

// A batch that ends leaves a selection made meanwhile alone.
func TestBatchEndLeavesANewSelection(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	mine, k := key("acme", "web", 7), key("acme", "api", 12)
	m.selected[mine] = byHand
	bs := &batchState{kind: runner.Merge, goes: map[model.PRKey]bool{k: true}, order: []model.PRKey{k},
		done: map[model.PRKey]progress{k: {result: runner.Failed}}}
	run(m, m.noteBatch(bs))
	if len(m.selected) != 1 || !m.isSelected(mine) {
		t.Errorf("selection = %v", m.selected)
	}
	if m.retry == nil {
		t.Error("a merge that failed left nothing to try again")
	}
}

// A skip is a decision, not a failure: there is nothing to try again.
func TestSkippedIsNotTriedAgain(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	k := key("acme", "api", 12)
	bs := &batchState{kind: runner.Merge, goes: map[model.PRKey]bool{k: true}, order: []model.PRKey{k},
		done: map[model.PRKey]progress{k: {result: runner.Skipped}}}
	run(m, m.noteBatch(bs))
	if m.retry != nil {
		t.Errorf("retry = %+v", m.retry)
	}
	press(m, ".")
	if !strings.Contains(status(m), "nothing to try again") {
		t.Errorf("status = %q", status(m))
	}
}

// unfoldBots unfolds the bots section of the tab shown.
func unfoldBots(m *Model) { m.tab().folded[string(plan.GroupBots)] = false }
