package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/runner"
)

// prTabs are the PR screen's tabs.
var prTabs = []string{"overview", "files", "checks", "conversation"}

// prView is the screen of one opened pull request.
type prView struct {
	key    model.PRKey
	tab    int
	scroll int
}

func (m *Model) openPR(p model.PR) tea.Cmd {
	m.prv = prView{key: p.Key}
	m.view = viewPR
	return tea.Batch(m.ensureDetail(p.Key), m.ensureDiff(p))
}

func (m *Model) prKey(k string) tea.Cmd {
	is := func(a keymap.Action) bool { return m.keys.Matches(a, k) }
	p, ok := m.knownPR(m.prv.key)
	switch {
	case is("quit"), k == keymap.Back:
		m.view = viewList
		return m.ensure()
	case is("down"):
		m.prv.scroll++
	case is("up"):
		m.prv.scroll = max(0, m.prv.scroll-1)
	case is("page_down"):
		m.prv.scroll += m.height / 2
	case is("page_up"):
		m.prv.scroll = max(0, m.prv.scroll-m.height/2)
	case is("top"):
		m.prv.scroll = 0
	case is("bottom"):
		m.prv.scroll = 1 << 30
	case is("pr_next_tab"):
		m.prv.tab, m.prv.scroll = (m.prv.tab+1)%len(prTabs), 0
	case is("pr_prev_tab"):
		m.prv.tab, m.prv.scroll = (m.prv.tab+len(prTabs)-1)%len(prTabs), 0
	case !ok:
	case is("pr_approve"):
		return m.askApprove(p)
	case is("pr_merge"):
		return m.startBulk(runner.Merge)
	case is("pr_comment"):
		return m.startCompose(p, model.EventComment)
	case is("pr_request_changes"):
		return m.startCompose(p, model.EventRequestChanges)
	case is("pr_browse"):
		return m.browse(p)
	case is("pr_claude"):
		return m.openChat(p)
	}
	return nil
}

// prTabLabels are the PR screen's tabs as it shows them.
func (m *Model) prTabLabels() []string {
	var tabs []string
	for i, t := range prTabs {
		if i == m.prv.tab {
			tabs = append(tabs, m.st.tabOn.Render(t))
		} else {
			tabs = append(tabs, m.st.tabOff.Render(t))
		}
	}
	return tabs
}

func (m *Model) prTabWidths() []int {
	var ws []int
	for _, t := range m.prTabLabels() {
		ws = append(ws, lipgloss.Width(t))
	}
	return ws
}

func (m *Model) renderPR(b *strings.Builder, height int) {
	st := m.st
	p, ok := m.knownPR(m.prv.key)
	if !ok {
		b.WriteString(st.red.Render("  " + m.prv.key.String() + " is no longer listed"))
		return
	}
	head := st.bold.Render(p.Title) + "  " + st.muted.Render(fmt.Sprintf("%s · %s → %s", p.Author, p.HeadRef, p.BaseRef)) +
		"  " + m.heavyMarks(p)
	b.WriteString(fit(head, m.width) + "\n")
	b.WriteString(strings.Join(m.prTabLabels(), "   ") + "\n")
	b.WriteString(st.border.Render(strings.Repeat("╌", max(m.width, 0))) + "\n")
	ls := m.prLines(p)
	rest := max(height-3, 1)
	m.prv.scroll = min(m.prv.scroll, max(len(ls)-rest, 0))
	for i := range rest {
		s := ""
		if j := m.prv.scroll + i; j < len(ls) {
			s = ls[j].text
		}
		b.WriteString(fit(" "+s, m.width) + "\n")
	}
}

func (m *Model) prLines(p model.PR) []line {
	st := m.st
	d := m.cache.details[p.Key]
	waitDetail := func() []line {
		switch {
		case d == nil || d.loading:
			return []line{{text: st.dim.Render("reading…")}}
		case d.err != nil:
			return []line{{text: st.red.Render(d.err.Error())}}
		}
		return nil
	}
	switch prTabs[m.prv.tab] {
	case "overview":
		if w := waitDetail(); w != nil {
			return w
		}
		return m.bodyLines(d.val.Body, m.width-1)
	case "files":
		e := m.cache.diffs[diffID(p)]
		switch {
		case e == nil || e.loading:
			return []line{{text: st.dim.Render("reading the diff…")}}
		case e.err != nil:
			return []line{{text: st.red.Render(e.err.Error())}}
		}
		return m.diffLines(e.val, m.width-1, nil)
	case "checks":
		if w := waitDetail(); w != nil {
			return w
		}
		if len(d.val.Checks) == 0 {
			return []line{{text: st.dim.Render("no checks")}}
		}
		var out []line
		for _, c := range d.val.Checks {
			mark := m.ciMark(c.State)
			name := st.sub1.Render(c.Name)
			switch c.State {
			case model.CheckSkipped, model.CheckNeutral:
				mark, name = st.dim.Render("–"), st.dim.Render(c.Name)
			}
			out = append(out, line{text: mark + " " + name})
		}
		return out
	case "conversation":
		if w := waitDetail(); w != nil {
			return w
		}
		var out []line
		for _, c := range d.val.Comments {
			out = append(out, line{text: st.sub0.Render(c.Author) + " " + st.dim.Render(shortAge(m.set.Now().Sub(c.Created))+" ago")})
			for l := range strings.SplitSeq(strings.TrimSpace(htmlComment.ReplaceAllString(c.Body, "")), "\n") {
				out = append(out, line{text: "  " + st.muted.Render(l)})
			}
			out = append(out, line{})
		}
		var reviews []string
		for _, r := range p.Reviews {
			reviews = append(reviews, reviewerStyle(st, r.State).Render(r.Author+" "+strings.ToLower(strings.ReplaceAll(string(r.State), "_", " "))))
		}
		if len(reviews) == 0 {
			reviews = []string{st.dim.Render("none yet")}
		}
		return append(out, line{text: st.muted.Render("reviews: ") + strings.Join(reviews, "  ")})
	}
	return nil
}

// heavyMarks are the checks, the review decision and the line counts, or a
// spinner while they are read.
func (m *Model) heavyMarks(p model.PR) string {
	st := m.st
	if !p.HeavyRead() {
		return st.dim.Render(m.spinner())
	}
	return m.ciMark(p.Checks) + " " + m.decisionMark(p.Decision) + "  " +
		st.green.Render(fmt.Sprintf("+%d", p.Additions)) + " " + st.red.Render(fmt.Sprintf("−%d", p.Deletions))
}
