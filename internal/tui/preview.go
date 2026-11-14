package tui

import (
	"fmt"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/diff"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
)

type paneMode int

// The pane's tabs, in the order it shows them.
const (
	paneOverview paneMode = iota
	paneDiff
)

// pane is the preview pane under the list: the overview or the diff of the
// pull request under the cursor.
type pane struct {
	on bool
	// height is the pane's lines, borders left out.
	height int
	mode   paneMode
	scroll int
	// files are the files of each pull request's diff folded or unfolded
	// by hand, by path; the others are folded when they match the
	// collapse patterns.
	files map[model.PRKey]map[string]bool
	// file is the file tab of each pull request's diff shown, by path;
	// with none, all the files are.
	file map[model.PRKey]string
	// tabRow is the line of the pane the file tabs were last drawn on,
	// -1 with none, and tabs where each tab is on it, for a click.
	tabRow int
	tabs   []fileTabSpan
	// folded are the parts of the description pane folded, by name.
	folded map[string]bool
	// heads are the parts, or the files of the diff, whose headers the
	// pane last drew, by its lines, for a click to fold them; links are
	// the pages a click on each line opens.
	heads []string
	links [][]link
	// bar is where the pane last drew what scrolls, for the scrollbar on
	// its right edge; the zero value when it draws its own.
	bar track
}

// fileTabSpan is a file tab on cells x to x+w of the pane's line; tab is
// the file's index, -1 for all.
type fileTabSpan struct{ x, w, tab int }

// link is a page a click on cells x to x+w of a line opens.
type link struct {
	x, w int
	url  string
}

// linked is a line built from pieces, some of which are links.
type linked struct {
	text  string
	n     int
	links []link
}

// add puts s at the end of the line, a link to url unless it is "".
func (l *linked) add(s, url string) {
	w := lipgloss.Width(s)
	if url != "" {
		l.links = append(l.links, link{x: l.n, w: w, url: url})
	}
	l.text += s
	l.n += w
}

// clip is the line's links within width cells, where it is cut.
func (l *linked) clip(width int) []link {
	var out []link
	for _, k := range l.links {
		if k.x < width {
			k.w = min(k.w, width-k.x)
			out = append(out, k)
		}
	}
	return out
}

func (p *pane) toggleMode() {
	if p.mode == paneOverview {
		p.mode = paneDiff
	} else {
		p.mode = paneOverview
	}
	p.scroll = 0
}

// line is a line of the pane's scrolled part, with the file it belongs to.
// A head is a file's header, named by its path in part, or that of a part
// of the description pane, named by part.
type line struct {
	text string
	file int
	head bool
	part string
	// links are the pages clicks on the line open.
	links []link
}

// diffLines are a diff's lines as the pane shows them,
// width cells wide. Like the parts of the description pane, a file's
// header runs to the edge with a rule, and the lines of an unfolded file
// have a gutter, closed under its last line. folded tells which files are
// one line; nil folds none.
func (m *Model) diffLines(files []diff.File, width int, folded func(diff.File) bool) []line {
	st := m.st
	gutter := st.border.Render("│ ")
	var out []line
	for i, f := range files {
		fold := folded != nil && folded(f)
		mark := "▾ "
		if fold {
			mark = "▸ "
		}
		add, del := f.Stat()
		head := st.blue.Render(mark) + st.file.Render(f.Path()) + "  " + st.green.Render(fmt.Sprintf("+%d", add)) + " " + st.red.Render(fmt.Sprintf("−%d", del))
		if f.Binary {
			head += "  " + st.dim.Render("binary")
		}
		head += " "
		head += st.border.Render(strings.Repeat("─", max(width-lipgloss.Width(head), 0)))
		out = append(out, line{text: head, file: i, head: true, part: f.Path()})
		if fold {
			continue
		}
		for _, h := range f.Hunks {
			hh := fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
			if h.Section != "" {
				hh += " " + st.dim.Render(h.Section)
			}
			out = append(out, line{text: gutter + st.hunk.Render(hh), file: i})
			for _, l := range m.hunkLines(h.Lines) {
				out = append(out, line{text: gutter + l, file: i})
			}
		}
		out = append(out, line{text: st.border.Render("╵"), file: i})
	}
	return out
}

// paneDiffLines are p's diff as the pane shows it: the file of the file
// tab shown, or all of them, each folded as it was left, or as the
// collapse patterns say.
func (m *Model) paneDiffLines(p model.PR, files []diff.File) []line {
	if i := m.fileTab(p, files); i >= 0 {
		return m.diffLines(files[i:i+1], max(m.width-4, 10), nil)
	}
	return m.diffLines(files, max(m.width-4, 10), func(f diff.File) bool { return m.fileFolded(p, f.Path()) })
}

// fileFolded tells whether the file of p's diff at path is folded in the
// pane: as it was left, or, before that, whether it matches the collapse
// patterns.
func (m *Model) fileFolded(p model.PR, path string) bool {
	if v, ok := m.pane.files[p.Key][path]; ok {
		return v
	}
	return plan.MatchAny(m.set.Collapse, path)
}

// fileTab is the index of the file of p's diff whose tab the pane shows,
// or -1 for all.
func (m *Model) fileTab(p model.PR, files []diff.File) int {
	if path := m.pane.file[p.Key]; path != "" {
		for i, f := range files {
			if f.Path() == path {
				return i
			}
		}
	}
	return -1
}

// showFileTab shows the tab of the file of p's diff at index i, or all
// of them with -1.
func (m *Model) showFileTab(p model.PR, files []diff.File, i int) {
	if i < 0 {
		delete(m.pane.file, p.Key)
	} else {
		m.pane.file[p.Key] = files[i].Path()
	}
	m.pane.scroll = 0
}

// stepFileTab shows the next or previous file tab of the diff; all comes
// before the first file.
func (m *Model) stepFileTab(dir int) {
	p, _, ok := m.currentPR()
	if !ok {
		return
	}
	e := m.cache.diffs[diffID(p)]
	if e == nil || e.loading || e.err != nil || len(e.val) < 2 {
		return
	}
	m.showFileTab(p, e.val, min(max(m.fileTab(p, e.val)+dir, -1), len(e.val)-1))
}

// stepPane moves the preview pane of p one tab along overview, all of the
// diff, then each file of it; it stops at either end, and does nothing
// while the pane is hidden.
func (m *Model) stepPane(p model.PR, dir int) tea.Cmd {
	if !m.pane.on {
		return nil
	}
	e := m.cache.diffs[diffID(p)]
	read := e != nil && !e.loading && e.err == nil
	switch {
	case m.pane.mode == paneOverview && dir > 0:
		m.pane.toggleMode()
		delete(m.pane.file, p.Key)
		return m.ensure()
	case m.pane.mode == paneOverview:
		return nil
	case dir < 0 && (!read || m.fileTab(p, e.val) < 0):
		m.pane.toggleMode()
		return m.ensure()
	}
	m.stepFileTab(dir)
	return nil
}

// clickFileTab shows the file tab at cell x of the pane's line, if one is
// there.
func (m *Model) clickFileTab(x int) {
	p, _, ok := m.currentPR()
	if !ok {
		return
	}
	e := m.cache.diffs[diffID(p)]
	if e == nil || e.loading || e.err != nil {
		return
	}
	for _, t := range m.pane.tabs {
		if x >= t.x && x < t.x+t.w && t.tab < len(e.val) {
			m.showFileTab(p, e.val, t.tab)
			return
		}
	}
}

// fileTabNames are the names of the diff's file tabs: each file's base
// name, with as many of its directories as tell it from the others.
func fileTabNames(files []diff.File) []string {
	names := make([]string, len(files))
	parts := make([][]string, len(files))
	depth := make([]int, len(files))
	for i, f := range files {
		parts[i] = strings.Split(f.Path(), "/")
		depth[i] = 1
	}
	for {
		same := map[string][]int{}
		for i, ps := range parts {
			names[i] = strings.Join(ps[len(ps)-min(depth[i], len(ps)):], "/")
			same[names[i]] = append(same[names[i]], i)
		}
		grew := false
		for _, is := range same {
			for _, i := range is {
				if len(is) > 1 && depth[i] < len(parts[i]) {
					depth[i]++
					grew = true
				}
			}
		}
		if !grew {
			return names
		}
	}
}

// fileTabLine is the line of the diff's file tabs, all first, and where
// each tab is on it. When they do not fit in width, the tabs before the
// one shown go, so that it stays in sight.
func (m *Model) fileTabLine(files []diff.File, cur, width int) (string, []fileTabSpan) {
	st := m.st
	labels := append([]string{fmt.Sprintf("all (%d)", len(files))}, fileTabNames(files)...)
	ws := make([]int, len(labels))
	for i, l := range labels {
		ws[i] = lipgloss.Width(" " + l + " ")
	}
	const more = "‹ "
	from := 0
	for from < cur+1 {
		w := 0
		if from > 0 {
			w = lipgloss.Width(more)
		}
		for _, x := range ws[from : cur+2] {
			w += x + len(tabGap)
		}
		if w-len(tabGap) <= width {
			break
		}
		from++
	}
	var b strings.Builder
	var spans []fileTabSpan
	at := 0
	if from > 0 {
		b.WriteString(st.dim.Render(more))
		at = lipgloss.Width(more)
	}
	for i := from; i < len(labels); i++ {
		if i > from {
			b.WriteString(tabGap)
			at += len(tabGap)
		}
		b.WriteString(pill(st.subOn, st.subOff, labels[i], i == cur+1))
		spans = append(spans, fileTabSpan{x: at, w: ws[i], tab: i - 1})
		at += ws[i]
	}
	return b.String(), spans
}

// foldFile folds or unfolds the file of p's diff at path.
func (m *Model) foldFile(p model.PR, path string) {
	if m.pane.files[p.Key] == nil {
		m.pane.files[p.Key] = map[string]bool{}
	}
	m.pane.files[p.Key][path] = !m.fileFolded(p, path)
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
	zeroWidth   = strings.NewReplacer("&#8203;", "", "\u200b", "")
)

// bodyLines are a description in lines width cells wide: links keep their
// text. It is drawn from its Markdown, its HTML turned into Markdown, or,
// when the config asks for it as written, HTML comments and some tags go,
// headings stand out and prose wraps at the edge, while tables and code
// blocks are cut instead, as wrapping would break their columns.
func (m *Model) bodyLines(body string, width int) []line {
	if strings.TrimSpace(body) == "" {
		return []line{{text: m.st.dim.Render("no description")}}
	}
	body = strings.ReplaceAll(body, "\r", "")
	width = max(width, 10)
	if !m.set.PlainDescription {
		body = zeroWidth.Replace(mdLinkRe.ReplaceAllString(htmlToMarkdown(body), "$1"))
		return m.markdownLines(strings.TrimRight(body, " \t\n"), width)
	}
	body = htmlTag.ReplaceAllString(htmlComment.ReplaceAllString(body, ""), "")
	body = zeroWidth.Replace(mdLinkRe.ReplaceAllString(body, "$1"))
	var out []line
	wrapped := func(l string, style lipgloss.Style) {
		for w := range strings.SplitSeq(ansi.Wrap(l, width, " "), "\n") {
			out = append(out, line{text: style.Render(w)})
		}
	}
	code := false
	for l := range strings.SplitSeq(body, "\n") {
		fence := strings.HasPrefix(strings.TrimSpace(l), "```")
		switch {
		case fence || code:
			out = append(out, line{text: m.st.sub1.Render(l)})
		case strings.HasPrefix(l, "#"):
			wrapped(l, m.st.mauve)
		case strings.HasPrefix(l, "|"):
			out = append(out, line{text: m.st.teal.Render(l)})
		default:
			wrapped(l, m.st.sub1)
		}
		if fence {
			code = !code
		}
	}
	return out
}

// mdLinkRe is a link or an image; the text, or the alt text, stays.
var mdLinkRe = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)

// jumpFile shows the next or previous file tab of the diff, or scrolls
// the overview to the next or previous part's header.
func (m *Model) jumpFile(dir int) {
	if m.pane.mode == paneDiff {
		m.stepFileTab(dir)
		return
	}
	p, _, ok := m.currentPR()
	if !ok {
		return
	}
	left, _ := m.descWidths()
	ls := m.descLines(p, left)
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
			body = []string{st.muted.Render(m.firstKey("menu") + " to fold or unfold")}
		}
	} else {
		var files []diff.File
		e := m.cache.diffs[diffID(p)]
		if e != nil && !e.loading && e.err == nil {
			files = e.val
		}
		tabs = m.paneTabs(p)
		hint = m.paneHint(p)
		m.pane.bar = track{}
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
		edge := st.border.Render(" │")
		if ok && m.pane.bar.thumb(i) {
			// On the frame, so it takes no width from what scrolls; it
			// has to stand out from the frame it is drawn on.
			edge = " " + st.muted.Render("┃")
		}
		b.WriteString(st.border.Render("│ ") + fit(s, inner) + edge + "\n")
	}
	b.WriteString(st.border.Render("╰"+strings.Repeat("─", max(m.width-2, 0))+"╯") + "\n")
}

// paneTabs are the pane's tabs as its top border draws them.
func (m *Model) paneTabs(p model.PR) string {
	return pill(m.st.tabOn, m.st.tabOff, "overview", m.pane.mode == paneOverview) + tabGap +
		pill(m.st.tabOn, m.st.tabOff, m.diffTabName(p), m.pane.mode == paneDiff)
}

// paneHint is what the pane's top border says at its right of the pull
// request shown: its name, which a click opens, and the pane's keys.
func (m *Model) paneHint(p model.PR) string {
	return p.Key.String() + " · " + m.firstKey("preview_down") + "/" + m.firstKey("preview_up") + " · " + m.firstKey("preview_tab")
}

// hintLinks are the links of the pane's top border, by the cells of the
// screen: the name of the pull request shown. The border is drawn as
// renderPane does, with the hint at its right end.
func (m *Model) hintLinks(p model.PR, tabs int) []link {
	hint := lipgloss.Width(m.paneHint(p))
	// "╭ ", the tabs and a space; a space before the hint and " ╮" after.
	if 2+tabs+1+1+hint+2 > m.width {
		return nil
	}
	return []link{{x: m.width - 2 - hint, w: lipgloss.Width(p.Key.String()), url: m.prURL(p)}}
}

// diffTabName is the name of the pane's diff tab, with the count of files
// once they are read.
func (m *Model) diffTabName(p model.PR) string {
	e := m.cache.diffs[diffID(p)]
	if e == nil || e.loading || e.err != nil {
		return "diff"
	}
	return fmt.Sprintf("diff (%d)", len(e.val))
}

// paneBody is the pane's lines: the overview, or what the columns
// do not say, why the pull request is here, what a bot updates, then the
// diff, scrolled.
func (m *Model) paneBody(p model.PR, n *model.Notification, e *entry[[]diff.File], files []diff.File, height int) []string {
	if m.pane.mode == paneOverview {
		return m.descPane(p, n, height)
	}
	st := m.st
	// The branches, the teams and the reviewers open their pages, as the
	// overview's facts do.
	var meta linked
	meta.add(st.sub0.Render(p.HeadRef), m.headURL(p))
	if p.BaseRef != "" {
		meta.add(st.dim.Render(" → "), "")
		meta.add(st.dim.Render(p.BaseRef), m.baseURL(p))
	}
	gap := "  "
	for _, r := range p.Requested {
		if r.Team {
			meta.add(gap, "")
			meta.add(st.teal.Render("@"+r.Login), m.webURL("orgs/"+p.Key.Owner+"/teams/"+r.Login))
			gap = " "
		}
	}
	gap = "  "
	for _, r := range p.Reviews {
		if r.Author != m.viewer {
			meta.add(gap, "")
			meta.add(reviewerStyle(st, r.State).Render(r.Author+reviewMark[r.State]), m.webURL(r.Author))
			gap = " "
		}
	}
	if mi, ok := m.cache.info[p.Key]; ok && p.State == model.StateOpen {
		d := plan.ForMerge(mi, p, m.viewer, m.set.Plan)
		meta.add("  "+st.muted.Render("merge:")+" "+m.stepStyle(d.Step).Render(m.mergeHow(d)), "")
	}
	top := []string{meta.text}
	topLinks := [][]link{meta.clip(max(m.width-4, 10))}
	if l, ok := m.unreadLine(n); ok {
		top = append(top, l.text)
		topLinks = append(topLinks, l.links)
	}
	if s, ok := m.bumpsLine(p, m.cache.details[p.Key]); ok {
		top = append(top, s)
		topLinks = append(topLinks, nil)
	}

	var ls []line
	switch {
	case e == nil || e.loading:
		ls = []line{{text: st.dim.Render("reading the diff…")}}
	case e.err != nil:
		ls = []line{{text: m.errText(e.err)}}
	default:
		ls = m.paneDiffLines(p, files)
	}
	rest := max(height-len(top), 1)
	strip := len(files) > 1
	if strip {
		rest--
	}
	m.pane.scroll = min(m.pane.scroll, max(len(ls)-rest, 0))
	shown := ls[m.pane.scroll:min(len(ls), m.pane.scroll+rest)]
	cur := m.fileTab(p, files)
	m.pane.tabRow, m.pane.tabs = -1, nil
	if strip {
		s, spans := m.fileTabLine(files, cur, max(m.width-4, 10))
		m.pane.tabRow, m.pane.tabs = len(top), spans
		top = append(top, s)
		topLinks = append(topLinks, nil)
	}
	m.pane.bar = track{row: len(top), room: rest, first: m.pane.scroll, total: len(ls)}
	m.pane.heads = make([]string, len(top), height)
	m.pane.links = append(make([][]link, 0, height), topLinks...)
	for _, l := range shown {
		top = append(top, l.text)
		// A file shown in its own tab does not fold.
		part := ""
		if l.head && cur < 0 {
			part = l.part
		}
		m.pane.heads = append(m.pane.heads, part)
		m.pane.links = append(m.pane.links, nil)
	}
	return top
}

// unreadLine is the first line of the unread comment a notification is
// about, if it has one, with its author's page.
func (m *Model) unreadLine(n *model.Notification) (line, bool) {
	st := m.st
	if n == nil || !n.Unread || n.LatestCommentURL == "" {
		return line{}, false
	}
	c := m.cache.comments[n.LatestCommentURL]
	switch {
	case c == nil || c.loading:
		return line{text: st.peach.Render("❝ ") + st.dim.Render("reading the comment…")}, true
	case c.err == nil:
		first := ""
		for l := range strings.SplitSeq(c.val.Body, "\n") {
			if strings.TrimSpace(l) != "" {
				first = strings.TrimSpace(l)
				break
			}
		}
		// The mark comes before the author.
		return line{
			text:  st.peach.Render("❝ ") + st.sub0.Render(c.val.Author) + " " + st.dim.Render(n.Reason) + "  " + st.sub1.Render(first),
			links: []link{{x: 2, w: lipgloss.Width(c.val.Author), url: m.userURL(c.val.Author)}},
		}, true
	}
	return line{}, false
}

// bumpsLine is what a bot's pull request updates, read from its body.
func (m *Model) bumpsLine(p model.PR, d *entry[model.Detail]) (string, bool) {
	st := m.st
	if !p.AuthorIsBot || d == nil || d.loading || d.err != nil {
		return "", false
	}
	bs := plan.Bumps(d.val.Body)
	if len(bs) == 0 {
		return "", false
	}
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
	return strings.Join(parts, "   "), true
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
