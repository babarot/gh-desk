package tui

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kyokomi/emoji/v2"
)

// A description is drawn from its Markdown, unless the config asks for it
// as written: headings lose their #, list items get a bullet or a box,
// code and tables keep their columns, and prose wraps at the edge. Only
// what pull request descriptions use is drawn; anything else is shown as
// written. HTML is turned into this Markdown first, in html.go. See
// ADR-013 and ADR-020.

// span is text in one style.
type span struct {
	text  string
	style lipgloss.Style
	// from is the span a wrapped piece came from, so pieces of one span
	// on a line are drawn as one.
	from int
}

func renderSpans(ss []span) string {
	var b strings.Builder
	for _, s := range ss {
		b.WriteString(s.style.Render(s.text))
	}
	return b.String()
}

// token is a word, a run of spaces, or one other character, such as one
// of a Japanese sentence, which may break anywhere.
var token = regexp.MustCompile(`[!-~]+|\s+|.`)

// noLineStart are the characters a line may not start with; each goes to
// the next line with what comes before it.
const noLineStart = "、。，．）」』】,.;:)!?！？"

// piece is a token of the span from.
type piece struct {
	text string
	from int
}

// units are the tokens of ss, joined where a line may not break between
// them: a word that goes on in the next span with no space between, such
// as code and the comma after it, and a character a line may not start
// with and what comes before it.
func units(ss []span) [][]piece {
	var out [][]piece
	word := false
	for i, s := range ss {
		for _, tok := range token.FindAllString(s.text, -1) {
			space := strings.TrimSpace(tok) == ""
			if k := len(out) - 1; k >= 0 && word && !space && joined(out[k][len(out[k])-1].text, tok) {
				out[k] = append(out[k], piece{tok, i})
			} else {
				out = append(out, []piece{{tok, i}})
			}
			word = !space
		}
	}
	return out
}

func joined(prev, next string) bool {
	r, _ := utf8.DecodeRuneInString(next)
	return ascii(prev) && ascii(next) || strings.ContainsRune(noLineStart, r)
}

func ascii(s string) bool {
	for i := range len(s) {
		if s[i] < '!' || s[i] > '~' {
			return false
		}
	}
	return true
}

// wrapSpans breaks spans into lines width cells wide. Lines after the first
// are indented by hang cells, under the text after a bullet.
func wrapSpans(ss []span, width, hang int) [][]span {
	var out [][]span
	var cur []span
	n := 0
	add := func(text string, s span, from int) {
		if k := len(cur) - 1; k >= 0 && cur[k].from == from {
			cur[k].text += text
		} else {
			cur = append(cur, span{text: text, style: s.style, from: from})
		}
		n += ansi.StringWidth(text)
	}
	newLine := func() {
		// The space a line broke at is not left at its end.
		if k := len(cur) - 1; k >= 0 {
			cur[k].text = strings.TrimRight(cur[k].text, " ")
		}
		out = append(out, cur)
		cur, n = nil, 0
		if hang > 0 {
			cur, n = []span{{text: strings.Repeat(" ", hang), from: -1}}, hang
		}
	}
	for _, u := range units(ss) {
		w := 0
		for _, p := range u {
			w += ansi.StringWidth(p.text)
		}
		switch {
		case strings.TrimSpace(u[0].text) == "":
			if n+w > width {
				newLine()
				continue
			}
			if n == hang && len(out) > 0 {
				continue
			}
		case n+w > width && w > width-hang:
			// A word longer than a line breaks where it reaches the edge.
			for _, p := range u {
				for _, r := range p.text {
					if n+ansi.StringWidth(string(r)) > width {
						newLine()
					}
					add(string(r), ss[p.from], p.from)
				}
			}
			continue
		case n+w > width:
			newLine()
		}
		for _, p := range u {
			add(p.text, ss[p.from], p.from)
		}
	}
	return append(out, cur)
}

var (
	mdFence    = regexp.MustCompile("^\\s*(```|~~~)\\s*(\\S*)")
	mdHeading  = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
	mdQuote    = regexp.MustCompile(`^\s*>\s?(.*)`)
	mdRule     = regexp.MustCompile(`^\s*([-*_])(\s*[-*_]){2,}\s*$`)
	mdTask     = regexp.MustCompile(`^(\s*)[-*+]\s+\[([ xX])\]\s+(.*)`)
	mdBullet   = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)`)
	mdOrdered  = regexp.MustCompile(`^(\s*)(\d+[.)])\s+(.*)`)
	mdTableSep = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)
	mdInline   = regexp.MustCompile("`[^`]+`|\\*\\*[^*]+\\*\\*")
)

// inline is a line of prose as spans: code in its own colors, bold in bold.
func (m *Model) inline(s string, style lipgloss.Style) []span {
	var out []span
	at := 0
	for _, loc := range mdInline.FindAllStringIndex(s, -1) {
		if loc[0] > at {
			out = append(out, span{text: emojize(s[at:loc[0]]), style: style})
		}
		if t := s[loc[0]:loc[1]]; t[0] == '`' {
			out = append(out, span{text: t[1 : len(t)-1], style: m.st.code})
		} else {
			out = append(out, span{text: emojize(t[2 : len(t)-2]), style: style.Bold(true)})
		}
		at = loc[1]
	}
	if at < len(s) {
		out = append(out, span{text: emojize(s[at:]), style: style})
	}
	return out
}

// shortcode is an emoji written as GitHub writes it, such as :smile:.
var shortcode = regexp.MustCompile(`:[a-z0-9_+-]+:`)

// emojize turns the shortcodes in prose into the emoji they name, as
// GitHub shows them; one with no emoji, such as :shipit:, stays as
// written. Code is never given to it.
func emojize(s string) string {
	if !strings.Contains(s, ":") {
		return s
	}
	codes := emoji.CodeMap()
	return shortcode.ReplaceAllStringFunc(s, func(c string) string {
		if e, ok := codes[c]; ok {
			return e
		}
		return c
	})
}

// shownTitle is a pull request's title as it is drawn, with its
// shortcodes as emoji as GitHub shows them. The title itself is kept as
// written: rules, the filter and Claude read it so. Every title drawn
// goes through it, so that one pull request reads the same everywhere.
func shownTitle(s string) string { return emojizeOutsideCode(s) }

// emojizeOutsideCode is emojize for a line that may hold code between
// backquotes, which stays as written.
func emojizeOutsideCode(s string) string {
	parts := strings.Split(s, "`")
	for i := 0; i < len(parts); i += 2 {
		parts[i] = emojize(parts[i])
	}
	return strings.Join(parts, "`")
}

// markdownLines draws a description, cleaned as bodyLines cleans it, in
// lines width cells wide.
func (m *Model) markdownLines(body string, width int) []line {
	st := m.st
	var out []line
	add := func(ss []span, hang int) {
		for _, l := range wrapSpans(ss, width, hang) {
			out = append(out, line{text: renderSpans(l)})
		}
	}
	src := strings.Split(body, "\n")
	for i := 0; i < len(src); i++ {
		l := src[i]
		if f := mdFence.FindStringSubmatch(l); f != nil {
			var code []string
			for i++; i < len(src) && !strings.HasPrefix(strings.TrimSpace(src[i]), f[1]); i++ {
				code = append(code, src[i])
			}
			out = append(out, m.codeBlock(code, f[2], width)...)
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(l), "|") {
			var rows []string
			for ; i < len(src) && strings.HasPrefix(strings.TrimSpace(src[i]), "|"); i++ {
				rows = append(rows, src[i])
			}
			i--
			out = append(out, m.table(rows, width)...)
			continue
		}
		if g := mdHeading.FindStringSubmatch(l); g != nil {
			add(m.inline(g[1], st.mauve.Bold(true)), 0)
			// A heading takes one line: the blank line under it goes.
			if i+1 < len(src) && strings.TrimSpace(src[i+1]) == "" {
				i++
			}
			continue
		}
		if g := mdQuote.FindStringSubmatch(l); g != nil {
			for _, w := range wrapSpans(m.inline(g[1], st.sub0), max(width-2, 1), 0) {
				out = append(out, line{text: st.dim.Render("┃ ") + renderSpans(w)})
			}
			continue
		}
		if mdRule.MatchString(l) {
			out = append(out, line{text: st.dim.Render(strings.Repeat("─", min(width, 40)))})
			continue
		}
		if g := mdTask.FindStringSubmatch(l); g != nil {
			box, text := span{text: "☐ ", style: st.dim}, st.sub1
			if g[2] != " " {
				box, text = span{text: "☑ ", style: st.green}, st.sub0
			}
			add(append([]span{{text: g[1]}, box}, m.inline(g[3], text)...), len(g[1])+2)
			continue
		}
		if g := mdBullet.FindStringSubmatch(l); g != nil {
			add(append([]span{{text: g[1]}, {text: "• ", style: st.dim}}, m.inline(g[2], st.sub1)...), len(g[1])+2)
			continue
		}
		if g := mdOrdered.FindStringSubmatch(l); g != nil {
			mark := g[2] + " "
			add(append([]span{{text: g[1]}, {text: mark, style: st.dim}}, m.inline(g[3], st.sub1)...), len(g[1])+len(mark))
			continue
		}
		if strings.TrimSpace(l) == "" {
			out = append(out, line{})
			continue
		}
		add(m.inline(l, st.sub1), 0)
	}
	return out
}

// codeBlock draws a fenced block on its own background, cut at the edge
// rather than wrapped, with its language at the right of the first line.
func (m *Model) codeBlock(code []string, lang string, width int) []line {
	w := 0
	for _, c := range code {
		w = max(w, ansi.StringWidth(c))
	}
	tag := ""
	if lang != "" {
		tag = " " + lang + " "
	}
	w = min(w+2+ansi.StringWidth(tag), width)
	out := make([]line, 0, len(code))
	for i, c := range code {
		if i == 0 && tag != "" && ansi.StringWidth(" "+c)+ansi.StringWidth(tag) <= w {
			out = append(out, line{text: m.st.block.Render(fit(" "+c, w-ansi.StringWidth(tag))) + m.st.blockLang.Render(tag)})
			continue
		}
		out = append(out, line{text: m.st.block.Render(fit(" "+c, w))})
	}
	return out
}

// table draws a Markdown table with its columns lined up, cut at the edge.
func (m *Model) table(rows []string, width int) []line {
	st := m.st
	var cells [][]string
	for _, r := range rows {
		if mdTableSep.MatchString(r) {
			continue
		}
		r = strings.TrimSpace(r)
		r = strings.TrimSuffix(strings.TrimPrefix(r, "|"), "|")
		var cs []string
		for c := range strings.SplitSeq(r, "|") {
			cs = append(cs, strings.NewReplacer("`", "", "**", "").Replace(emojizeOutsideCode(strings.TrimSpace(c))))
		}
		cells = append(cells, cs)
	}
	var ws []int
	for _, cs := range cells {
		for j, c := range cs {
			if j == len(ws) {
				ws = append(ws, 0)
			}
			ws[j] = max(ws[j], ansi.StringWidth(c))
		}
	}
	var out []line
	for i, cs := range cells {
		style := st.sub1
		if i == 0 {
			style = st.sub1.Bold(true)
		}
		var b strings.Builder
		for j, w := range ws {
			if j > 0 {
				b.WriteString(st.dim.Render(" │ "))
			}
			c := ""
			if j < len(cs) {
				c = cs[j]
			}
			b.WriteString(style.Render(c + spaces(w-ansi.StringWidth(c))))
		}
		out = append(out, line{text: ansi.Truncate(b.String(), width, "…")})
		if i == 0 && len(cells) > 1 {
			var sep []string
			for _, w := range ws {
				sep = append(sep, strings.Repeat("─", w))
			}
			out = append(out, line{text: ansi.Truncate(st.dim.Render(strings.Join(sep, "─┼─")), width, "…")})
		}
	}
	return out
}
