package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-triage/internal/tui/keymap"
)

func mouseAt(m *Model, msg tea.Msg) {
	_, cmd := m.Update(msg)
	run(m, cmd)
}

// clickTwice clicks at x, y twice, as a link wants to open its page.
func clickTwice(m *Model, x, y int) {
	mouseAt(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	mouseAt(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

// lineOf is the first line of the screen that has s, and where s starts
// in it.
func lineOf(t *testing.T, m *Model, s string) (x, y int) {
	t.Helper()
	for y, l := range strings.Split(screen(m), "\n") {
		if before, _, ok := strings.Cut(l, s); ok {
			return len([]rune(before)), y
		}
	}
	t.Fatalf("%q is not on the screen:\n%s", s, screen(m))
	return 0, 0
}

func TestClickTab(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	x, y := lineOf(t, m, "3 involved")
	mouseAt(m, tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseLeft})
	if m.cur != 2 {
		t.Fatalf("tab = %d, want 2", m.cur)
	}
}

func TestClickRowAndDoubleClick(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	x, y := lineOf(t, m, "▸ requested from a team")
	mouseAt(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if r, _ := m.current(); r.kind != rowGroup {
		t.Fatalf("row = %+v, want the team group", r)
	}
	// A single click on a group unfolds it, and another folds it again.
	if !strings.Contains(screen(m), "settings") {
		t.Fatalf("the team group did not unfold:\n%s", screen(m))
	}
	x, y = lineOf(t, m, "▾ requested from a team")
	mouseAt(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if strings.Contains(screen(m), "settings") {
		t.Fatalf("the team group did not fold:\n%s", screen(m))
	}
	mouseAt(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})

	x, y = lineOf(t, m, "Add retries")
	mouseAt(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if m.menu != nil {
		t.Fatal("a single click opened the menu")
	}
	mouseAt(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if p, _, _ := m.currentPR(); m.menu == nil || p.Key != key("acme", "api", 12) {
		t.Fatalf("menu = %v, cursor on %v; want the menu of acme/api#12", m.menu, p.Key)
	}
}

func TestClickPaneTab(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	x, y := lineOf(t, m, "diff (2)")
	mouseAt(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if m.pane.mode != paneDiff || m.dragging {
		t.Fatalf("mode = %v, dragging = %v; want the diff", m.pane.mode, m.dragging)
	}
	x, y = lineOf(t, m, "overview")
	mouseAt(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if m.pane.mode != paneOverview || m.dragging {
		t.Fatalf("mode = %v, dragging = %v; want the overview", m.pane.mode, m.dragging)
	}
}

type fakeLayout struct{ saved []int }

func (f *fakeLayout) SavePreviewHeight(h int) error {
	f.saved = append(f.saved, h)
	return nil
}

func TestDragPane(t *testing.T) {
	lay := &fakeLayout{}
	set := testSettings()
	set.Layout = lay
	m := New(fixture(), &fakeRunner{}, &fakeClaude{}, keymap.Default(), nil, set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	run(m, m.Init())

	_, y := lineOf(t, m, "╭")
	mouseAt(m, tea.MouseClickMsg{X: 60, Y: y, Button: tea.MouseLeft})
	mouseAt(m, tea.MouseMotionMsg{X: 60, Y: y - 4, Button: tea.MouseLeft})
	mouseAt(m, tea.MouseReleaseMsg{X: 60, Y: y - 4, Button: tea.MouseLeft})
	if m.pane.height != 14 || len(lay.saved) != 1 || lay.saved[0] != 14 {
		t.Fatalf("height = %d, saved = %v; want 14 kept", m.pane.height, lay.saved)
	}
	if _, top := lineOf(t, m, "╭"); top != y-4 {
		t.Fatalf("the pane's top is on line %d, want %d", top, y-4)
	}
	// The list keeps a third of the lines however far it is dragged.
	mouseAt(m, tea.MouseClickMsg{X: 60, Y: y - 4, Button: tea.MouseLeft})
	mouseAt(m, tea.MouseMotionMsg{X: 60, Y: 0, Button: tea.MouseLeft})
	mouseAt(m, tea.MouseReleaseMsg{X: 60, Y: 0, Button: tea.MouseLeft})
	if l := m.layout(); l.list < (l.list+l.pane)/3 {
		t.Fatalf("layout = %+v; the list has less than a third", l)
	}

	press(m, "-")
	if got := lay.saved[len(lay.saved)-1]; got != m.pane.height {
		t.Fatalf("saved %d after -, height is %d", got, m.pane.height)
	}
}

func TestPaneOpensAtDefaultHeight(t *testing.T) {
	// With the pane hidden at start and no height kept, opening it takes
	// the default height the app passes.
	set := testSettings()
	set.PreviewHeight, set.PaneHeight, set.DefaultPaneHeight = 0, 0, 9
	m := New(fixture(), &fakeRunner{}, &fakeClaude{}, keymap.Default(), nil, set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	run(m, m.Init())
	if m.pane.on {
		t.Fatal("the pane is shown at start")
	}
	press(m, "p")
	if !m.pane.on || m.pane.height != 9 {
		t.Fatalf("on = %v, height = %d; want shown at 9", m.pane.on, m.pane.height)
	}
}

func TestWheel(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	sel := m.tab().sel
	mouseAt(m, tea.MouseWheelMsg{X: 10, Y: 3, Button: tea.MouseWheelDown})
	if m.tab().sel != sel+1 {
		t.Fatalf("sel = %d, want %d", m.tab().sel, sel+1)
	}
	mouseAt(m, tea.MouseWheelMsg{X: 10, Y: 3, Button: tea.MouseWheelUp})
	_, y := lineOf(t, m, "╭")
	mouseAt(m, tea.MouseWheelMsg{X: 10, Y: y + 2, Button: tea.MouseWheelDown})
	if m.tab().sel != sel || m.pane.scroll != wheelLines {
		t.Fatalf("sel = %d, pane scroll = %d; want %d, %d", m.tab().sel, m.pane.scroll, sel, wheelLines)
	}
}

func TestMouseLeavesPopupsAlone(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "S")
	if m.picker == nil {
		t.Fatal("S did not open the picker")
	}
	sel := m.tab().sel
	x, y := lineOf(t, m, "3 involved")
	mouseAt(m, tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseLeft})
	mouseAt(m, tea.MouseWheelMsg{X: 10, Y: 3, Button: tea.MouseWheelDown})
	if m.cur != 0 || m.tab().sel != sel || m.picker == nil {
		t.Fatalf("tab = %d, sel = %d, picker = %v; the click went through the popup", m.cur, m.tab().sel, m.picker)
	}
}

func TestClickOpensInTheBrowser(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	m.set.Host = "github.example.com"
	click := func(s string) {
		t.Helper()
		x, y := lineOf(t, m, s)
		clickTwice(m, x+1, y)
	}
	click("@me")
	press(m, "b") // unfold the bots
	click("api#30")
	click("renovate")
	click("alice")
	want := []string{"https://github.example.com/me", "https://github.example.com/acme/api/pull/30", "https://github.example.com/apps/renovate", "https://github.example.com/alice"}
	if strings.Join(f.browsed, " ") != strings.Join(want, " ") {
		t.Fatalf("browsed %q, want %q", f.browsed, want)
	}
	if m.view != viewList {
		t.Fatal("a click on a link opened the pull request")
	}
}

func TestClickOnALinkWaitsForASecond(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	clock := m.set.Now()
	m.set.Now = func() time.Time { return clock }
	x, y := lineOf(t, m, "alice")
	click := func(x, y int) {
		t.Helper()
		mouseAt(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	}

	// One click shows "open?" over the link and opens nothing.
	click(x+1, y)
	if len(f.browsed) != 0 {
		t.Fatalf("one click opened %q", f.browsed)
	}
	if l := strings.Split(screen(m), "\n")[y]; !strings.Contains(l, "open?") || strings.Contains(l, "alice") {
		t.Fatalf("the link does not wait for a second click:\n%s", l)
	}
	// It keeps the background of the row it is on, the cursor's here.
	raw, _ := m.render()
	if want := m.st.peach.Bold(true).Background(m.st.sel.GetBackground()).Underline(true).Render("open?"); !strings.Contains(strings.Split(raw, "\n")[y], want) {
		t.Fatalf("%q is not drawn on the row's background:\n%q", "open", strings.Split(raw, "\n")[y])
	}

	// The wait ends, and the link is drawn again; a click then waits anew.
	clock = clock.Add(armedFor)
	mouseAt(m, disarmMsg{})
	if l := strings.Split(screen(m), "\n")[y]; !strings.Contains(l, "alice") {
		t.Fatalf("the link is not back after the wait:\n%s", l)
	}
	click(x+1, y)
	if len(f.browsed) != 0 {
		t.Fatalf("a click after the wait opened %q", f.browsed)
	}

	// A key press lets go of the link too.
	press(m, "j", "k")
	click(x+1, y)
	if len(f.browsed) != 0 {
		t.Fatalf("a click after a key opened %q", f.browsed)
	}

	// So does a click elsewhere: the title of the row is no link.
	tx, ty := lineOf(t, m, "Add retries")
	click(tx, ty)
	click(x+1, y)
	if len(f.browsed) != 0 {
		t.Fatalf("a click after one elsewhere opened %q", f.browsed)
	}

	// A second click in time opens the page, anywhere on the link.
	clock = clock.Add(armedFor - time.Millisecond)
	click(x+3, y)
	if want := "https://github.com/alice"; strings.Join(f.browsed, " ") != want {
		t.Fatalf("browsed %q, want %q", f.browsed, want)
	}
	if m.armed != nil {
		t.Fatal("the link still waits after it opened")
	}
}
