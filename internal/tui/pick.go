package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// pick is the popup of S: the rules, how many pull requests of the tab each
// matches, and those the rules chosen match together: the rules checked,
// or with none checked the one under the cursor. Choosing replaces the
// selection with them, adding keeps the selection and adds them, and
// filtering narrows the tab's rows to them; approving and merging stay
// with A and M.
type pick struct {
	sel     int
	loading bool
	// checked are the rules checked, by their index in rules().
	checked map[int]bool
	// top is the first line shown, kept by renderPick so sel is in sight.
	top int
	pointer
}

// ready reports whether the rules can be matched: the bodies they read are
// in, and the tab's heavy fields, which max_files and checks look at.
func (m *Model) pickReady() bool {
	return m.picker != nil && !m.picker.loading && m.tabDone(m.tab())
}

type bodiesMsg struct {
	got map[model.PRKey]string
	err error
	// tab is the index of the tab they were read for.
	tab int
}

func (m *Model) rules() []plan.Rule {
	return append([]plan.Rule{plan.ApprovedByMe}, m.set.Rules...)
}

// openPick opens the popup, reading the bodies a rule needs first.
func (m *Model) openPick() tea.Cmd {
	m.picker = &pick{checked: map[int]bool{}}
	if !needsBody(m.rules()) {
		return nil
	}
	cmd := m.fetchBodies(m.cur)
	m.picker.loading = cmd != nil
	return cmd
}

// readBodies reads, after a reading of tab i, the bodies its rule filter
// needs: a pull request whose body is not read does not match a rule on
// the packages, so it would stay out of the rows.
func (m *Model) readBodies(i int) tea.Cmd {
	if !needsBody(m.tabs[i].ruleFilter) {
		return nil
	}
	return m.fetchBodies(i)
}

func needsBody(rs []plan.Rule) bool {
	return slices.ContainsFunc(rs, plan.Rule.NeedsBody)
}

// fetchBodies reads the bodies of the bot pull requests of tab i not read
// yet; nil when there are none.
func (m *Model) fetchBodies(i int) tea.Cmd {
	t := m.tabs[i]
	var ids []string
	keys := map[string]model.PRKey{}
	for _, p := range m.allOf(t) {
		if _, ok := m.body(p.Key); ok || !p.AuthorIsBot || p.ID == "" {
			continue
		}
		ids = append(ids, p.ID)
		keys[p.ID] = p.Key
	}
	if len(ids) == 0 {
		return nil
	}
	t.readingBodies = true
	return func() tea.Msg {
		got, err := m.src.Bodies(m.ctx, ids)
		out := map[model.PRKey]string{}
		for id, b := range got {
			out[keys[id]] = b
		}
		return bodiesMsg{out, err, i}
	}
}

func (m *Model) bodies(msg bodiesMsg) {
	for k, b := range msg.got {
		m.cache.bodies[k] = safe(b)
	}
	m.tabs[msg.tab].readingBodies = false
	if m.picker != nil && msg.tab == m.cur {
		m.picker.loading = false
	}
	if msg.err != nil {
		m.log.Warn("descriptions read in part", "err", msg.err.Error())
		m.fail("could not read every description: " + msg.err.Error())
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

// ruleMatches are the pull requests of the tab every one of rs picks.
func (m *Model) ruleMatches(rs ...plan.Rule) []model.PR {
	t := m.tab()
	var out []model.PR
	now := m.set.Now()
	for _, p := range m.allOf(t) {
		if !t.hidden[p.Key] && matchesAll(rs, p, m.viewer, m.ruleFacts(p, now)) {
			out = append(out, p)
		}
	}
	return out
}

// ruleFacts are what a rule may need beyond p: its body, if read, whether
// only approval checks are pending, and now, the time its days count back
// from, taken once for every pull request matched at once.
func (m *Model) ruleFacts(p model.PR, now time.Time) plan.Facts {
	body, known := m.body(p.Key)
	f := plan.Facts{Body: body, BodyKnown: known, Now: now}
	if mi, ok := m.cache.info[p.Key]; ok {
		f.AwaitingApproval = plan.AwaitingApproval(mi.Checks, m.set.Plan.ApprovalChecks)
	}
	return f
}

// matchesAll reports whether every rule picks p. A rule on the size or the
// checks does not pick a pull request whose heavy fields are not read for
// its head: unread, they look like an empty pull request with no checks,
// which would pass.
func matchesAll(rs []plan.Rule, p model.PR, viewer string, f plan.Facts) bool {
	for _, r := range rs {
		if r.NeedsHeavy() && !p.HeavyRead() || !r.Match(p, viewer, f) {
			return false
		}
	}
	return true
}

// chosen are the rules choosing acts on: those checked, in the order shown,
// or with none checked the one under the cursor.
func (m *Model) chosen() []plan.Rule {
	rs, v := m.rules(), m.picker
	var out []plan.Rule
	for i, r := range rs {
		if v.checked[i] {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		out = []plan.Rule{rs[v.sel]}
	}
	return out
}

// ruleNames names rules put together, as the popup and the status line
// say them.
func ruleNames(rs []plan.Rule) string {
	names := make([]string, len(rs))
	for i, r := range rs {
		names[i] = r.Name
	}
	return strings.Join(names, " + ")
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
	case is("toggle_rule"):
		if v.checked == nil {
			v.checked = map[int]bool{}
		}
		if v.checked[v.sel] {
			delete(v.checked, v.sel)
		} else {
			v.checked[v.sel] = true
		}
	case is("choose") || is("add_to_selection"):
		return m.chooseRules(is("add_to_selection"))
	case is("filter_by_rule") && m.pickReady():
		chosen := m.chosen()
		m.picker = nil
		return m.filterByRules(chosen)
	}
	return nil
}

// chooseRules selects what the rules chosen match, once they can be
// matched, as applyRules does.
func (m *Model) chooseRules(add bool) tea.Cmd {
	if !m.pickReady() {
		return nil
	}
	chosen := m.chosen()
	m.picker = nil
	return m.applyRules(chosen, add)
}

// applyRules selects what the rules match together, replacing the
// selection or, with add, keeping it; it unfolds their groups and puts the
// cursor on the first of them.
func (m *Model) applyRules(rs []plan.Rule, add bool) tea.Cmd {
	hits := m.ruleMatches(rs...)
	name := ruleNames(rs)
	if !add {
		clear(m.selected)
	}
	added := 0
	t := m.tab()
	in := map[model.PRKey]bool{}
	for _, p := range hits {
		in[p.Key] = true
		if !m.isSelected(p.Key) {
			added++
			m.selected[p.Key] = byHand
		}
		if t.grouped {
			t.folded[plan.GroupOf(p, m.viewer)] = false
		}
	}
	if len(hits) == 0 {
		m.say(name + ": nothing matches in this tab")
		return nil
	}
	for i, row := range m.rows(t) {
		if row.kind == rowPR && in[row.pr.Key] {
			t.sel = i
			break
		}
	}
	what := fmt.Sprintf("selected %d", len(hits))
	if add {
		what = fmt.Sprintf("added %d, selected %d", added, len(m.selected))
	}
	m.say(fmt.Sprintf("%s: %s · %s to merge, %s to approve", name, what, m.firstKey("merge"), m.firstKey("approve")))
	return m.ensure()
}

// filterByRules narrows the tab's rows to what the rules match together,
// replacing the rules it was narrowed to, and unfolds their groups. The
// selection is kept.
func (m *Model) filterByRules(rs []plan.Rule) tea.Cmd {
	t := m.tab()
	t.ruleFilter = rs
	if t.grouped {
		for _, p := range m.ruleMatches(rs...) {
			t.folded[plan.GroupOf(p, m.viewer)] = false
		}
	}
	t.sel, t.offset = 0, 0
	return m.ensure()
}

func (m *Model) renderPick() []string {
	st, v := m.st, m.picker
	w := min(max(m.width-8, 40), 110)
	inner := w - 4
	rs := m.rules()
	var lines []string
	var keep keepLines
	var owner []int
	for i, r := range rs {
		cursor := " "
		if i == v.sel {
			cursor = st.blue.Render("▎")
		}
		mark := " "
		if v.checked[i] {
			mark = st.mauve.Render("✓")
		}
		n := len(m.ruleMatches(r))
		count := st.dim.Render(fmt.Sprintf("%d", n))
		if n > 0 {
			count = st.mauve.Render(fmt.Sprintf("%d", n))
		}
		rule := hang(cursor+" "+mark+" "+fit(st.sub1.Render(r.Name), 20)+fit(count+st.dim.Render(" PRs"), 9), st.muted.Render(ruleWords(r)), inner)
		if i == v.sel {
			for j := range rule {
				rule[j] = onBackground(rule[j], st.sel, inner)
			}
			keep = keepLines{len(lines), len(rule)}
		}
		lines = append(lines, rule...)
		for range rule {
			owner = append(owner, i)
		}
	}
	chosen := m.chosen()
	head := st.group.Render("matches")
	if len(v.checked) > 0 && m.pickReady() {
		head += st.dim.Render(" · ") + st.sub1.Render(ruleNames(chosen)) + st.dim.Render(fmt.Sprintf(" · %d PRs", len(m.ruleMatches(chosen...))))
	}
	lines = append(lines, "", head)
	if !m.pickReady() {
		what := "descriptions"
		if !m.tabDone(m.tab()) {
			what = "checks and sizes"
		}
		lines = append(lines, "  "+m.working(st.dim, "reading "+what+"…"))
	} else {
		hits := m.ruleMatches(chosen...)
		if len(hits) == 0 {
			lines = append(lines, st.dim.Render("  nothing in this tab"))
		}
		for i, p := range hits {
			if i == 8 {
				lines = append(lines, hang("  ", st.dim.Render(fmt.Sprintf("… %d more; the approve and merge popups list them all", len(hits)-8)), inner)...)
				break
			}
			name := st.sub1.Render(p.Key.Repository()) + st.muted.Render(fmt.Sprintf("#%d", p.Key.Number))
			// A title may be cut: the name beside it tells the pull request.
			lines = append(lines, "  "+fit(name, 30)+" "+st.sub0.Render(p.Title))
		}
	}
	foot := append([]string{""}, keyLines(st, inner,
		st.key.Render(m.firstKey("toggle_rule"))+" "+st.muted.Render("check"),
		st.key.Render(m.firstKey("choose"))+" "+st.muted.Render("select these"),
		st.key.Render(m.firstKey("add_to_selection"))+" "+st.muted.Render("add them"),
		st.key.Render(m.firstKey("filter_by_rule"))+" "+st.muted.Render("show only these"),
		st.key.Render(keymap.Back)+" "+st.muted.Render("cancel"))...)
	out, top := scrollBox(st, "select by rule", lines, foot, w, m.popupHeight(), v.top, keep, "")
	v.top = top
	v.items = boxItems(owner, top, len(out), len(foot))
	return out
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
	if len(r.Authors) > 0 {
		parts = append(parts, strings.Join(r.Authors, ", "))
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
	if r.MaxLines > 0 {
		parts = append(parts, fmt.Sprintf("≤%d lines", r.MaxLines))
	}
	if r.UpdatedWithinDays > 0 {
		parts = append(parts, fmt.Sprintf("updated ≤%dd ago", r.UpdatedWithinDays))
	}
	if r.CreatedWithinDays > 0 {
		parts = append(parts, fmt.Sprintf("created ≤%dd ago", r.CreatedWithinDays))
	}
	if r.ChecksPassing {
		parts = append(parts, "checks passing")
	}
	var s strings.Builder
	for i, p := range parts {
		if i > 0 {
			s.WriteString(" · ")
		}
		s.WriteString(p)
	}
	return s.String()
}

// keyLines lays out the keys of a popup's foot in lines of width, a key
// never apart from what it does.
func keyLines(st styles, width int, parts ...string) []string {
	sep := st.dim.Render(" · ")
	var out []string
	line := ""
	for _, p := range parts {
		switch {
		case line == "":
			line = p
		case lipgloss.Width(line+sep+p) <= width:
			line += sep + p
		default:
			out = append(out, line)
			line = p
		}
	}
	return append(out, line)
}
