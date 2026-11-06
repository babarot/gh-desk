package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// bells carries out cmd as run does and counts the bells it rings.
func bells(m *Model, cmd tea.Cmd) int {
	n := 0
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil, tickMsg, toastMsg, spinMsg, freshMsg, tea.QuitMsg:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case tea.RawMsg:
			if msg.Msg == bell {
				n++
			}
		default:
			_, next := m.Update(msg)
			queue = append(queue, next)
		}
	}
	return n
}

// startBell starts the TUI with list.bell on and set changed by edit.
func startBell(t *testing.T, f *fakeSource, edit func(*Settings)) *Model {
	t.Helper()
	set := testSettings()
	set.Bell = true
	if edit != nil {
		edit(&set)
	}
	m := New(f, &fakeRunner{results: map[model.PRKey]runner.Event{}}, &fakeClaude{}, keymap.Default(), nil, set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	if n := bells(m, m.Init()); n != 0 {
		t.Fatalf("the first reading rang the bell %d times", n)
	}
	return m
}

// rereadInbox reads the inbox again as the clock or r does.
func rereadInbox(m *Model) int { return bells(m, m.loadTab(m.inbox)) }

func TestBellRingsForANewUnreadNotification(t *testing.T) {
	f := fixture()
	m := startBell(t, f, nil)
	if n := rereadInbox(m); n != 0 {
		t.Fatalf("a reading with nothing new rang %d times", n)
	}
	f.notifs = append(f.notifs, model.Notification{ThreadID: "n3", Key: key("acme", "api", 30), Title: "Update dependency",
		Reason: "review_requested", Unread: true, Updated: now})
	if n := rereadInbox(m); n != 1 {
		t.Errorf("a new unread notification rang %d times, want 1", n)
	}
	if n := rereadInbox(m); n != 0 {
		t.Errorf("the same notification rang again %d times", n)
	}
}

func TestBellRingsForAnUpdatedOrUnreadAgainNotification(t *testing.T) {
	f := fixture()
	m := startBell(t, f, nil)
	f.notifs[0].Updated = now
	if n := rereadInbox(m); n != 1 {
		t.Errorf("an updated notification rang %d times, want 1", n)
	}
	f.notifs[0].Unread = false
	if n := rereadInbox(m); n != 0 {
		t.Errorf("a notification read rang %d times", n)
	}
	f.notifs[0].Unread, f.notifs[0].Updated = true, now.Add(time.Minute)
	if n := rereadInbox(m); n != 1 {
		t.Errorf("a notification unread again rang %d times, want 1", n)
	}
}

func TestBellIsOffByDefault(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	f.notifs[0].Updated = now
	if n := rereadInbox(m); n != 0 {
		t.Errorf("list.bell off rang %d times", n)
	}
}

// One put away while its mark of done is on its way, and read again
// unread, is not new.
func TestBellQuietForOnePutAway(t *testing.T) {
	f := fixture()
	m := startBell(t, f, nil)
	n := m.inboxTab().notifs[0]
	m.putAway(n.Key, &n)
	if got := rereadInbox(m); got != 0 {
		t.Errorf("a notification put away rang %d times", got)
	}
}

// The excluded ring no bell, merged or closed or left out by a rule, even
// while they are shown.
func TestBellQuietForTheExcluded(t *testing.T) {
	f := fixture()
	m := startBell(t, f, func(s *Settings) {
		s.Exclude = []plan.Rule{{Name: "bots", Authors: []string{"renovate"}}}
	})
	m.inboxTab().showExcluded = true
	f.notifs[1].Updated = now // acme/api#9 is merged
	f.notifs = append(f.notifs, model.Notification{ThreadID: "n3", Key: key("acme", "api", 30), Title: "Update dependency",
		Reason: "review_requested", Unread: true, Updated: now})
	if n := rereadInbox(m); n != 0 {
		t.Errorf("the excluded rang %d times", n)
	}
}
