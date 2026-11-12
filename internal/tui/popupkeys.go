package tui

import (
	"cmp"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// A popup declares the parts it holds, and the layer here reads their keys
// and mouse input the same way in every popup; a popup reads only the keys
// that are its own (ADR-125). docs/design/popups.md draws the popups and
// their parts.

// popupSpec is what the shared layer reads of the popup open, for the state
// it is in. It is built again for each key, as a popup's state changes it.
type popupSpec struct {
	// list is the list the popup holds, if it holds one.
	list *popupList
	// body is the text it holds to read, if it holds one.
	body *popupBody
	// question is the write it asks to confirm, if it asks one.
	question *popupQuestion
	// text is the text box it holds, if it holds one.
	text *popupText
	// draw draws the popup's box, and says on which of its rows the text
	// box with the focus starts, or -1. The popup is placed and drawn over
	// the screen from it (render, placePopup), so a popup declares itself
	// here and nowhere else.
	draw func() (lines []string, at int)
	// key and mouse take keys and mouse input of the popup's own before
	// the shared layer does, and report whether they took them.
	key   func(msg tea.KeyPressMsg) (tea.Cmd, bool)
	mouse func(msg tea.MouseMsg) (tea.Cmd, bool)
	// own are the popup's own keys, read before its parts.
	own []ownKey
	// close closes the popup, on esc and on a click outside. The key that
	// opened it does not (ADR-125).
	close func()
	// back, when set, is what esc does instead: the popup's innermost
	// level above its text box, such as stopping a request on its way.
	back func()
	// closeWords and backWords say what esc does, closing or going back:
	// "close" when empty.
	closeWords, backWords string
	// note is words that tell of keys no part names, after the parts'.
	note string
}

// popupText is a text box. While it has the focus it takes every key that
// types a character, the nav keys it edits with, and every key nothing
// else took; the popup's own keys and its claimed actions come before
// those (ADR-125).
type popupText struct {
	focused bool
	// focus puts the focus in it, on write.
	focus func() tea.Cmd
	// blur takes the focus out of it, on esc. nil keeps the focus in it as
	// long as the popup is open, and esc closes the popup at once.
	blur func()
	// edit gives a key to it.
	edit func(msg tea.Msg) tea.Cmd
	// editNav are the nav actions it edits with, which go to it and not to
	// the popup's list or body.
	editNav []keymap.Action
	// cursor is where its cursor is inside it, for the terminal to show
	// the text being composed there.
	cursor func() *tea.Cursor
	// claim tries the actions that claim nav keys, such as recalling the
	// questions asked before, and reports whether one took the key.
	claim func(msg tea.KeyPressMsg) bool
	// writeWords say what write does, blurWords what esc does from it
	// ("done writing" when empty), and typeWords what typing does there.
	writeWords, blurWords, typeWords string
}

// popupList is a list with a cursor at *sel over n items. nav moves the
// cursor, choose runs the item under it, and the mouse does as listMouse
// says.
type popupList struct {
	sel *int
	n   int
	ptr *pointer
	// choose runs the item under the cursor, on choose.
	choose func() tea.Cmd
	// click runs it on a second click; nil runs choose (ADR-053).
	click func() tea.Cmd
	// chooseWords say what choose does: the action's own when empty.
	chooseWords string
	// moveHint tells the keys that move it, where a text box takes the
	// letters that would otherwise.
	moveHint bool
}

// popupBody is text to read that scrolls: nav scrolls it by a line, a
// page or to an end, and the wheel by a few lines.
type popupBody struct {
	// scroll moves the first line shown by d, kept within the body.
	scroll func(d int)
	// page is how many lines the popup shows of it.
	page int
	// click takes a left click instead of the shared layer, which closes
	// the popup on a click outside it; nil has the layer do that.
	click func(x, y int)
}

// popupQuestion is a write to GitHub the popup asks to confirm: confirm
// runs it. It is never read while a text box has the focus (ADR-066).
type popupQuestion struct {
	confirm func() tea.Cmd
	// words say what confirm does: the action's own when empty.
	words string
}

// ownKey is a key of the popup's own: its action, and what it does.
type ownKey struct {
	action keymap.Action
	run    func() tea.Cmd
	// words say what it does: the action's own when empty. hide keeps it
	// out of the hints, as the popup tells of it otherwise. keyLine puts it
	// on the key line too, as one of the popup's most important keys
	// (ADR-125).
	words   string
	hide    bool
	keyLine bool
}

// popupHint is a key a popup tells of, and what it does.
type popupHint struct {
	key, words string
	keyLine    bool
}

// popupHints are the hints of the popup of p, from its parts and its own
// keys through the keymap, and what esc does there. No popup writes its
// keys by hand (ADR-125).
func (m *Model) popupHints(p popupSpec) (hs []popupHint, esc string) {
	words := func(w string, a keymap.Action) string {
		if w != "" {
			return w
		}
		return m.keys.Help(a)
	}
	add := func(a keymap.Action, w string, line bool) {
		if ks := m.keys.Keys(a); len(ks) > 0 {
			hs = append(hs, popupHint{key: ks[0], words: words(w, a), keyLine: line})
		}
	}
	t := p.text
	typing := t != nil && t.focused
	if typing && t.typeWords != "" {
		hs = append(hs, popupHint{key: "type", words: t.typeWords})
	}
	// A question writes to GitHub, so it is always on the key line too.
	if q := p.question; q != nil && !typing {
		add("confirm", q.words, true)
	}
	if l := p.list; l != nil {
		// A key that types is the text box's; with none other, no key is
		// told for moving.
		if up, down := m.untypedKey("up", "history_prev"), m.untypedKey("down", "history_next"); l.moveHint && up != "" && down != "" {
			key := arrowKey(up) + arrowKey(down)
			if up != "up" || down != "down" {
				key = up + "/" + down
			}
			hs = append(hs, popupHint{key: key, words: "move"})
		}
		add("choose", l.chooseWords, false)
	}
	// The most important keys come first, then write, then the rest.
	for _, o := range p.own {
		if !o.hide && o.keyLine {
			add(o.action, o.words, true)
		}
	}
	if t != nil && !typing && t.focus != nil {
		add("write", t.writeWords, false)
	}
	for _, o := range p.own {
		if !o.hide && !o.keyLine {
			add(o.action, o.words, false)
		}
	}
	if p.note != "" {
		hs = append(hs, popupHint{words: p.note})
	}
	switch {
	case p.back != nil:
		esc = cmp.Or(p.backWords, "back")
	case typing && t.blur != nil:
		esc = cmp.Or(t.blurWords, "done writing")
	default:
		esc = cmp.Or(p.closeWords, "close")
	}
	return hs, esc
}

// untypedKey is the first key of the actions as that types nothing, which
// reaches a list beside a text box with the focus; "" with none.
func (m *Model) untypedKey(as ...keymap.Action) string {
	for _, a := range as {
		for _, k := range m.keys.Keys(a) {
			if !typesText(k) {
				return k
			}
		}
	}
	return ""
}

// typesText reports whether the key k, as the keymap writes it, types a
// character into a text box: a single character, or space.
func typesText(k string) bool { return len([]rune(k)) == 1 || k == "space" }

// arrowKey writes up and down as arrows, which take one cell.
func arrowKey(k string) string {
	if k == "up" {
		return "↑"
	}
	return "↓"
}

// popupFootParts are the keys at the foot of the popup of p, each a key
// and what it does, esc last.
func (m *Model) popupFootParts(p popupSpec) []string {
	hs, esc := m.popupHints(p)
	var parts []string
	for _, h := range hs {
		parts = append(parts, m.hintPart(h.key, h.words))
	}
	return append(parts, m.hintPart(keymap.Back, esc))
}

// popupFoot is the line of keys at the foot of the popup of p, after lead:
// words of the popup's own, such as the question it asks, which end in
// their own space or separator.
func (m *Model) popupFoot(p popupSpec, lead string) string {
	return lead + strings.Join(m.popupFootParts(p), m.st.dim.Render(" · "))
}

// popupFootText is the keys of the popup of p as plain text, for a title.
func (m *Model) popupFootText(p popupSpec) string {
	hs, esc := m.popupHints(p)
	var parts []string
	for _, h := range hs {
		parts = append(parts, strings.TrimSpace(h.key+" "+h.words))
	}
	return strings.Join(append(parts, keymap.Back+" "+esc), " · ")
}

// popupKeyLine are the hints of the popup of p on the key line: only its
// most important keys, and what esc does (ADR-125, in place of ADR-024's
// rule for them). The rest are in the popup's frame.
func (m *Model) popupKeyLine(p popupSpec) []string {
	hs, esc := m.popupHints(p)
	var out []string
	for _, h := range hs {
		if h.keyLine {
			out = append(out, m.hintPart(h.key, h.words))
		}
	}
	return append(out, m.hintPart(keymap.Back, esc))
}

// hintPart is a key and what it does, or words alone with no key.
func (m *Model) hintPart(key, words string) string {
	if key == "" {
		return m.st.muted.Render(words)
	}
	return m.st.key.Render(key) + " " + m.st.muted.Render(words)
}

// popupKey reads a key with the popup of p open, in the order ADR-125
// sets: esc steps out of the innermost level; while a text box has the
// focus, a key that types goes to it before anything else; then the
// popup's own keys, the actions that claim nav keys, the nav keys the text
// box edits with, and the other parts; what is left goes to the text box.
// No question is read while the text box has the focus (ADR-066). quit is
// not read, so it never closes a popup and never quits from one.
func (m *Model) popupKey(p popupSpec, msg tea.KeyPressMsg) tea.Cmd {
	if p.key != nil {
		if cmd, took := p.key(msg); took {
			return cmd
		}
	}
	k := msg.String()
	t := p.text
	typing := t != nil && t.focused
	if k == keymap.Back {
		switch {
		case p.back != nil:
			p.back()
		case typing && t.blur != nil:
			t.blur()
		default:
			p.close()
		}
		return nil
	}
	if typing && msg.Text != "" {
		return t.edit(msg)
	}
	for _, o := range p.own {
		if m.keys.Matches(o.action, k) {
			return o.run()
		}
	}
	if typing {
		if t.claim != nil && t.claim(msg) {
			return nil
		}
		for _, a := range t.editNav {
			if m.keys.Matches(a, k) {
				return t.edit(msg)
			}
		}
	}
	if l := p.list; l != nil {
		if cmd, ok := m.popupListKey(l, k); ok {
			return cmd
		}
	}
	if b := p.body; b != nil && m.popupBodyKey(b, k) {
		return nil
	}
	if q := p.question; q != nil && !typing && m.keys.Matches("confirm", k) {
		return q.confirm()
	}
	if t != nil && !typing && t.focus != nil && m.keys.Matches("write", k) {
		return t.focus()
	}
	if typing {
		return t.edit(msg)
	}
	return nil
}

// navActions are the actions of nav, all of which a text box of many lines
// edits with: it moves its cursor by them.
var navActions = []keymap.Action{"up", "down", "top", "bottom", "page_up", "page_down"}

// popupListKey reads the keys of a list: nav moves its cursor, choose runs the
// item under it. ok is false for a key it does not read.
func (m *Model) popupListKey(l *popupList, k string) (tea.Cmd, bool) {
	is := func(a keymap.Action) bool { return m.keys.Matches(a, k) }
	last := max(l.n-1, 0)
	page := max(shownItems(l.ptr)-1, 1)
	switch {
	case is("down"):
		*l.sel = min(*l.sel+1, last)
	case is("up"):
		*l.sel = max(*l.sel-1, 0)
	case is("page_down"):
		*l.sel = min(*l.sel+page, last)
	case is("page_up"):
		*l.sel = max(*l.sel-page, 0)
	case is("bottom"):
		*l.sel = last
	case is("top"):
		*l.sel = 0
	case is("choose"):
		if l.n == 0 {
			return nil, true
		}
		*l.sel = min(*l.sel, last)
		return l.choose(), true
	default:
		return nil, false
	}
	return nil, true
}

// popupBodyKey reads the keys of a body: nav scrolls it. It reports
// whether it read the key.
func (m *Model) popupBodyKey(b *popupBody, k string) bool {
	is := func(a keymap.Action) bool { return m.keys.Matches(a, k) }
	page := max(b.page-1, 1)
	switch {
	case is("down"):
		b.scroll(1)
	case is("up"):
		b.scroll(-1)
	case is("page_down"):
		b.scroll(page)
	case is("page_up"):
		b.scroll(-page)
	case is("bottom"):
		b.scroll(1 << 20)
	case is("top"):
		b.scroll(-(1 << 20))
	default:
		return false
	}
	return true
}

// shownItems is how many items of a list its box shows, as the box was
// last drawn.
func shownItems(p *pointer) int {
	seen := map[int]bool{}
	for _, i := range p.items {
		if i >= 0 {
			seen[i] = true
		}
	}
	return len(seen)
}

// popupListMouse is mouse input on the popup of p, which holds a list: the
// wheel moves the cursor, a second click on an item runs it, and a click
// outside closes the popup.
func (m *Model) popupListMouse(p popupSpec, msg tea.MouseMsg) tea.Cmd {
	l := p.list
	run, closed := m.listMouse(msg, l.ptr, l.sel, l.n)
	switch {
	case closed:
		p.close()
	case run && l.click != nil:
		return l.click()
	case run:
		return l.choose()
	}
	return nil
}

// popupBodyMouse is mouse input on the popup of p, which holds a body: the
// wheel scrolls it, and a click outside closes the popup, unless the body
// takes its clicks itself.
func (m *Model) popupBodyMouse(p popupSpec, msg tea.MouseMsg) {
	b := p.body
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelDown:
			b.scroll(wheelLines)
		case tea.MouseWheelUp:
			b.scroll(-wheelLines)
		}
	case tea.MouseClickMsg:
		switch {
		case msg.Button != tea.MouseLeft:
		case b.click != nil:
			b.click(msg.X, msg.Y)
		case !m.popupAt.has(msg.X, msg.Y):
			p.close()
		}
	}
}
