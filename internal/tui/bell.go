package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

// bell is the terminal's bell (BEL): the terminal rings it, or flashes,
// as it is set to.
const bell = "\a"

// newlyUnread are the pull requests whose notification is unread in now
// and was not in was, the inbox's previous reading: new, read before, or
// updated since. One put away here while its mark of done is on its way
// is still unread in was, so it is not new.
func newlyUnread(was, now []model.Notification) []model.PRKey {
	before := map[model.PRKey]model.Notification{}
	for _, n := range was {
		if n.Unread {
			before[n.Key] = n
		}
	}
	var keys []model.PRKey
	for _, n := range now {
		if !n.Unread || slices.Contains(keys, n.Key) {
			continue
		}
		if b, ok := before[n.Key]; ok && !n.Updated.After(b.Updated) {
			continue
		}
		keys = append(keys, n.Key)
	}
	return keys
}

// noteNewUnread keeps, for the bell, the pull requests newly unread in
// the inbox t's reading ns, when list.bell is on. The first reading from
// GitHub in a run only sets what later ones are compared with: what came
// while gh-desk was not running does not ring it.
func (m *Model) noteNewUnread(t *tab, ns []model.Notification) {
	if !m.set.Bell {
		return
	}
	if t.readLive {
		for _, k := range newlyUnread(t.notifs, ns) {
			if !slices.Contains(t.newUnread, k) {
				t.newUnread = append(t.newUnread, k)
			}
		}
	}
	t.readLive = true
}

// ringBell rings the bell once the inbox t's pull requests are read, when
// one of those newly unread is not excluded, and forgets them. Those
// excluded stay quiet even while the excluded are shown.
func (m *Model) ringBell(t *tab) tea.Cmd {
	keys := t.newUnread
	t.newUnread = nil
	n, now := 0, m.set.Now()
	for _, k := range keys {
		nt, ok := m.unread[k]
		if !ok || t.hidden[k] {
			continue
		}
		if !m.exclusionOf(t, m.notifPR(&nt), &nt, now).excluded() {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	m.log.Debug("bell", logging.KeyOp, "bell", "count", n)
	return tea.Raw(bell)
}
