package tui

import (
	"slices"
	"testing"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// A refresh reads the description pane of the pull request under the
// cursor again, and the unread comment a notification shows.
func TestRefreshReadsThePaneAgain(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	m.pane.mode = paneOverview
	p, _, ok := m.currentPR()
	if !ok {
		t.Fatal("no pull request under the cursor")
	}
	run(m, m.ensureDetail(p.Key))
	url := "https://api.github.com/repos/acme/api/issues/comments/1"
	run(m, m.ensureComment(url))

	f.details[p.Key] = model.Detail{Body: "edited"}
	f.comments[url] = model.Comment{Author: "dave", Body: "edited too"}
	press(m, "r")
	run(m, m.ensureComment(url))

	if e := m.cache.details[p.Key]; e == nil || e.loading || e.val.Body != "edited" {
		t.Errorf("detail after refresh = %+v, want the edited body", e)
	}
	if e := m.cache.comments[url]; e == nil || e.loading || e.val.Body != "edited too" {
		t.Errorf("comment after refresh = %+v, want the edited comment", e)
	}
}

// A read started before a refresh and back after it is dropped: its entry
// was cleared, or another read's entry took its place.
func TestReadFromBeforeRefreshIsDropped(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	m.pane.mode = paneOverview
	p, _, _ := m.currentPR()
	url := "https://api.github.com/repos/acme/api/issues/comments/1"
	// Reads in flight when r is pressed.
	delete(m.cache.details, p.Key)
	delete(m.cache.comments, url)
	readDetail := m.ensureDetail(p.Key)
	readComment := m.ensureComment(url)
	if readDetail == nil || readComment == nil {
		t.Fatal("reads not started")
	}
	staleDetail, staleComment := readDetail(), readComment()

	f.details[p.Key] = model.Detail{Body: "fresh"}
	press(m, "r")
	// The comment's entry is gone; the detail's is a new read's.
	m.Update(staleComment)
	m.Update(staleDetail)

	if _, ok := m.cache.comments[url]; ok {
		t.Errorf("a stale comment made an entry: %+v", m.cache.comments[url])
	}
	if e := m.cache.details[p.Key]; e == nil || e.val.Body != "fresh" {
		t.Errorf("detail = %+v, want the read after the refresh", e)
	}
}

// The clock reads the tabs not shown too, so that one shown later is as
// fresh as the one shown (ADR-123).
func TestTheClockReadsTabsNotShown(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	if m.tabs[m.cur].name != "review" {
		t.Fatalf("shown tab = %q", m.tabs[m.cur].name)
	}
	mine := slices.IndexFunc(m.tabs, func(tb *tab) bool { return tb.name == "mine" })
	gone := f.searches["mine"][0].Key
	// Merged elsewhere while the review tab was shown.
	f.searches["mine"] = nil
	_, cmd := m.Update(tickMsg{})
	run(m, cmd)
	if slices.ContainsFunc(m.allOf(m.tabs[mine]), func(p model.PR) bool { return p.Key == gone }) {
		t.Error("the mine tab kept a pull request GitHub no longer lists")
	}
}
