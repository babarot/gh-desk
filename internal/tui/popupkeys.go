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
	// own are the popup's own keys, read before its parts.
	own []ownKey
	// opener is the action that opened the popup; it closes it, unless a
	// part reads its key first.
	opener keymap.Action
	// close closes the popup, on esc, on the opener and on a click outside.
	close func()
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

// ownKey is a key of the popup's own: its action, and what it does.
type ownKey struct {
	action keymap.Action
	run    func() tea.Cmd
}

// popupKey reads a key with the popup of p open: esc and the popup's own
// keys first, then the keys of its parts, then the opener. quit is not
// read: it never closes a popup, and never quits from one.
func (m *Model) popupKey(p popupSpec, k string) tea.Cmd {
	if k == keymap.Back {
		p.close()
		return nil
	}
	for _, o := range p.own {
		if m.keys.Matches(o.action, k) {
			return o.run()
		}
	}
	if l := p.list; l != nil {
		if cmd, ok := m.popupListKey(l, k); ok {
			return cmd
		}
	}
	if p.opener != "" && m.keys.Matches(p.opener, k) {
		p.close()
	}
	return nil
}

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
