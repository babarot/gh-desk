package tui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
)

// The scopes Settings.Scope may be: what every tab is narrowed by, one
// value at a time.
const (
	ScopeOwner  = "owner"
	ScopeRepo   = "repo"
	ScopeAuthor = "author"
)

// allScope is how the header and the scope chooser name no value chosen.
const allScope = "all"

// scoped reports whether the tabs are narrowed by a scope: one is set, and
// a value of it is chosen.
func (m *Model) scoped() bool { return m.set.Scope != "" && m.scope != "" }

// scopeOf is the value of the scope set that p has: its owner, its
// owner/repo, or its author without app/. "" when it is not known, as the
// author of a notification's pull request not read yet.
func (m *Model) scopeOf(p model.PR) string {
	switch m.set.Scope {
	case ScopeOwner:
		return p.Key.Owner
	case ScopeRepo:
		return p.Key.Repository()
	case ScopeAuthor:
		return strings.TrimPrefix(p.Author, "app/")
	}
	return ""
}

// inScope reports whether p is in the value of the scope chosen; every pull
// request is while none is.
func (m *Model) inScope(p model.PR) bool {
	return !m.scoped() || m.scopeOf(p) == m.scope
}

// scopeValues are the values of the scope set that the tabs hold, by name:
// those of every pull request read, but one that every tab leaves out, by
// an exclusion rule or, in the inbox unless it keeps them, as merged or
// closed. A value whose
// rows a tab's excluded section leaves out is kept, so that it does not
// come and go as its pull requests are judged.
func (m *Model) scopeValues() []string {
	seen := map[string]bool{}
	now := m.set.Now()
	add := func(p model.PR, n *model.Notification) {
		v := m.scopeOf(p)
		if v == "" || seen[v] {
			return
		}
		if n != nil && settled(p) && !m.inboxTab().keepSettled {
			return
		}
		f := m.ruleFacts(p, now)
		if n != nil {
			f.Reason = n.Reason
		}
		for _, r := range m.set.Exclude {
			if r.Judge(p, m.viewer, f) == plan.Yes {
				return
			}
		}
		seen[v] = true
	}
	for _, t := range m.tabs {
		if t.inbox {
			for i := range t.notifs {
				add(m.notifPR(&t.notifs[i]), &t.notifs[i])
			}
			continue
		}
		for _, p := range t.prs {
			add(p, nil)
		}
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return out
}

// scopeChoices are what the scope can be set to, in the order < and > go
// through them: none first, then the values by name, the one chosen among
// them even when no tab holds it any more.
func (m *Model) scopeChoices() []string {
	vs := m.scopeValues()
	if m.scope != "" && !slices.Contains(vs, m.scope) {
		vs = append(vs, m.scope)
		slices.SortFunc(vs, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	}
	return append([]string{""}, vs...)
}

// stepScope chooses the value by steps from the one chosen, going round.
func (m *Model) stepScope(by int) tea.Cmd {
	if m.set.Scope == "" {
		m.refuse("no scope is set: list.scope narrows every tab by an owner, a repo or an author")
		return nil
	}
	cs := m.scopeChoices()
	i := max(slices.Index(cs, m.scope), 0)
	return m.setScope(cs[(i+by+len(cs))%len(cs)])
}

// setScope chooses v, "" for none, and keeps it for the next launch. Each
// tab's cursor stays on its pull request when it is in v, and goes to the
// first one otherwise, as when a filter is typed: what the user does to
// leave a row out lets it go (ADR-115).
func (m *Model) setScope(v string) tea.Cmd {
	m.scope, m.scopeChecked = v, true
	for _, t := range m.tabs {
		t.gone = false
		if t.at != nil {
			if i := slices.IndexFunc(m.rows(t), isPR(t.at.pr.Key)); i >= 0 {
				t.sel = i
				continue
			}
		}
		t.sel, t.offset, t.at, t.seen = 0, 0, nil, false
		m.toFirstPR(t)
	}
	m.log.Debug("scope chosen", logging.KeyOp, "scope", "all", v == "")
	return tea.Batch(m.ensure(), m.saveScope())
}

// saveScope keeps the value chosen for the next launch, away from the
// screen; a failure only costs the next launch the value.
func (m *Model) saveScope() tea.Cmd {
	if m.set.Layout == nil {
		return nil
	}
	kind, v, lay, log := m.set.Scope, m.scope, m.set.Layout, m.log
	return func() tea.Msg {
		if err := lay.SaveScope(kind, v); err != nil {
			log.Warn("could not keep the scope", "err", err.Error())
		}
		return nil
	}
}

// checkScope lets go of the value kept from the last launch, once every tab
// is read and none holds it: until then a tab not read may hold it. A tab
// whose reading failed may too, and so may, for an author, a notification
// whose pull request is not read yet, so it waits for them. It is let go
// for this launch only: the file keeps it, for a launch whose tabs hold it.
func (m *Model) checkScope() {
	if m.scopeChecked || !m.scoped() {
		return
	}
	for _, t := range m.tabs {
		if !t.loaded || t.loading || t.err != nil || !t.asOf.IsZero() {
			return
		}
		if t.inbox && m.set.Scope == ScopeAuthor {
			for _, n := range t.notifs {
				if _, ok := m.knownPR(n.Key); !ok {
					return
				}
			}
		}
	}
	m.scopeChecked = true
	if !slices.Contains(m.scopeValues(), m.scope) {
		m.log.Debug("scope kept from the last launch let go: no tab holds it", logging.KeyOp, "scope")
		m.scope = ""
	}
}
