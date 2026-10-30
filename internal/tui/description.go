package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/plan"
)

// The description pane: the title and the latest comment at the top, the
// facts the list's columns leave out in a column on the right, and on the
// left the parts the right column does not say, each of which folds: the
// description, the checks one by one, and the comments. Too narrow for the
// column, the facts go above the parts. See ADR-013.

const (
	// factsWidth is the width of the column of facts.
	factsWidth = 34
	// factsFrom is the narrowest pane that has room for the column.
	factsFrom = 100
	// factKey is the width of a fact's name and the space after it.
	factKey = 10
)

// The parts of the description pane.
const (
	partDescription = "description"
	partChecks      = "checks"
	partComments    = "comments"
)

// descWidths are the widths of the pane's left side and of the column of
// facts, which is 0 when the pane is too narrow for it.
func (m *Model) descWidths() (left, facts int) {
	inner := max(m.width-4, 10)
	if inner < factsFrom {
		return inner, 0
	}
	return inner - factsWidth - 3, factsWidth
}

// descPane is the description pane's lines, height of them.
func (m *Model) descPane(p model.PR, n *model.Notification, height int) []string {
	st := m.st
	left, right := m.descWidths()
	d := m.cache.details[p.Key]
	top := []string{st.text.Bold(true).Render(p.Title) + "  " + st.sub0.Render(p.Author)}
	if s, ok := m.unreadLine(n); ok {
		top = append(top, s)
	}
	if s, ok := m.bumpsLine(p, d); ok {
		top = append(top, s)
	}
	var facts []string
	if right > 0 {
		facts = m.facts(p, d, right)
	}
	ls := m.descLines(p, left)
	rest := max(height-len(top), 1)
	m.pane.scroll = min(m.pane.scroll, max(len(ls)-rest, 0))
	shown := ls[m.pane.scroll:min(len(ls), m.pane.scroll+rest)]
	m.pane.heads = make([]string, len(top), height)
	for _, l := range shown {
		top = append(top, l.text)
		m.pane.heads = append(m.pane.heads, l.part)
	}
	if right == 0 {
		return top
	}
	out := make([]string, height)
	for i := range out {
		var l, f string
		if i < len(top) {
			l = top[i]
		}
		if i < len(facts) {
			f = facts[i]
		}
		out[i] = fit(l, left) + st.border.Render(" │ ") + f
	}
	return out
}

// descLines are the parts of the description pane, width cells wide, with
// the facts above them when the pane has no column for them.
func (m *Model) descLines(p model.PR, width int) []line {
	st := m.st
	var out []line
	if _, right := m.descWidths(); right == 0 {
		for _, f := range m.facts(p, m.cache.details[p.Key], width) {
			out = append(out, line{text: f})
		}
		out = append(out, line{})
	}
	head := func(part, rest string) bool {
		mark := "▾ "
		if m.pane.folded[part] {
			mark = "▸ "
		}
		out = append(out, line{text: st.sub0.Render(mark) + st.group.Render(part) + rest, head: true, part: part})
		return !m.pane.folded[part]
	}
	indent := func(ls []line, by string) {
		for _, l := range ls {
			out = append(out, line{text: by + l.text})
		}
	}
	d := m.cache.details[p.Key]
	if head(partDescription, "") {
		switch {
		case d == nil || d.loading:
			out = append(out, line{text: "  " + st.dim.Render("reading the description…")})
		case d.err != nil:
			out = append(out, line{text: "  " + st.red.Render(d.err.Error())})
		default:
			indent(m.bodyLines(d.val.Body, width-2), "  ")
		}
	}
	if d == nil || d.loading || d.err != nil {
		return out
	}
	if cs := d.val.Checks; len(cs) > 0 {
		out = append(out, line{})
		if head(partChecks, st.dim.Render(fmt.Sprintf(" %d  ", len(cs)))+m.checkCounts(cs)) {
			required := m.requiredChecks(p)
			for _, c := range cs {
				s := "  " + m.ciMark(c.State) + " " + st.sub1.Render(c.Name)
				if required[c.Name] {
					s += st.dim.Render("  required")
				}
				out = append(out, line{text: s})
			}
		}
	}
	if cs := d.val.Comments; len(cs) > 0 {
		out = append(out, line{})
		if head(partComments, st.dim.Render(fmt.Sprintf(" %d", len(cs)))) {
			for i, c := range cs {
				if i > 0 {
					out = append(out, line{})
				}
				s := "  " + st.sub0.Render(c.Author)
				if !c.Created.IsZero() {
					s += "  " + st.dim.Render(shortAge(m.set.Now().Sub(c.Created))+" ago")
				}
				out = append(out, line{text: s})
				indent(m.bodyLines(c.Body, width-4), "    ")
			}
		}
	}
	return out
}

// checkCounts writes how many checks passed, failed and are running.
func (m *Model) checkCounts(cs []model.Check) string {
	var ok, failed, running int
	for _, c := range cs {
		switch c.State {
		case model.CheckSuccess:
			ok++
		case model.CheckFailure, model.CheckError:
			failed++
		case model.CheckPending, model.CheckExpected:
			running++
		}
	}
	var parts []string
	if ok > 0 {
		parts = append(parts, m.st.green.Render(fmt.Sprintf("✓%d", ok)))
	}
	if failed > 0 {
		parts = append(parts, m.st.red.Render(fmt.Sprintf("✗%d", failed)))
	}
	if running > 0 {
		parts = append(parts, m.st.yellow.Render(fmt.Sprintf("●%d", running)))
	}
	return strings.Join(parts, " ")
}

// requiredChecks are the names of the checks the base branch requires, as
// far as the merge information read says.
func (m *Model) requiredChecks(p model.PR) map[string]bool {
	out := map[string]bool{}
	if mi, ok := m.cache.info[p.Key]; ok {
		for _, c := range mi.Checks {
			if c.Required {
				out[c.Name] = true
			}
		}
	}
	return out
}

// facts are what the list's columns leave out, a fact a line, width cells
// wide: a fact whose values do not fit on one line goes on over the next.
func (m *Model) facts(p model.PR, d *entry[model.Detail], width int) []string {
	st := m.st
	var out []string
	fact := func(name string, values ...string) {
		if len(values) == 0 {
			return
		}
		key := st.muted.Render(name + spaces(factKey-len(name)))
		cur, n := "", 0
		for _, v := range values {
			w := lipgloss.Width(v)
			if n > 0 && n+1+w > width-factKey {
				out = append(out, fit(key+cur, width))
				key, cur, n = spaces(factKey), "", 0
			}
			if n > 0 {
				cur += " "
				n++
			}
			cur += v
			n += w
		}
		out = append(out, fit(key+cur, width))
	}
	words := func(s string, style styleFn) []string {
		var out []string
		for _, w := range strings.Fields(s) {
			out = append(out, style.Render(w))
		}
		return out
	}

	switch {
	case p.State == model.StateMerged:
		fact("status", st.mauve.Render("merged"))
	case p.State == model.StateClosed:
		fact("status", st.red.Render("closed"))
	case p.Draft:
		fact("status", st.dim.Render("draft"))
	default:
		fact("status", st.green.Render("open"))
	}
	if mi, ok := m.cache.info[p.Key]; ok && p.State == model.StateOpen {
		dec := plan.ForMerge(mi, p, m.viewer, m.set.Plan)
		how := dec.Reason
		if dec.Method != "" {
			how += " (" + methodWord(dec.Method) + ")"
		}
		fact("merge", words(how, m.stepStyle(dec.Step))...)
	}
	var reviews []string
	for _, r := range p.Reviews {
		if r.Author != m.viewer {
			reviews = append(reviews, reviewerStyle(st, r.State).Render(r.Author+reviewMark[r.State]))
		}
	}
	fact("reviews", reviews...)
	var requested []string
	for _, r := range p.Requested {
		if r.Team {
			requested = append(requested, st.teal.Render("@"+r.Login))
		} else {
			requested = append(requested, st.sub0.Render(r.Login))
		}
	}
	fact("requested", requested...)
	switch {
	case d != nil && !d.loading && d.err == nil && len(d.val.Checks) > 0:
		fact("checks", m.checkCounts(d.val.Checks))
	case p.Checks != "":
		fact("checks", m.ciMark(p.Checks))
	}
	if p.HeavyHead != "" {
		var labels []string
		for _, l := range p.Labels {
			labels = append(labels, st.teal.Render(l))
		}
		fact("labels", labels...)
		fact("size", st.green.Render(fmt.Sprintf("+%d", p.Additions)), st.red.Render(fmt.Sprintf("−%d", p.Deletions)),
			st.muted.Render(fmt.Sprintf("· %d files", p.ChangedFiles)))
	}
	if p.HeadRef != "" {
		fact("branch", st.sub0.Render(p.HeadRef))
		if p.BaseRef != "" {
			fact("", st.dim.Render("→ "+p.BaseRef))
		}
	}
	if !p.Created.IsZero() {
		fact("opened", st.sub0.Render(shortAge(m.set.Now().Sub(p.Created))+" ago"))
	}
	if !p.Updated.IsZero() {
		fact("updated", st.sub0.Render(shortAge(m.set.Now().Sub(p.Updated))+" ago"))
	}
	return out
}

// foldPart folds or unfolds the first part whose header the pane shows,
// or, with none in sight, the part the pane is in; the header stays in
// sight.
func (m *Model) foldPart() {
	p, _, ok := m.currentPR()
	if !ok {
		return
	}
	for _, part := range m.pane.heads {
		if part != "" {
			m.pane.folded[part] = !m.pane.folded[part]
			return
		}
	}
	left, _ := m.descWidths()
	ls := m.descLines(p, left)
	for i := min(m.pane.scroll, len(ls)-1); i >= 0; i-- {
		if ls[i].head {
			m.pane.folded[ls[i].part] = !m.pane.folded[ls[i].part]
			m.pane.scroll = i
			return
		}
	}
}
