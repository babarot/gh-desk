package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/plan"
)

// pick is the popup of S: the rules, how many pull requests of the tab each
// matches, and those the rule under the cursor matches. Choosing one
// replaces the selection with them; approving and merging stay with A and M.
type pick struct {
	sel     int
	loading bool
}

type bodiesMsg struct {
	got map[model.PRKey]string
	err error
}

func (m *Model) rules() []plan.Rule {
	return append([]plan.Rule{plan.ApprovedByMe}, m.set.Rules...)
}

// openPick opens the popup, reading the bodies a rule needs first.
func (m *Model) openPick() tea.Cmd {
	m.picker = &pick{}
	needs := false
	for _, r := range m.rules() {
		needs = needs || r.NeedsBody()
	}
	if !needs {
		return nil
	}
	var ids []string
	keys := map[string]model.PRKey{}
	for _, p := range m.allOf(m.tab()) {
		if _, ok := m.body(p.Key); ok || !p.AuthorIsBot || p.ID == "" {
			continue
		}
		ids = append(ids, p.ID)
		keys[p.ID] = p.Key
	}
	if len(ids) == 0 {
		return nil
	}
	m.picker.loading = true
	return func() tea.Msg {
		got, err := m.src.Bodies(m.ctx, ids)
		out := map[model.PRKey]string{}
		for id, b := range got {
			out[keys[id]] = b
		}
		return bodiesMsg{out, err}
	}
}

func (m *Model) bodies(msg bodiesMsg) {
	for k, b := range msg.got {
		m.cache.bodies[k] = safe(b)
	}
	if m.picker != nil {
		m.picker.loading = false
	}
	if msg.err != nil {
		m.setStatus("could not read every description: "+msg.err.Error(), true)
	}
}

// body is a pull request's description, if it has been read.
func (m *Model) body(k model.PRKey) (string, bool) {
	if b, ok := m.cache.bodies[k]; ok {
		return b, true
	}
	if d, ok := m.cache.details[k]; ok && !d.loading && d.err == nil {
		return d.val.Body, true
	}
	return "", false
}

// matches are the pull requests of the tab a rule picks.
func (m *Model) ruleMatches(r plan.Rule) []model.PR {
	t := m.tab()
	var out []model.PR
	for _, p := range m.allOf(t) {
		if t.hidden[p.Key] {
			continue
		}
		body, known := m.body(p.Key)
		f := plan.Facts{Body: body, BodyKnown: known}
		if mi, ok := m.cache.info[p.Key]; ok {
			f.AwaitingApproval = plan.AwaitingApproval(mi.Checks, m.set.Plan.ApprovalChecks)
		}
		if r.Match(p, m.viewer, f) {
			out = append(out, p)
		}
	}
	return out
}

func (m *Model) pickKey(k string) tea.Cmd {
	rs := m.rules()
	v := m.picker
	is := func(a keymap.Action) bool { return m.keys.Matches(a, k) }
	switch {
	case k == keymap.Back, is("select_rule"):
		m.picker = nil
	case is("down"):
		v.sel = min(v.sel+1, len(rs)-1)
	case is("up"):
		v.sel = max(v.sel-1, 0)
	case is("choose") && !v.loading:
		m.picker = nil
		return m.applyRule(rs[v.sel])
	}
	return nil
}

// applyRule replaces the selection with what the rule matches, unfolds
// their groups, and puts the cursor on the first.
func (m *Model) applyRule(r plan.Rule) tea.Cmd {
	hits := m.ruleMatches(r)
	clear(m.selected)
	t := m.tab()
	for _, p := range hits {
		m.selected[p.Key] = true
		if t.grouped {
			t.folded[plan.GroupOf(p, m.viewer)] = false
		}
	}
	if len(hits) == 0 {
		m.setStatus(r.Name+": nothing matches in this tab", false)
		return nil
	}
	for i, row := range m.rows(t) {
		if row.kind == rowPR && m.selected[row.pr.Key] {
			t.sel = i
			break
		}
	}
	m.setStatus(fmt.Sprintf("%s: selected %d · %s to merge, %s to approve", r.Name, len(hits), m.firstKey("merge"), m.firstKey("approve")), false)
	return m.ensure()
}

func (m *Model) renderPick() []string {
	st, v := m.st, m.picker
	w := min(max(m.width-8, 40), 110)
	rs := m.rules()
	var lines []string
	for i, r := range rs {
		cursor := " "
		if i == v.sel {
			cursor = st.blue.Render("▎")
		}
		n := len(m.ruleMatches(r))
		count := st.dim.Render(fmt.Sprintf("%d", n))
		if n > 0 {
			count = st.mauve.Render(fmt.Sprintf("%d", n))
		}
		line := cursor + " " + fit(st.sub1.Render(r.Name), 22) + fit(count+st.dim.Render(" PRs"), 9) + st.muted.Render(ruleWords(r))
		if i == v.sel {
			line = onBackground(line, st.sel, w-4)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", st.group.Render("matches"))
	if v.loading {
		lines = append(lines, st.dim.Render("  reading descriptions…"))
	} else {
		hits := m.ruleMatches(rs[v.sel])
		if len(hits) == 0 {
			lines = append(lines, st.dim.Render("  nothing in this tab"))
		}
		for i, p := range hits {
			if i == 8 {
				lines = append(lines, st.dim.Render(fmt.Sprintf("  … %d more; the approve and merge popups list them all", len(hits)-8)))
				break
			}
			name := st.sub1.Render(p.Key.Repository()) + st.muted.Render(fmt.Sprintf("#%d", p.Key.Number))
			lines = append(lines, "  "+fit(name, 30)+" "+st.sub0.Render(p.Title))
		}
	}
	lines = append(lines, "", st.key.Render(m.firstKey("choose"))+" "+st.muted.Render("select these")+
		st.dim.Render(" · ")+st.key.Render(keymap.Back)+" "+st.muted.Render("cancel"))
	return box(st, "select by rule", lines, w)
}

// ruleWords says what a rule asks for.
func ruleWords(r plan.Rule) string {
	var parts []string
	if r.ApprovedByMe {
		parts = append(parts, "approved by you")
	}
	if r.Bot {
		parts = append(parts, "bots")
	}
	if r.Author != "" {
		parts = append(parts, r.Author)
	}
	if len(r.Packages) > 0 {
		noun := "packages"
		if len(r.Packages) == 1 {
			noun = "package"
		}
		parts = append(parts, fmt.Sprintf("%d %s", len(r.Packages), noun))
	}
	if r.Title != nil {
		parts = append(parts, "title /"+r.Title.String()+"/")
	}
	if r.MaxFiles > 0 {
		parts = append(parts, fmt.Sprintf("≤%d files", r.MaxFiles))
	}
	if r.ChecksPassing {
		parts = append(parts, "checks passing")
	}
	s := ""
	for i, p := range parts {
		if i > 0 {
			s += " · "
		}
		s += p
	}
	return s
}
