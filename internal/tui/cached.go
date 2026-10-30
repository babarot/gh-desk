package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-triage/internal/listcache"
	"github.com/babarot/gh-triage/internal/logging"
	"github.com/babarot/gh-triage/internal/model"
)

// Lists keeps the lists last read, for the next launch to show at once.
// The app package passes the listcache package's Cache, opened for the
// viewer it read.
type Lists interface {
	List(query string) (listcache.List, bool)
	Inbox() (listcache.Inbox, bool)
	PutList(l listcache.List) error
	PutInbox(in listcache.Inbox) error
}

// loadCached shows the lists last read, as of when they were read; each
// tab is read again at once.
func (m *Model) loadCached() {
	if m.set.Lists == nil {
		return
	}
	n := 0
	for _, t := range m.tabs {
		if t.inbox {
			in, ok := m.set.Lists.Inbox()
			if !ok {
				continue
			}
			prs := map[model.PRKey]model.PR{}
			for _, p := range in.PRs {
				prs[p.Key] = safePR(p)
			}
			m.setNotifs(t, in.Notifs)
			t.inboxPRs, t.loaded, t.asOf, t.readAt = prs, true, in.Read, in.Read
			n++
			continue
		}
		l, ok := m.set.Lists.List(t.query)
		if !ok {
			continue
		}
		t.prs, t.total, t.loaded, t.asOf, t.readAt = safePRs(l.PRs), l.Total, true, l.Read, l.Read
		m.toFirstPR(t)
		n++
	}
	m.log.Debug("lists from the cache", "tabs", n)
}

// saveTab keeps a tab's list once its reading is over. The rows kept only
// to show a result are left out: they are as they were while open, and the
// next launch would show a merged pull request as open.
func (m *Model) saveTab(i int) tea.Cmd {
	t := m.tabs[i]
	if m.set.Lists == nil || t.inbox || t.readAt.IsZero() || !t.asOf.IsZero() {
		return nil
	}
	l := listcache.List{Query: t.query, Total: t.total, Read: t.readAt}
	for _, p := range t.prs {
		if !t.kept[p.Key] {
			l.PRs = append(l.PRs, p)
		}
	}
	return m.put(func() error { return m.set.Lists.PutList(l) })
}

func (m *Model) saveInbox(t *tab) tea.Cmd {
	if m.set.Lists == nil {
		return nil
	}
	in := listcache.Inbox{Read: t.readAt, Notifs: t.notifs}
	for _, p := range t.inboxPRs {
		in.PRs = append(in.PRs, p)
	}
	return m.put(func() error { return m.set.Lists.PutInbox(in) })
}

// put writes the cache away from the screen; a failure only costs the next
// launch its head start.
func (m *Model) put(write func() error) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		if err := write(); err != nil {
			m.log.Warn("could not keep the lists", "err", err.Error())
			return nil
		}
		m.log.Debug("lists kept", logging.KeyDurationMS, time.Since(start).Milliseconds())
		return nil
	}
}
