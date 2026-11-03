package tui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

func TestFirstInbox(t *testing.T) {
	read, changed, unread, fresh := key("acme", "api", 1), key("acme", "api", 2), key("acme", "api", 3), key("acme", "api", 4)
	at := now.Add(-time.Hour)
	tb := &tab{inbox: true,
		notifs:   []model.Notification{{Key: read, Updated: at}, {Key: changed, Updated: at}, {Key: unread, Updated: at}},
		inboxPRs: map[model.PRKey]model.PR{read: {Key: read}, changed: {Key: changed}}}
	ns := []model.Notification{
		{Key: fresh, Updated: now},
		{Key: changed, Updated: now},
		{Key: read, Updated: at},
		{Key: unread, Updated: at},
		{Key: fresh, Updated: now},
	}
	got := firstInbox(tb, ns)
	want := []model.PRKey{fresh, changed, unread}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("first = %v, want %v", got, want)
	}

	ns = nil
	for i := range 30 {
		ns = append(ns, model.Notification{Key: key("acme", "web", i+1), Updated: now})
	}
	if got := firstInbox(&tab{inbox: true, inboxPRs: map[model.PRKey]model.PR{}}, ns); len(got) != firstInboxPRs || got[0] != ns[0].Key {
		t.Errorf("first of a new inbox = %v, want the first %d", got, firstInboxPRs)
	}
}

// readInbox starts a reading of the inbox and returns the messages of its
// read ahead and of its read of every pull request, not yet taken in.
func readInbox(t *testing.T, m *Model) (first inboxFirstMsg, all inboxPRsMsg) {
	t.Helper()
	msg := m.loadTab(m.inbox)()
	cmd := m.handleData(msg)
	if cmd == nil {
		t.Fatal("no reads after the notifications")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("the notifications did not start two reads")
	}
	var gotFirst, gotAll bool
	for _, c := range batch {
		switch msg := c().(type) {
		case inboxFirstMsg:
			first, gotFirst = msg, true
		case inboxPRsMsg:
			all, gotAll = msg, true
		}
	}
	if !gotFirst || !gotAll {
		t.Fatalf("reads: first %v, all %v", gotFirst, gotAll)
	}
	return first, all
}

// A notification that comes in shows its pull request as soon as the read
// ahead is back, before every pull request of the inbox is read.
func TestInboxNewNotificationFirst(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	k := key("acme", "api", 41)
	f.extra[k] = model.PR{ID: "41", Key: k, Title: "Fix the timeout", Author: "erin", State: model.StateOpen, Checks: model.CheckSuccess}
	f.notifs = append([]model.Notification{{ThreadID: "n41", Key: k, Title: "Fix the timeout", Reason: "review_requested", Unread: true, Updated: now}}, f.notifs...)

	first, all := readInbox(t, m)
	in := m.inboxTab()
	run(m, func() tea.Msg { return first })
	if p := in.inboxPRs[k]; p.Author != "erin" || !in.loading {
		t.Errorf("after the read ahead: pr %+v, loading %v; want the pull request while still loading", p, in.loading)
	}
	run(m, func() tea.Msg { return all })
	if p := in.inboxPRs[k]; p.Author != "erin" || in.loading {
		t.Errorf("after every pull request: pr %+v, loading %v", p, in.loading)
	}
}

// A read ahead back after the read of every pull request is dropped: that
// read is the newer word.
func TestInboxReadAheadAfterAllDropped(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	k := key("acme", "api", 41)
	f.extra[k] = model.PR{ID: "41", Key: k, Title: "Fix the timeout", Author: "erin", State: model.StateOpen}
	f.notifs = append([]model.Notification{{ThreadID: "n41", Key: k, Title: "Fix the timeout", Unread: true, Updated: now}}, f.notifs...)

	first, all := readInbox(t, m)
	run(m, func() tea.Msg { return all })
	stale := first
	stale.prs = map[model.PRKey]model.PR{k: {ID: "41", Key: k, Title: "Fix the timeout", Author: "erin", State: model.StateClosed}}
	run(m, func() tea.Msg { return stale })
	if p := m.inboxTab().inboxPRs[k]; p.State != model.StateOpen {
		t.Errorf("state = %v, want what the read of every pull request said", p.State)
	}
}

// What an older reading of the inbox read is dropped once it is read again.
func TestInboxOlderReadingDropped(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	k := key("acme", "api", 41)
	f.extra[k] = model.PR{ID: "41", Key: k, Title: "Fix the timeout", Author: "erin", State: model.StateOpen}
	old := m.loadTab(m.inbox)()

	f.notifs = append([]model.Notification{{ThreadID: "n41", Key: k, Title: "Fix the timeout", Unread: true, Updated: now}}, f.notifs...)
	first, all := readInbox(t, m)
	in := m.inboxTab()
	m.handleData(old)
	if len(in.notifs) == 0 || in.notifs[0].Key != k {
		t.Fatal("notifications of an older reading were taken")
	}
	oldFirst, oldAll := first, all
	oldFirst.gen--
	oldFirst.prs = map[model.PRKey]model.PR{k: {ID: "41", Key: k, Author: "erin", State: model.StateClosed}}
	oldAll.gen--
	oldAll.prs = map[model.PRKey]model.PR{}
	run(m, func() tea.Msg { return oldFirst })
	run(m, func() tea.Msg { return oldAll })
	if !in.loading || len(in.inboxPRs) == 0 || in.inboxPRs[k].State == model.StateClosed {
		t.Errorf("reads of an older reading were taken: loading %v, prs %d, %v", in.loading, len(in.inboxPRs), in.inboxPRs[k].State)
	}
	run(m, func() tea.Msg { return first })
	run(m, func() tea.Msg { return all })
	if in.loading || in.inboxPRs[k].State != model.StateOpen {
		t.Errorf("after its reading: loading %v, state %v", in.loading, in.inboxPRs[k].State)
	}
}
