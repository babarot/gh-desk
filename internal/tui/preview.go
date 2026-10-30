package tui

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/babarot/gh-triage/internal/diff"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/plan"
)

type paneMode int

const (
	paneDiff paneMode = iota
	paneBody
)

// pane is the preview pane under the list: the diff or the description of
// the pull request under the cursor.
type pane struct {
	on     bool
	mode   paneMode
	scroll int
	unfold map[model.PRKey]bool
}

func (p *pane) toggleMode() {
	if p.mode == paneDiff {
		p.mode = paneBody
	} else {
		p.mode = paneDiff
	}
	p.scroll = 0
}

// line is a line of the pane's scrolled part, with the file it belongs to.
type line struct {
	text string
	file int
	head bool
}

// diffLines are a diff's lines as the pane and the PR screen show them.
// Files matching the collapse patterns are one line until unfolded.
func (m *Model) diffLines(files []diff.File, unfold bool) []line {
	st := m.st
	var out []line
	for i, f := range files {
		add, del := f.Stat()
		head := st.file.Render("── "+f.Path()) + "  " + st.green.Render(fmt.Sprintf("+%d", add)) + " " + st.red.Render(fmt.Sprintf("−%d", del))
		folded := !unfold && plan.MatchAny(m.set.Collapse, f.Path())
		if folded {
			head += "  " + st.dim.Render("folded · "+m.firstKey("unfold")+" to unfold")
		}
		if f.Binary {
			head += "  " + st.dim.Render("binary")
		}
		out = append(out, line{text: head, file: i, head: true})
		if folded {
			continue
		}
		for _, h := range f.Hunks {
			hh := fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
			if h.Section != "" {
				hh += " " + st.dim.Render(h.Section)
			}
			out = append(out, line{text: st.hunk.Render(hh), file: i})
			for _, l := range m.hunkLines(h.Lines) {
				out = append(out, line{text: l, file: i})
			}
		}
	}
	return out
}

// hunkLines colors a hunk's lines, marking the words that changed between
// a deleted line and the added line in its place.
func (m *Model) hunkLines(ls []diff.Line) []string {
	st := m.st
	out := make([]string, len(ls))
	for i := 0; i < len(ls); {
		if ls[i].Kind != diff.Deleted {
			switch ls[i].Kind {
			case diff.Added:
				out[i] = st.add.Render("+" + ls[i].Text)
			default:
				out[i] = st.sub0.Render(" " + ls[i].Text)
			}
			i++
			continue
		}
		// A run of deleted lines, then the run of added lines after it.
		d := i
		for i < len(ls) && ls[i].Kind == diff.Deleted {
			i++
		}
		a := i
		for i < len(ls) && ls[i].Kind == diff.Added {
			i++
		}
		for j := d; j < a; j++ {
			out[j] = st.del.Render("-" + ls[j].Text)
		}
		for j := a; j < i; j++ {
			out[j] = st.add.Render("+" + ls[j].Text)
		}
		for n := 0; n < a-d && n < i-a; n++ {
			oldSp, newSp := diff.Inline(ls[d+n].Text, ls[a+n].Text)
			out[d+n] = st.del.Render("-") + marked(ls[d+n].Text, oldSp, st.del)
			out[a+n] = st.add.Render("+") + marked(ls[a+n].Text, newSp, st.add)
		}
	}
	return out
}

// marked draws s in style, with the spans in bold and underlined.
func marked(s string, spans []diff.Span, style lipgloss.Style) string {
	if len(spans) == 0 {
		return style.Render(s)
	}
	hi := style.Bold(true).Underline(true)
	var b strings.Builder
	at := 0
	for _, sp := range spans {
		b.WriteString(style.Render(s[at:sp.Start]))
		b.WriteString(hi.Render(s[sp.Start:sp.End]))
		at = sp.End
	}
	b.WriteString(style.Render(s[at:]))
	return b.String()
}

var (
	htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	htmlTag     = regexp.MustCompile(`</?(details|summary|p|br|img|sub|sup|div|span)[^>]*>`)
	zeroWidth   = strings.NewReplacer("&#8203;", "", "​", "")
)

// bodyLines are a description as plain lines: links keep their text, HTML
// comments and tags go, headings stand out.
func (m *Model) bodyLines(body string) []line {
	if strings.TrimSpace(body) == "" {
		return []line{{text: m.st.dim.Render("no description")}}
	}
	body = htmlTag.ReplaceAllString(htmlComment.ReplaceAllString(body, ""), "")
	body = zeroWidth.Replace(mdLinkRe.ReplaceAllString(body, "$1"))
	var out []line
	for _, l := range strings.Split(strings.ReplaceAll(body, "\r", ""), "\n") {
		switch {
		case strings.HasPrefix(l, "#"):
			out = append(out, line{text: m.st.mauve.Render(l)})
		case strings.HasPrefix(l, "|"):
			out = append(out, line{text: m.st.teal.Render(l)})
		default:
			out = append(out, line{text: m.st.sub1.Render(l)})
		}
	}
	return out
}

var mdLinkRe = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

// jumpFile scrolls the pane to the next or previous file's header.
func (m *Model) jumpFile(dir int) {
	p, _, ok := m.currentPR()
	if !ok || m.pane.mode != paneDiff {
		return
	}
	e := m.cache.diffs[diffID(p)]
	if e == nil || e.loading {
		return
	}
	ls := m.diffLines(e.val, m.pane.unfold[p.Key])
	if dir > 0 {
		for i := m.pane.scroll + 1; i < len(ls); i++ {
			if ls[i].head {
				m.pane.scroll = i
				return
			}
		}
		return
	}
	for i := min(m.pane.scroll, len(ls)) - 1; i >= 0; i-- {
		if ls[i].head {
			m.pane.scroll = i
			return
		}
	}
	m.pane.scroll = 0
}

// renderPane draws the preview pane in height lines, borders included.
func (m *Model) renderPane(b *strings.Builder, height int) {
	st := m.st
	inner := max(m.width-4, 10)
	p, n, ok := m.currentPR()
	var tabs, hint string
	var body []string
	if !ok {
		tabs = st.title.Render("preview")
		if r, isRow := m.current(); isRow && r.kind == rowGroup {
			hint = fmt.Sprintf("%d pull requests", r.count)
			body = []string{st.muted.Render(m.firstKey("open") + " to fold or unfold")}
		}
	} else {
		var files []diff.File
		e := m.cache.diffs[diffID(p)]
		if e != nil && !e.loading && e.err == nil {
			files = e.val
		}
		diffTab := "diff"
		if files != nil {
			diffTab = fmt.Sprintf("diff %d", len(files))
		}
		pill := func(s string, on bool) string {
			if on {
				return st.tabPill.Render(" " + s + " ")
			}
			return st.tabPillOff.Render(" " + s + " ")
		}
		tabs = pill(diffTab, m.pane.mode == paneDiff) + " " + pill("description", m.pane.mode == paneBody)
		hint = p.Key.String() + " · " + m.firstKey("pane_down") + "/" + m.firstKey("pane_up") + " · " + m.firstKey("pane_tab")
		body = m.paneBody(p, n, e, files, height-2)
	}
	top := st.border.Render("╭ ") + tabs + " "
	right := " " + st.dim.Render(hint) + st.border.Render(" ╮")
	fill := m.width - lipgloss.Width(top) - lipgloss.Width(right)
	b.WriteString(top + st.border.Render(strings.Repeat("─", max(fill, 0))) + right + "\n")
	for i := 0; i < height-2; i++ {
		s := ""
		if i < len(body) {
			s = body[i]
		}
		b.WriteString(st.border.Render("│ ") + fit(s, inner) + st.border.Render(" │") + "\n")
	}
	b.WriteString(st.border.Render("╰"+strings.Repeat("─", max(m.width-2, 0))+"╯") + "\n")
}

// paneBody is the pane's lines: what the columns do not say, why the
// pull request is here, what a bot updates, then the diff or the
// description, scrolled.
func (m *Model) paneBody(p model.PR, n *model.Notification, e *entry[[]diff.File], files []diff.File, height int) []string {
	st := m.st
	meta := st.sub0.Render(p.HeadRef)
	if p.BaseRef != "" {
		meta += st.dim.Render(" → " + p.BaseRef)
	}
	var teams, others []string
	for _, r := range p.Requested {
		if r.Team {
			teams = append(teams, st.teal.Render("@"+r.Login))
		}
	}
	for _, r := range p.Reviews {
		if r.Author != m.viewer {
			others = append(others, reviewerStyle(st, r.State).Render(r.Author+reviewMark[r.State]))
		}
	}
	if len(teams) > 0 {
		meta += "  " + strings.Join(teams, " ")
	}
	if len(others) > 0 {
		meta += "  " + strings.Join(others, " ")
	}
	if mi, ok := m.cache.info[p.Key]; ok && p.State == model.StateOpen {
		d := plan.ForMerge(mi, p, m.viewer, m.set.Plan)
		how := d.Reason
		if d.Method != "" {
			how += " (" + strings.ToLower(string(d.Method)) + ")"
		}
		meta += "  " + st.muted.Render("merge:") + " " + m.stepStyle(d.Step).Render(how)
	}
	top := []string{meta}
	if n != nil && n.Unread && n.LatestCommentURL != "" {
		c := m.cache.comments[n.LatestCommentURL]
		switch {
		case c == nil || c.loading:
			top = append(top, st.peach.Render("❝ ")+st.dim.Render("reading the comment…"))
		case c.err == nil:
			first := ""
			for _, l := range strings.Split(c.val.Body, "\n") {
				if strings.TrimSpace(l) != "" {
					first = strings.TrimSpace(l)
					break
				}
			}
			top = append(top, st.peach.Render("❝ ")+st.sub0.Render(c.val.Author)+" "+st.dim.Render(n.Reason)+"  "+st.sub1.Render(first))
		}
	}
	d := m.cache.details[p.Key]
	if p.AuthorIsBot && d != nil && !d.loading && d.err == nil {
		if bs := plan.Bumps(d.val.Body); len(bs) > 0 {
			var parts []string
			for _, bp := range bs[:min(len(bs), 2)] {
				s := st.teal.Render("◆ ") + st.sub1.Render(bp.Package)
				if bp.Update != "" {
					s += " " + st.dim.Render(bp.Update)
				}
				change := bp.To
				if bp.From != "" {
					change = bp.From + " → " + bp.To
				}
				s += " " + st.yellow.Render(change)
				if bp.Pending != "" {
					s += " " + st.dim.Render("(pending "+bp.Pending+")")
				}
				parts = append(parts, s)
			}
			if len(bs) > 2 {
				parts = append(parts, st.dim.Render(fmt.Sprintf("+%d", len(bs)-2)))
			}
			top = append(top, strings.Join(parts, "   "))
		}
	}

	var ls []line
	switch {
	case m.pane.mode == paneBody:
		switch {
		case d == nil || d.loading:
			ls = []line{{text: st.dim.Render("reading the description…")}}
		case d.err != nil:
			ls = []line{{text: st.red.Render(d.err.Error())}}
		default:
			ls = m.bodyLines(d.val.Body)
		}
	case e == nil || e.loading:
		ls = []line{{text: st.dim.Render("reading the diff…")}}
	case e.err != nil:
		ls = []line{{text: st.red.Render(e.err.Error())}}
	default:
		ls = m.diffLines(files, m.pane.unfold[p.Key])
	}
	rest := max(height-len(top), 1)
	strip := m.pane.mode == paneDiff && len(files) > 1
	if strip {
		rest--
	}
	m.pane.scroll = min(m.pane.scroll, max(len(ls)-rest, 0))
	shown := ls[m.pane.scroll:min(len(ls), m.pane.scroll+rest)]
	if strip {
		cur := 0
		if len(shown) > 0 {
			cur = shown[0].file
		}
		var names []string
		for i, f := range files {
			name := f.Path()[strings.LastIndex(f.Path(), "/")+1:]
			if i == cur {
				names = append(names, st.mauve.Bold(true).Render(name))
			} else {
				names = append(names, st.muted.Render(name))
			}
		}
		top = append(top, strings.Join(names, st.dim.Render(" · ")))
	}
	for _, l := range shown {
		top = append(top, l.text)
	}
	return top
}

func reviewerStyle(st styles, s model.ReviewState) lipgloss.Style {
	switch s {
	case model.ReviewApproved:
		return st.green
	case model.ReviewChangesRequested:
		return st.red
	}
	return st.sub0
}
