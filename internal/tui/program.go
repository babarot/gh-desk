package tui

import (
	"context"
	"io"
	"log/slog"
	"math/rand/v2"
	"reflect"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// Program is gh-desk on a terminal given as what is typed and what is
// drawn, not the one gh-desk runs in: the settings page's preview draws it
// in the page.
type Program struct {
	p   *tea.Program
	m   *Model
	ctx context.Context
}

// NewProgram makes gh-desk read what is typed from in and draw on out, a
// terminal of cols by rows with true color. It runs once Run is called.
func NewProgram(ctx context.Context, in io.Reader, out io.Writer, cols, rows int, src Source, run Runner, cl Claude, km keymap.Map, log *slog.Logger, set Settings) *Program {
	m := New(src, run, cl, km, log, set)
	m.ctx = ctx
	// TTY_FORCE has the color profile taken as a terminal's though out is
	// not one; COLORTERM makes it true color.
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out), tea.WithWindowSize(cols, rows),
		tea.WithEnvironment([]string{"TERM=xterm-256color", "COLORTERM=truecolor", "TTY_FORCE=1"}), tea.WithoutSignals())
	return &Program{p: p, m: m, ctx: ctx}
}

// Run shows gh-desk until it quits or the context given to NewProgram
// ends.
func (p *Program) Run() error {
	_, err := p.p.Run()
	p.m.waitClaude()
	if p.ctx.Err() != nil {
		return nil
	}
	return err
}

// Resize tells gh-desk the terminal's new size. It waits for the program
// to take it, and returns at once once it ended.
func (p *Program) Resize(cols, rows int) {
	p.p.Send(tea.WindowSizeMsg{Width: cols, Height: rows})
}

// Apply gives gh-desk new settings while it runs, as the preview does
// while its file is edited; it waits as Resize does.
func (p *Program) Apply(set Settings, km keymap.Map) {
	p.p.Send(settingsMsg{set: set, keys: km})
}

// settingsMsg is settings given while running. The lists are those of
// Settings.Lists again, so it is meant for a TUI that reads nothing
// (Settings.Offline): what one that reads had read since is not kept.
type settingsMsg struct {
	set  Settings
	keys keymap.Map
}

// themeName is the theme for the terminal's background.
func (m *Model) themeName() string {
	if m.light {
		return m.set.ThemeLight
	}
	return m.set.ThemeDark
}

// applySettings puts new settings in place of the ones gh-desk started
// with. The tabs that are still there, by their search, keep the cursor on
// its pull request, what was hidden or shown, and the groups folded. What is
// open stays: the ? list, the menu and the pane draw from the settings, a
// confirm decides its rows again, and S's popup forgets which rules were
// checked when the rules changed, as it holds them by their place.
func (m *Model) applySettings(msg settingsMsg) tea.Cmd {
	set := msg.set
	if set.Now == nil {
		set.Now = time.Now
	}
	if set.Pick == nil {
		set.Pick = rand.IntN
	}
	rulesChanged := !reflect.DeepEqual(m.set.Rules, set.Rules)
	m.set, m.keys = set, msg.keys
	m.st = newStyles(theme(m.themeName()))
	m.spinStyle = spinner(set.Spinner)

	type was struct {
		t   *tab
		key model.PRKey
	}
	old := map[string]was{}
	keyOf := func(t *tab) string {
		if t.inbox {
			return "\x00inbox"
		}
		return t.query
	}
	for _, t := range m.tabs {
		w := was{t: t}
		if rs := m.rows(t); t.sel >= 0 && t.sel < len(rs) && rs[t.sel].kind == rowPR {
			w.key = rs[t.sel].pr.Key
		}
		old[keyOf(t)] = w
	}
	shown := keyOf(m.tab())
	m.tabs, m.inbox, m.cur = nil, 0, min(m.cur, max(len(set.Tabs)-1, 0))
	for i, s := range set.Tabs {
		t := newTab(s)
		if s.Inbox {
			m.inbox = i
		}
		if w, ok := old[keyOf(t)]; ok {
			t.sel, t.offset, t.hidden, t.showExcluded, t.folded, t.keep = w.t.sel, w.t.offset, w.t.hidden, w.t.showExcluded, w.t.folded, w.key
			if !rulesChanged {
				t.ruleFilter = w.t.ruleFilter
			}
			if keyOf(t) == shown {
				m.cur = i
			}
		}
		m.tabs = append(m.tabs, t)
	}
	m.tabFrom = min(m.tabFrom, m.cur)
	m.loadCached()
	if set.Offline {
		m.markNotRead()
	}
	for _, t := range m.tabs {
		if w, ok := old[keyOf(t)]; ok {
			m.putCursorBack(t, w.t.sel)
			t.keep = model.PRKey{}
		}
	}
	if m.picker != nil && rulesChanged {
		m.picker.checked = map[int]bool{}
		m.picker.sel = min(m.picker.sel, len(m.rules())-1)
	}
	if m.bulk != nil {
		m.decideBulk()
	}
	return m.ensure()
}

// putCursorBack puts t's cursor on the pull request it was on, or at the
// line it was at, on a pull request.
func (m *Model) putCursorBack(t *tab, at int) {
	rs := m.rows(t)
	if len(rs) == 0 {
		t.sel, t.offset = 0, 0
		return
	}
	for i, r := range rs {
		if r.kind == rowPR && r.pr.Key == t.keep {
			t.sel = i
			return
		}
	}
	t.sel = min(at, len(rs)-1)
	if t.sel < 0 || rs[t.sel].kind != rowPR {
		m.toFirstPR(t)
	}
}
