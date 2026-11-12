package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// actionMenu is the popup of enter on a pull request: what can be done to
// it, or to the selection when there is one, each with its key and what it
// would do now. An action that cannot run is shown with the reason.
type actionMenu struct {
	sel int
	// top is the first line shown, kept by renderMenu so sel is in sight.
	top int
	pointer
}

// menuActions are the actions the menu can offer, in their default order:
// what reads a pull request, then what answers it, then what finishes it.
var menuActions = []keymap.Action{"ask_claude", "browse", "review", "comment", "approve", "merge", "stop", "draft", "close", "hide"}

// MenuActions are the actions the menu can offer, in their default order.
func MenuActions() []string {
	out := make([]string, len(menuActions))
	for i, a := range menuActions {
		out[i] = string(a)
	}
	return out
}

// menuOrder is the actions the menu offers, in their order: the config
// file's, or all of them in the default order.
func (m *Model) menuOrder() []keymap.Action {
	if m.set.Menu == nil {
		return menuActions
	}
	out := make([]keymap.Action, len(m.set.Menu))
	for i, a := range m.set.Menu {
		out[i] = keymap.Action(a)
	}
	return out
}

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
	for _, a := range m.menuOrder() {
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
		return m.mergeHow(d), false
	case "draft":
		switch kind, _, why := m.draftOf([]model.PR{p}); {
		case why != "":
			return why, true
		case kind == runner.Ready:
			return "mark it ready, asking its reviewers to review it", false
		}
		return "turn it back into a draft", false
	case "close":
		if p.State != model.StateOpen {
			return stateWords(p.State), true
		}
		return "close it, as the next popup says", false
	case "comment":
		return "comment on its conversation", false
	case "stop":
		return m.aboutStop([]model.PR{p})
	case "review":
		if m.own(p) {
			return "comment in a review of your own pull request", false
		}
		return "comment, approve or request changes in a review", false
	case "ask_claude":
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
	case "stop":
		return m.aboutStop(prs)
	case "draft":
		kind, n, why := m.draftOf(prs)
		switch {
		case why != "":
			return why, true
		case kind == runner.Ready:
			return of("mark ready", n), false
		}
		return of("convert to draft", n), false
	case "close":
		n := count(func(p model.PR) bool { return p.State == model.StateOpen })
		if n == 0 {
			return "none of them is open", true
		}
		return of("close", n), false
	case "comment":
		return fmt.Sprintf("the same comment on each of %d", len(prs)), false
	case "review":
		return fmt.Sprintf("the same review of each of %d", len(prs)), false
	case "browse":
		return fmt.Sprintf("open %d on github.com", len(prs)), false
	case "hide":
		ks, _ := m.selectedHere()
		if len(ks) == 0 {
			return "none of them is shown here", true
		}
		what := fmt.Sprintf("hide %d from this tab", len(ks))
		if m.tab().inbox {
			what = fmt.Sprintf("mark %d done", len(ks))
		}
		if skip := len(prs) - len(ks); skip > 0 {
			what += fmt.Sprintf(", skip %d not shown here", skip)
		}
		return what, false
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

// draftOf decides which way the key of draft turns prs, as the list
// tells; the popup reads them again before it acts. Only a pull request of
// the viewer's or of a bot counts, and n is how many; another person's is
// skipped. When those that count are not all drafts or all ready, which
// way is not known, and why says so (ADR-113).
func (m *Model) draftOf(prs []model.PR) (kind runner.Kind, n int, why string) {
	open, drafts := 0, 0
	for _, p := range prs {
		if p.State != model.StateOpen {
			continue
		}
		open++
		if !plan.OwnOrBot(p, m.viewer) {
			continue
		}
		n++
		if p.Draft {
			drafts++
		}
	}
	switch {
	case open == 0 && len(prs) == 1:
		return 0, 0, stateWords(prs[0].State)
	case open == 0:
		return 0, 0, "none of them is open"
	case n == 0 && len(prs) == 1:
		return 0, 0, "someone else's pull request"
	case n == 0:
		return 0, 0, "none of them is yours or a bot's"
	case drafts == n:
		return runner.Ready, n, ""
	case drafts == 0:
		return runner.ToDraft, n, ""
	}
	return 0, 0, "some are drafts and some are not"
}

// startDraft opens the popup that marks the drafts ready, or turns the
// pull requests back into drafts, as draftOf decides.
func (m *Model) startDraft() tea.Cmd {
	prs := m.menuTargets()
	if len(prs) == 0 {
		return nil
	}
	kind, _, why := m.draftOf(prs)
	if why != "" {
		m.refuse(m.keys.Help("draft") + ": " + why)
		return nil
	}
	return m.startBulkFor(kind, runner.ReviewText{}, prs)
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

// menuSpec is the menu as the shared popup layer reads it: a list of the
// actions, each also run by its own key (ADR-125). With nothing to act
// on, the menu closes.
func (m *Model) menuSpec() popupSpec {
	closeMenu := func() { m.menu = nil }
	its := m.menuItems()
	if len(its) == 0 {
		m.menu = nil
		return popupSpec{close: closeMenu}
	}
	v := m.menu
	v.sel = min(v.sel, len(its)-1)
	own := make([]ownKey, len(its))
	for i, it := range its {
		own[i] = ownKey{action: it.action, run: func() tea.Cmd {
			v.sel = i
			return m.runMenu(it)
		}}
	}
	return popupSpec{
		list: &popupList{sel: &v.sel, n: len(its), ptr: &v.pointer,
			choose: func() tea.Cmd { return m.runMenu(its[v.sel]) },
			// Marking done in the inbox writes to GitHub with no popup
			// to confirm it, so a click does not (ADR-012, ADR-053).
			click: func() tea.Cmd {
				if it := its[v.sel]; it.action == "hide" && m.tab().inbox && !it.off {
					m.refuse(m.keys.Help(it.action) + ": press " + m.firstKey(it.action) + ", as a click does not mark done on GitHub")
					return nil
				}
				return m.runMenu(its[v.sel])
			}},
		own:    own,
		opener: "menu",
		close:  closeMenu,
	}
}

// runMenu runs an item as its key on the list would. One that cannot run
// leaves the menu open and says why.
func (m *Model) runMenu(it menuItem) tea.Cmd {
	if it.off {
		m.refuse(m.keys.Help(it.action) + ": " + it.about)
		return nil
	}
	m.menu = nil
	switch it.action {
	case "approve":
		return m.startApprove()
	case "merge":
		return m.startBulk(runner.Merge)
	case "stop":
		return m.stopWork()
	case "draft":
		return m.startDraft()
	case "close":
		return m.startClose()
	case "comment":
		return m.startComment()
	case "review":
		return m.startReview()
	}
	if len(m.selected) > 0 {
		switch it.action {
		case "browse":
			return m.browseAll(m.menuTargets())
		case "hide":
			return m.hideSelected()
		}
	}
	p, n, ok := m.currentPR()
	if !ok {
		return nil
	}
	switch it.action {
	case "ask_claude":
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
		lines = append(lines, keysLines(st.sub0, prs, inner, 3)...)
	} else if len(prs) > 0 {
		title = prs[0].Key.String()
		// The title may be cut: the popup's title names the pull request.
		lines = append(lines, st.sub0.Render(shownTitle(prs[0].Title)))
	}
	lines = append(lines, "")
	sel := 0
	if m.menu != nil {
		sel = min(m.menu.sel, len(its)-1)
	}
	var keep keepLines
	owner := noItems(len(lines))
	for i, it := range its {
		cursor := " "
		if i == sel {
			cursor = st.blue.Render("▎")
		}
		label, key, about := st.sub1, st.key, st.muted
		if it.off {
			label, key, about = st.dim, st.dim, st.dim
		}
		item := hang(cursor+" "+fit(label.Render(m.keys.Help(it.action)), 22)+fit(key.Render(m.menuKeyHint(it.action)), 7), about.Render(it.about), inner)
		if i == sel {
			for j := range item {
				item[j] = onBackground(item[j], st.sel, inner)
			}
			keep = keepLines{len(lines), len(item)}
		}
		lines = append(lines, item...)
		for range item {
			owner = append(owner, i)
		}
	}
	foot := []string{"", st.key.Render(m.firstKey("choose")) + " " + st.muted.Render("run") +
		st.dim.Render(" · ") + st.muted.Render("or the action's key") +
		st.dim.Render(" · ") + st.key.Render(keymap.Back) + " " + st.muted.Render("close")}
	top := 0
	if m.menu != nil {
		top = m.menu.top
	}
	out, top := scrollBox(st, title, lines, foot, w, m.popupHeight(), top, keep, "")
	if m.menu != nil {
		m.menu.top = top
		m.menu.items = boxItems(owner, top, len(out), len(foot))
	}
	return out
}

// menuKeyHint is an action's key in the menu; none for one without a
// key, such as close (ADR-066), which the menu is the way to.
func (m *Model) menuKeyHint(a keymap.Action) string {
	if ks := m.keys.Keys(a); len(ks) > 0 {
		return ks[0]
	}
	return ""
}
