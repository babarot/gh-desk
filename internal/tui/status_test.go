package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/tui/keymap"
	"github.com/babarot/gh-desk/internal/tui/statusline"
)

// clockAt sets the model's clock d after the tests' now.
func clockAt(m *Model, d time.Duration) {
	t := now.Add(d)
	m.set.Now = func() time.Time { return t }
}

// A refusal goes at the next key the TUI takes in.
func TestStatusRefusalGoesAtNextKey(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	m.refuse("you cannot approve your own pull request")
	if kind, _ := m.shownStatus(); kind != statusline.Refusal {
		t.Fatalf("status = %q", status(m))
	}
	press(m, "j")
	if got := status(m); strings.Contains(got, "cannot approve") {
		t.Errorf("a refusal stayed after the next key: %q", got)
	}
}

// Work in progress shows all at once, writes to GitHub first, under an
// event while the event lasts.
func TestStatusActivities(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	m.tab().loading = true
	m.refreshing = &refreshRun{tabs: []int{m.cur}, cleared: 2}
	p, _, _ := m.currentPR()
	m.posting = &posting{prs: []model.PR{p, p}, next: 0}
	m.sending = "2 pull requests"
	want := "sending the comment… 1/2 · refreshing… · 2 results cleared"
	if got := status(m); !strings.HasSuffix(got, want) {
		t.Fatalf("status = %q, want it to end with %q", got, want)
	}
	m.say("canceled")
	if got := status(m); got != "canceled" {
		t.Errorf("an event did not go first: %q", got)
	}
	clockAt(m, statusline.InfoFor)
	if got := status(m); !strings.HasSuffix(got, want) {
		t.Errorf("the work in progress did not come back: %q", got)
	}
}

// The result of the row the cursor moved onto goes before the work in
// progress for a moment, and under it after.
func TestStatusRowResult(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	p, _, _ := m.currentPR()
	m.progress[p.Key] = progress{text: "– conflicts"}
	m.tab().loading = true
	m.refreshing = &refreshRun{tabs: []int{m.cur}}
	m.movedOnto()
	row := p.Key.String() + ": – conflicts"
	if got := status(m); got != row {
		t.Fatalf("status = %q, want the row's result", got)
	}
	clockAt(m, statusline.RowFor)
	if got := status(m); !strings.Contains(got, "refreshing…") {
		t.Errorf("the row's result hid the work in progress after its moment: %q", got)
	}
	m.refreshing = nil
	if got := status(m); got != row {
		t.Errorf("the row's result is not under the work in progress: %q", got)
	}
}

// A reading again that keeps failing is kept in the notifications once,
// and again only after a reading that went through.
func TestFailedReadingNotedOnce(t *testing.T) {
	f := fixture()
	f.searchErr = errString("secondary rate limit")
	m := newModel(f, cachedReview())
	run(m, m.Init())
	noted := func() int {
		n := 0
		for _, x := range m.notes {
			if strings.Contains(x.title, "could not update review") {
				n++
			}
		}
		return n
	}
	if noted() != 1 {
		t.Fatalf("notes = %d, want 1", noted())
	}
	run(m, m.reload())
	if noted() != 1 {
		t.Errorf("a failure again was noted again: %d", noted())
	}
	f.searchErr = nil
	run(m, m.reload())
	f.searchErr = errString("secondary rate limit")
	run(m, m.reload())
	if noted() != 2 {
		t.Errorf("a failure after a reading that went through was not noted: %d", noted())
	}
}
