package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/plan"
	"github.com/babarot/gh-triage/internal/runner"
)

// actionMenu is the popup of enter on a pull request: what can be done to
// it, or to the selection when there is one, each with its key and what it
// would do now. An action that cannot run is shown with the reason.
type actionMenu struct {
	sel int
}

// menuActions are the actions the menu offers, in its order.
var menuActions = []keymap.Action{"approve", "merge", "ready_for_review", "comment", "review", "pr_claude", "browse", "hide"}

// menuItem is one line of the menu: what the action would do, or why it
// cannot run.
type menuItem struct {
	action keymap.Action
	about  string
	off    bool
}

func (m *Model) openMenu() tea.Cmd {
	m.menu = &actionMenu{}
	return nil
}

// menuTargets are the pull requests the menu acts on: the selection, or the
// one under the cursor.
func (m *Model) menuTargets() []model.PR {
	if len(m.selected) == 0 {
		if p, _, ok := m.currentPR(); ok {
			return []model.PR{p}
		}
		return nil
	}
	var prs []model.PR
	for _, t := range m.tabs {
		for _, p := range m.allOf(t) {
			if m.isSelected(p.Key) && !containsKey(prs, p.Key) {
				prs = append(prs, p)
			}
		}
	}
	return prs
}

func (m *Model) menuItems() []menuItem {
	prs := m.menuTargets()
	if len(prs) == 0 {
		return nil
	}
	var out []menuItem
	for _, a := range menuActions {
		it := menuItem{action: a}
		if len(m.selected) > 0 {
			it.about, it.off = m.aboutMany(a, prs)
		} else {
			it.about, it.off = m.aboutOne(a, prs[0])
		}
		out = append(out, it)
	}
	return out
}

// aboutOne says what action would do to p, or why it cannot.
func (m *Model) aboutOne(a keymap.Action, p model.PR) (string, bool) {
	mi, known := m.cache.info[p.Key]
	switch a {
	case "approve":
		if why, ok := m.cannotApprove(p); ok {
			return why, true
		}
		return "send an approving review", false
	case "merge":
		if p.State != model.StateOpen {
			return stateWords(p.State), true
		}
		if !known {
			return "merge it, as the next popup decides", false
		}
		d := plan.ForMerge(mi, p, m.viewer, m.set.Plan)
		if d.Step == plan.StepSkip {
			return d.Reason, true
		}
		how := d.Reason
		if d.Method != "" {
			how += " (" + methodWord(d.Method) + ")"
		}
		if d.Approve {
			how = "approve, then " + how
		}
		return how, false
	case "ready_for_review":
		return m.aboutReady(p)
	case "comment":
		return "comment on its conversation", false
	case "review":
		if m.own(p) {
			return "comment in a review of your own pull request", false
		}
		return "comment, approve or request changes in a review", false
	case "pr_claude":
		if m.claude == nil {
			return "claude is not available", true
		}
		return "talk with claude about its diff", false
	case "browse":
		return "open it on github.com", false
	case "hide":
		if m.tab().inbox {
			return "mark it done on GitHub", false
		}
		return "hide it from this tab", false
	}
	return "", true
}

// aboutMany says what action would do to the selection, or why it cannot.
// Approve and merge count what the next popup would act on; the popup
// names the rest and why.
func (m *Model) aboutMany(a keymap.Action, prs []model.PR) (string, bool) {
	count := func(can func(model.PR) bool) int {
		n := 0
		for _, p := range prs {
			if can(p) {
				n++
			}
		}
		return n
	}
	of := func(verb string, n int) string {
		if n == len(prs) {
			return fmt.Sprintf("%s %d", verb, n)
		}
		return fmt.Sprintf("%s %d of %d, skip %d", verb, n, len(prs), len(prs)-n)
	}
	switch a {
	case "approve":
		n := count(func(p model.PR) bool { _, no := m.cannotApprove(p); return !no })
		if n == 0 {
			return "none of them can be approved", true
		}
		return of("approve", n), false
	case "merge":
		n := count(func(p model.PR) bool { _, off := m.aboutOne("merge", p); return !off })
		if n == 0 {
			return "none of them can be merged", true
		}
		return of("merge", n), false
	case "ready_for_review":
		n := count(func(p model.PR) bool { _, off := m.aboutReady(p); return !off })
		if n == 0 {
			return "none of them is a draft", true
		}
		return of("mark ready", n), false
	}
	return "one pull request at a time", true
}

// cannotApprove is why p cannot be approved, as far as what has been read
// tells. Failing checks do not stop it: the approve popup asks about them.
func (m *Model) cannotApprove(p model.PR) (string, bool) {
	switch {
	case p.State != model.StateOpen:
		return stateWords(p.State), true
	case m.own(p):
		return "your own pull request", true
	}
	if mi, ok := m.cache.info[p.Key]; ok {
		if d := plan.ForApprove(mi, p, m.viewer, true, m.set.Plan); d.Step == plan.StepSkip {
			return d.Reason, true
		}
	}
	return "", false
}

// aboutReady says what marking p ready for review would do, or why it
// cannot, as the list tells; the popup reads it again before it acts.
func (m *Model) aboutReady(p model.PR) (string, bool) {
	switch {
	case p.State != model.StateOpen:
		return stateWords(p.State), true
	case !p.Draft:
		return "not a draft", true
	}
	return "ask its reviewers to review it", false
}

func stateWords(s model.PRState) string {
	switch s {
	case model.StateMerged:
		return "merged"
	case model.StateClosed:
		return "closed"
	}
	return "not open"
}

func (m *Model) menuKey(k string) tea.Cmd {
	its := m.menuItems()
	if len(its) == 0 {
		m.menu = nil
		return nil
	}
	v := m.menu
	v.sel = min(v.sel, len(its)-1)
	is := func(a keymap.Action) bool { return m.keys.Matches(a, k) }
	switch {
	case k == keymap.Back:
		m.menu = nil
	case is("down"):
		v.sel = min(v.sel+1, len(its)-1)
	case is("up"):
		v.sel = max(v.sel-1, 0)
	case is("top"):
		v.sel = 0
	case is("bottom"):
		v.sel = len(its) - 1
	case is("choose"):
		return m.runMenu(its[v.sel])
	default:
		for i, it := range its {
			if m.keys.Matches(it.action, k) {
				v.sel = i
				return m.runMenu(it)
			}
		}
	}
	return nil
}

// runMenu runs an item as its key on the list would. One that cannot run
// leaves the menu open and says why.
func (m *Model) runMenu(it menuItem) tea.Cmd {
	if it.off {
		m.setStatus(m.keys.Help(it.action)+": "+it.about, true)
		return nil
	}
	m.menu = nil
	switch it.action {
	case "approve":
		return m.startBulk(runner.Approve)
	case "merge":
		return m.startBulk(runner.Merge)
	case "ready_for_review":
		return m.startBulk(runner.Ready)
	}
	p, n, ok := m.currentPR()
	if !ok {
		return nil
	}
	switch it.action {
	case "comment":
		return m.startComment(p)
	case "review":
		return m.startCompose(p, model.EventComment)
	case "pr_claude":
		return m.openChat(p)
	case "browse":
		return m.browse(p)
	case "hide":
		return m.hide(p, n)
	}
	return nil
}

func (m *Model) renderMenu() []string {
	st := m.st
	w := min(max(m.width-8, 40), 84)
	inner := w - 4
	prs := m.menuTargets()
	its := m.menuItems()
	var title string
	var lines []string
	if len(m.selected) > 0 {
		title = fmt.Sprintf("%d selected", len(prs))
		var names []string
		for _, p := range prs {
			names = append(names, p.Key.String())
		}
		lines = append(lines, st.sub0.Render(strings.Join(names, "  ")))
	} else if len(prs) > 0 {
		title = prs[0].Key.String()
		lines = append(lines, st.sub0.Render(prs[0].Title))
	}
	lines = append(lines, "")
	sel := 0
	if m.menu != nil {
		sel = min(m.menu.sel, len(its)-1)
	}
	for i, it := range its {
		cursor := " "
		if i == sel {
			cursor = st.blue.Render("▎")
		}
		label, key, about := st.sub1, st.key, st.muted
		if it.off {
			label, key, about = st.dim, st.dim, st.dim
		}
		line := cursor + " " + fit(label.Render(m.keys.Help(it.action)), 22) + fit(key.Render(m.firstKey(it.action)), 7) + about.Render(it.about)
		if i == sel {
			line = onBackground(line, st.sel, inner)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", st.key.Render(m.firstKey("choose"))+" "+st.muted.Render("run")+
		st.dim.Render(" · ")+st.muted.Render("or the action's key")+
		st.dim.Render(" · ")+st.key.Render(keymap.Back)+" "+st.muted.Render("close"))
	return box(st, title, lines, w)
}
