package tui

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
)

// spend takes ks out of the selection, as an action confirmed on them
// does, and keeps those that were selected for u to bring back. What was
// not selected, as a draft from Claude on another pull request, is left
// out, and the rest of the selection stays.
func (m *Model) spend(ks []model.PRKey) {
	var was []model.PRKey
	for _, k := range ks {
		if m.isSelected(k) {
			was = append(was, k)
			delete(m.selected, k)
		}
	}
	if len(was) > 0 {
		m.lastSelected = was
	}
}

// clearSelection takes the whole selection away; u brings it back.
func (m *Model) clearSelection() {
	m.lastSelected = make([]model.PRKey, 0, len(m.selected))
	for k := range m.selected {
		m.lastSelected = append(m.lastSelected, k)
	}
	clear(m.selected)
}

// restoreSelection brings the last selection back, without what has left
// the list or was put away since, so nothing out of sight comes back.
func (m *Model) restoreSelection() {
	n := 0
	for _, k := range m.lastSelected {
		if m.onTheList(k) {
			m.selected[k] = byHand
			n++
		}
	}
	m.lastSelected = nil
	if n == 0 {
		m.say("no selection to bring back")
		return
	}
	m.say(fmt.Sprintf("selected again: %d pull requests", n))
}

// onTheList reports whether a tab still has k and has not put it away.
func (m *Model) onTheList(k model.PRKey) bool {
	for _, t := range m.tabs {
		if t.hidden[k] {
			continue
		}
		if slices.ContainsFunc(m.allOf(t), func(p model.PR) bool { return p.Key == k }) {
			return true
		}
	}
	return false
}

// retry is an action that stopped on some of its pull requests, left to
// try again. It is kept apart from the selection: it acts only when asked,
// and on those alone (ADR-094).
type retry struct {
	// open opens the confirm popup of the action again; nil for done.
	open func() tea.Cmd
	// notes are the notifications that could not be marked done.
	notes []model.Notification
}

// retryBatch keeps what a batch failed on or stopped on, and reports
// whether there is any. What was skipped is not kept: a skip is a decision,
// not a failure, and the popup would take it again.
func (m *Model) retryBatch(bs *batchState) bool {
	var left []model.PRKey
	for _, k := range bs.order {
		switch bs.done[k].result {
		case runner.Failed, runner.Stopped:
			left = append(left, k)
		}
	}
	if len(left) == 0 {
		return false
	}
	m.retry = &retry{open: func() tea.Cmd {
		cmd := m.startBulkFor(bs.kind, bs.review, m.knownPRs(left))
		if m.bulk == nil {
			return cmd
		}
		m.bulk.retry = true
		if bs.kind == runner.Close {
			m.bulk.close = m.newCloseForm(bs.comment)
			m.bulk.close.del = bs.deleteBranch
		}
		return cmd
	}}
	return true
}

// retryDone keeps a notification that could not be marked done, with
// the others that failed the same way.
func (m *Model) retryDone(n model.Notification) {
	if m.retry == nil || m.retry.open != nil {
		m.retry = &retry{}
	}
	m.retry.notes = append(m.retry.notes, n)
}

// tryAgain runs the retry key: the popup of what stopped, which reads each
// pull request again, or done on what failed, which has no popup.
func (m *Model) tryAgain() tea.Cmd {
	r := m.retry
	switch {
	case r == nil:
		m.say("nothing to try again")
		return nil
	case r.open != nil:
		cmd := r.open()
		if m.bulk == nil {
			m.retry = nil
			m.say("nothing to try again: they have left the list")
		}
		return cmd
	}
	m.retry = nil
	cmds := make([]tea.Cmd, len(r.notes))
	for i, n := range r.notes {
		m.inboxTab().hidden[n.Key] = true
		delete(m.unread, n.Key)
		cmds[i] = m.markDone(n)
	}
	return tea.Batch(append(cmds, m.ensure())...)
}

// markDone marks n done, as the user asked.
func (m *Model) markDone(n model.Notification) tea.Cmd {
	return func() tea.Msg { return doneMsg{n, m.src.MarkDone(m.ctx, n), true} }
}

// knownPRs are the pull requests of ks the list still knows.
func (m *Model) knownPRs(ks []model.PRKey) []model.PR {
	var prs []model.PR
	for _, k := range ks {
		if p, ok := m.knownPR(k); ok {
			prs = append(prs, p)
		}
	}
	return prs
}
