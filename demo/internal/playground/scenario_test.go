package playground

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/app"
	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// decode is what Bubble Tea reads from b, as the page's terminal sends it.
func decode(t *testing.T, b []byte) []tea.Msg {
	t.Helper()
	var d uv.EventDecoder
	var out []tea.Msg
	for len(b) > 0 {
		n, ev := d.Decode(b)
		if n == 0 {
			t.Fatalf("%q is not read", b)
		}
		b = b[n:]
		switch ev := ev.(type) {
		case uv.KeyPressEvent:
			out = append(out, tea.KeyPressMsg(ev))
		default:
			t.Fatalf("%q is read as %T", b, ev)
		}
	}
	return out
}

// TestEveryKeyEncodes checks that every key of the keymap, and the fixed
// ones, has bytes, and that Bubble Tea reads them back as that key.
func TestEveryKeyEncodes(t *testing.T) {
	keys := []string{keymap.Back, keymap.Interrupt}
	for _, d := range keymap.Defaults {
		keys = append(keys, d.Keys...)
	}
	for _, k := range keys {
		b, err := Encode(k)
		if err != nil {
			t.Error(err)
			continue
		}
		msgs := decode(t, b)
		if len(msgs) != 1 {
			t.Errorf("%s: %q is read as %d keys", k, b, len(msgs))
			continue
		}
		key, ok := msgs[0].(tea.KeyPressMsg)
		if !ok {
			t.Errorf("%s: %q is read as %T, not a key press", k, b, msgs[0])
			continue
		}
		if got := key.String(); got != k {
			t.Errorf("%s: %q is read as %s", k, b, got)
		}
	}
}

// TestScenariosPress checks that every step of every scenario has a key,
// and that the names are unique.
func TestScenariosPress(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Scenarios() {
		if seen[s.Name] {
			t.Errorf("two scenarios are named %s", s.Name)
		}
		seen[s.Name] = true
		if s.Title == "" || len(s.Steps) == 0 {
			t.Errorf("%s has no title or no steps", s.Name)
		}
		for i, st := range s.Steps {
			set := 0
			for _, v := range []string{string(st.Action), st.Key, st.Text} {
				if v != "" {
					set++
				}
			}
			if set != 1 {
				t.Errorf("%s, step %d: sets %d of action, key and text", s.Name, i, set)
			}
		}
		if _, err := s.Press(keymap.Default()); err != nil {
			t.Error(err)
		}
	}
}

// desk is gh-desk on the playground's world, as the site runs it, with
// every command carried out at once.
type desk struct {
	t *testing.T
	m *tui.Model
	w *World
}

func newDesk(t *testing.T) *desk {
	t.Helper()
	cfg, _, err := config.Parse("~/.config/gh-desk/config.toml", []byte(config.Template(app.KeysTemplate())))
	if err != nil {
		t.Fatal(err)
	}
	w, c := newWorld()
	set := app.Settings(cfg)
	set.Viewer, set.Host, set.CanWait = Viewer, "github.com", true
	set.Now = c.now
	set.Tick = func(time.Duration, tea.Msg) tea.Cmd { return nil }
	set.Lists, set.Chats = nil, nil
	set.PaneHeight = PaneHeight(45)
	rn := runner.New(w.Runner(), logging.Discard(), runner.Options{Settings: set.Plan, Viewer: Viewer, CanWait: true,
		ChecksTimeout: cfg.Merge.ChecksTimeout.D, BlockedTimeout: cfg.Merge.BlockedTimeout.D,
		PollFast: time.Millisecond, PollSlow: time.Millisecond, Recheck: time.Millisecond, RecheckFor: time.Millisecond})
	t.Cleanup(rn.Wait)
	d := &desk{t: t, m: tui.New(w.Source(), rn, w.Claude(), keymap.Default(), logging.Discard(), set), w: w}
	d.update(tea.WindowSizeMsg{Width: 140, Height: 45})
	d.run(d.m.Init())
	return d
}

func (d *desk) update(msg tea.Msg) {
	_, cmd := d.m.Update(msg)
	d.run(cmd)
}

// run carries out cmd and every command its messages lead to. A command
// that has not returned in a while waits for something that does not come
// here, such as a tick, and is left.
func (d *desk) run(cmd tea.Cmd) {
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		got := make(chan tea.Msg, 1)
		go func() { got <- c() }()
		var msg tea.Msg
		select {
		case msg = <-got:
		case <-time.After(2 * time.Second):
			continue
		}
		switch msg := msg.(type) {
		case nil, tea.QuitMsg:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		default:
			// tea.Sequence's message is a list of commands, of a type
			// of its own.
			if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeFor[tea.Cmd]() {
				for i := range v.Len() {
					// The element type was checked above, so this holds.
					if cmd, ok := reflect.TypeAssert[tea.Cmd](v.Index(i)); ok {
						queue = append(queue, cmd)
					}
				}
				continue
			}
			_, next := d.m.Update(msg)
			queue = append(queue, next)
		}
	}
}

func (d *desk) screen() string { return ansi.Strip(d.m.View().Content) }

// TestScenariosReachTheirScreen plays each scenario on the site's gh-desk
// and checks the screen it ends on.
func TestScenariosReachTheirScreen(t *testing.T) {
	want := map[string][]string{
		"tour":   {"acme/api#479", "Drop support for Go 1.22"},
		"diff":   {"acme/api#482", "PRIMARY KEY (subscription_id, event_id, attempt)"},
		"rules":  {"rule: dependency bots + recent · 5 PRs", "#486 fix(deps): update module golang.org/x/net to v0.33.0"},
		"merge":  {"1 review (6)", "▸ requested from a team 2"},
		"claude": {"claude on acme/api#482", "Neither blocks a merge"},
		"triage": {"you  merge the bots with passing checks", "enter apply"},
		"inbox":  {"acme/mobile#342", "mention"},
		"help":   {"keys · z fold", "the list and its preview"},
	}
	for _, s := range Scenarios() {
		t.Run(s.Name, func(t *testing.T) {
			d := newDesk(t)
			select {
			case <-d.w.Ready():
			default:
				t.Fatal("the world is not ready after the first reads")
			}
			steps, err := s.Press(keymap.Default())
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range steps {
				for _, msg := range decode(t, p.Bytes) {
					d.update(msg)
				}
			}
			got := d.screen()
			ws, ok := want[s.Name]
			if !ok {
				t.Fatalf("no screen is checked for %s", s.Name)
			}
			for _, w := range ws {
				if !strings.Contains(got, w) {
					t.Errorf("the screen has no %q:\n%s", w, got)
				}
			}
		})
	}
}

// TestWaitHoldsWhilePaused checks that the time a scenario is paused does
// not count toward the wait for its next step.
func TestWaitHoldsWhilePaused(t *testing.T) {
	paused := make(chan bool)
	done := make(chan time.Duration)
	start := time.Now()
	go func() {
		_ = wait(context.Background(), 60*time.Millisecond, paused)
		done <- time.Since(start)
	}()
	time.Sleep(20 * time.Millisecond)
	paused <- true
	time.Sleep(100 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("the wait ended while paused")
	default:
	}
	paused <- false
	if got := <-done; got < 150*time.Millisecond {
		t.Errorf("the wait took %s, less than its time and the pause", got)
	}
}

// TestWaitStopsWithItsContext checks that a scenario stops while it waits,
// paused or not, when the visitor takes over.
func TestWaitStopsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	paused := make(chan bool)
	errc := make(chan error)
	go func() { errc <- wait(ctx, time.Hour, paused) }()
	paused <- true
	cancel()
	if err := <-errc; err == nil {
		t.Error("the wait did not stop")
	}
}
