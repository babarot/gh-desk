package tui

import (
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
	// own are the popup's own keys, read before its parts.
	own []ownKey
	// close closes the popup, on esc and on a click outside. The key that
	// opened it does not (ADR-125).
	close func()
	// back, when set, is what esc does instead: the popup's innermost
	// level above its text box, such as stopping a request on its way.
	back func()
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
	// claim tries the actions that claim nav keys, such as recalling the
	// questions asked before, and reports whether one took the key.
	claim func(msg tea.KeyPressMsg) bool
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
}

// ownKey is a key of the popup's own: its action, and what it does.
type ownKey struct {
	action keymap.Action
	run    func() tea.Cmd
}

// popupKey reads a key with the popup of p open, in the order ADR-125
// sets: esc steps out of the innermost level; while a text box has the
// focus, a key that types goes to it before anything else; then the
// popup's own keys, the actions that claim nav keys, the nav keys the text
// box edits with, and the other parts; what is left goes to the text box.
// No question is read while the text box has the focus (ADR-066). quit is
// not read, so it never closes a popup and never quits from one.
func (m *Model) popupKey(p popupSpec, msg tea.KeyPressMsg) tea.Cmd {
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
