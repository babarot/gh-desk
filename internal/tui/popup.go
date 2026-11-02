package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-triage/internal/model"
)

// A popup is drawn by box, which cuts a line longer than its frame. What
// must be read whole (an error, an explanation, a comment, the pull
// requests a popup acts on) is wrapped here first, and a popup taller than
// the screen scrolls its body in scrollBox, keeping its foot (the question
// and its keys) in sight.

// popupTop is the screen row a popup is drawn from.
const popupTop = 2

// popupHeight is how many lines a popup may take: from popupTop down to
// above the status and key lines, which stay in sight under it.
func (m *Model) popupHeight() int { return max(m.height-popupTop-2, 5) }

// hang wraps text to what is left of width after prefix, styles kept:
// prefix leads the first line and the others are indented as far, so the
// text reads as one column.
func hang(prefix, text string, width int) []string {
	pw := lipgloss.Width(prefix)
	ls := strings.Split(lipgloss.Wrap(text, max(width-pw, 10), " "), "\n")
	out := make([]string, len(ls))
	for i, l := range ls {
		if i == 0 {
			out[i] = prefix + l
		} else {
			out[i] = spaces(pw) + l
		}
	}
	return out
}

// wrapped is text made safe and wrapped to width, every line in s.
func wrapped(s styleFn, text string, width int) []string {
	return hang("", s.Render(safeLine(text)), width)
}

// keysLines names prs by their keys, wrapped to width, in at most n
// lines: what is left over is counted on the last.
func keysLines(s styleFn, prs []model.PR, width, n int) []string {
	ls := wrapped(s, keysOf(prs), width)
	if len(ls) <= n {
		return ls
	}
	// Name as many as fit in n lines with the count after them.
	for k := len(prs) - 1; k > 0; k-- {
		more := fmt.Sprintf("… and %d more", len(prs)-k)
		ls = hang("", s.Render(safeLine(keysOf(prs[:k])))+"  "+s.Render(more), width)
		if len(ls) <= n {
			return ls
		}
	}
	return ls[:n]
}

// keepLines are the n body lines from from on that a scrollBox keeps in
// sight; the zero value keeps none.
type keepLines struct{ from, n int }

// scrollBox draws body and foot in a box w wide and at most h tall. The
// foot is always shown; the body scrolls from its line top, moved so that
// the lines of keep are in sight, its first when they do not all fit. The
// bottom edge tells how many body lines are above and below, and keys how
// to scroll. It returns the box and the top it drew from, for the caller
// to keep.
func scrollBox(st styles, title string, body, foot []string, w, h, top int, keep keepLines, keys string) ([]string, int) {
	room := max(h-2-len(foot), 1)
	if keep.n > 0 {
		top = max(top, keep.from+keep.n-room)
		top = min(top, keep.from)
	}
	top = min(max(top, 0), max(len(body)-room, 0))
	end := min(top+room, len(body))
	out := box(st, title, append(append([]string(nil), body[top:end]...), foot...), w)
	if len(body) > room {
		out[len(out)-1] = scrollEdge(st, w, top, len(body)-end, keys)
	}
	return out, top
}

// scrollEdge is the bottom edge of a box w wide that tells how many lines
// are above and below what it shows, and the keys that scroll it.
func scrollEdge(st styles, w, above, below int, keys string) string {
	var parts []string
	if above > 0 {
		parts = append(parts, fmt.Sprintf("↑ %d", above))
	}
	if below > 0 {
		parts = append(parts, fmt.Sprintf("↓ %d", below))
	}
	tell := " " + strings.Join(parts, "  ")
	if keys != "" {
		tell += " · " + keys
	}
	tell = ansi.Truncate(tell+" ", max(w-3, 0), "…")
	fill := max(w-2-lipgloss.Width(tell)-1, 0)
	return st.border.Render("╰"+strings.Repeat("─", fill)) + st.dim.Render(tell) + st.border.Render("─╯")
}

// scrollKeys names the keys that scroll a popup's body.
func (m *Model) scrollKeys() string {
	return m.lastKey("down") + " " + m.lastKey("up")
}
