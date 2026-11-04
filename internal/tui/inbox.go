package tui

import (
	"maps"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

// firstInboxPRs is how many pull requests of new notifications are read
// ahead of the rest of the inbox, in one query: 5 took 1.6s where every
// pull request of a full inbox, 25 to a query, took 4s and more.
const firstInboxPRs = 10

// inboxFirstMsg is what the read ahead of the rest of the inbox read.
type inboxFirstMsg struct {
	gen int
	prs map[model.PRKey]model.PR
	err error
}

// firstInbox picks the pull requests to read ahead of the rest when the
// inbox t is read again: of notifications that are new, that have new
// activity, or whose pull request is not read yet, in the order GitHub
// gives them, most recently updated first. It is called before ns takes
// the place of t's notifications.
func firstInbox(t *tab, ns []model.Notification) []model.PRKey {
	prev := map[model.PRKey]time.Time{}
	for _, n := range t.notifs {
		prev[n.Key] = n.Updated
	}
	var out []model.PRKey
	seen := map[model.PRKey]bool{}
	for _, n := range ns {
		if len(out) == firstInboxPRs {
			break
		}
		if seen[n.Key] {
			continue
		}
		seen[n.Key] = true
		was, ok := prev[n.Key]
		_, read := t.inboxPRs[n.Key]
		if !ok || !was.Equal(n.Updated) || !read {
			out = append(out, n.Key)
		}
	}
	return out
}

// readInboxFirst reads the pull requests keys of the inbox's reading gen
// ahead of the rest.
func (m *Model) readInboxFirst(gen int, keys []model.PRKey) tea.Cmd {
	if len(keys) == 0 {
		return nil
	}
	return func() tea.Msg {
		start := time.Now()
		prs, err := m.src.PullRequests(m.ctx, keys)
		m.log.Debug("inbox read ahead", logging.KeyOp, "inbox", "prs", len(keys), logging.KeyDurationMS, time.Since(start).Milliseconds())
		return inboxFirstMsg{gen, prs, err}
	}
}

// inboxFirstRead puts the pull requests read ahead in the inbox's rows. The
// read of every pull request that follows replaces them as before, so a
// read ahead that comes after it, or after the inbox is read again, is
// dropped, and one that failed is only logged.
func (m *Model) inboxFirstRead(msg inboxFirstMsg) tea.Cmd {
	t := m.inboxTab()
	if msg.gen != t.gen || !t.loading {
		return nil
	}
	if msg.err != nil {
		m.log.Warn("inbox read ahead", logging.KeyOp, "inbox", "err", msg.err.Error())
	}
	for k, p := range msg.prs {
		t.inboxPRs[k] = safePR(p)
	}
	return tea.Batch(m.ensure(), m.ensureChecks(slices.Collect(maps.Values(t.inboxPRs))))
}
