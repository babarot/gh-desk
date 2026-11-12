package tui

import (
	"cmp"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/model"
)

// A popup is drawn by box, which cuts a line longer than its frame. What
// must be read whole (an error, an explanation, a comment, the pull
// requests a popup acts on) is wrapped here first, and a popup taller than
// the screen scrolls its body in scrollBox, keeping its foot (the question
// and its keys) in sight.

// popupTopMost is the highest row a popup is drawn from: under the header
// and the list's column heads.
const popupTopMost = 2

// popupTop is the row the popup open is drawn from.
func (m *Model) popupTop() int { return cmp.Or(m.popTop, popupTopMost) }

// placePopup decides the row popups are drawn from, when one opens from
// none: a quarter down the screen, where the eye is, or centered for a
// popup that would leave less room under it than above it there. It holds while popups are open, one taking the place of
// another or changing its height, so that nothing in them moves
// (ADR-127).
func (m *Model) placePopup() int {
	m.popTop = popupTopMost
	h := m.popupBoxHeight()
	quarter := max(popupTopMost, m.height/4)
	centered := max(popupTopMost, (m.height-h)/2)
	return min(quarter, centered)
}

// A popup is one of two widths: narrow for a short list to choose from,
// wide for anything to read or write (ADR-125). They are constants, not
// settings: they only keep popups alike, so that one popup taking the place
// of another keeps its width where it can.
const (
	popupNarrow = 84
	popupWide   = 110
)

// popupWidthOn is the width of a popup on a screen w wide, never wider
// than the screen.
func popupWidthOn(w int, wide bool) int {
	most := popupNarrow
	if wide {
		most = popupWide
	}
	return min(max(w-8, 40), most, max(w, 1))
}

// popupWidth is the width of a popup on this screen.
func (m *Model) popupWidth(wide bool) int { return popupWidthOn(m.width, wide) }

// popupHeight is how many lines a popup may take: from its top row down to
// above the status and key lines, which stay in sight under it.
func (m *Model) popupHeight() int { return max(m.height-m.popupTop()-2, 5) }

// hang wraps text to what is left of width after prefix, styles kept:
// prefix leads the first line and the others are indented as far, so the
// text reads as one column.
func hang(prefix, text string, width int) []string {
	return hangWith(prefix, spaces(lipgloss.Width(prefix)), text, width)
}

// hangWith is hang with rest leading the lines after the first, as wide as
// first: the line of a tree that goes on down, say.
func hangWith(first, rest, text string, width int) []string {
	ls := strings.Split(lipgloss.Wrap(text, max(width-lipgloss.Width(first), 10), " "), "\n")
	for i := range ls {
		if i == 0 {
			ls[i] = first + ls[i]
		} else {
			ls[i] = rest + ls[i]
		}
	}
	return ls
}

// treeLines are the prefixes of a line hung from the one above, at indent:
// its first line, and those it wraps to. The last of the lines hung from
// one closes the tree.
func treeLines(st styles, indent string, last bool) (first, rest string) {
	if last {
		return indent + st.border.Render("└ "), indent + "  "
	}
	return indent + st.border.Render("├ "), indent + st.border.Render("│ ")
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
	room := bodyRows(h, foot)
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

// bodyRows is how many lines of its body a scrollBox h tall with foot shows.
func bodyRows(h int, foot []string) int { return max(h-2-len(foot), 1) }

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
	return st.focus.Render("╰"+strings.Repeat("─", fill)) + st.dim.Render(tell) + st.focus.Render("─╯")
}

// scrollKeys names the keys that scroll a popup's body.
func (m *Model) scrollKeys() string {
	return m.lastKey("down") + " " + m.lastKey("up")
}
