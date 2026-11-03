package playground

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// A scenario takes the playground to one screen, so the docs can link to
// that screen and embed it: the page is opened with ?scenario=<name>, and
// the steps are typed into gh-desk one by one, at a pace a visitor can
// follow, as if pressed. The visitor sees every key, and can take over at
// any point.
//
// A step names an action, not a key: it is pressed with the action's
// first key in the keymap, so a change of the default keys moves the
// scenarios with it.

// Scenario is a named way to a screen.
type Scenario struct {
	Name string
	// Title says in a line what the scenario shows.
	Title string
	Steps []Step
}

// Step is one thing typed. Exactly one of Action, Key and Text is set.
type Step struct {
	// Action is pressed with its first key.
	Action keymap.Action
	// Key is a key no action is bound to, as keymap.Back.
	Key string
	// Text is typed as it is, into a text box.
	Text string
	// Wait is how long to wait before the step, besides the pace, for
	// what the step before started: an answer from Claude, a popup that
	// reads before it opens. It is waited for even when the step is typed
	// fast, on the way to a later one. It is negative for a step that
	// follows the one before quickly, as a key pressed again.
	Wait time.Duration
	// Hold is a moment for the visitor to read what the step before
	// showed; a replay on its way to a later step skips it.
	Hold time.Duration
}

// Scenarios are the scenarios the page can be opened with.
func Scenarios() []Scenario {
	return []Scenario{
		// The diff of acme/api#482 opens on all its three files: read down,
		// then file by file (the change, its test, the migration), and
		// back and forth between the last two.
		{Name: "diff", Title: "The diff of a pull request, file by file", Steps: []Step{
			{Action: "preview_tab"},
			{Action: "preview_down", Hold: time.Second},
			{Action: "preview_down", Wait: -scroll},
			{Action: "preview_down", Wait: -scroll},
			{Action: "next_file", Hold: time.Second},
			{Action: "preview_down", Hold: time.Second},
			{Action: "preview_down", Wait: -scroll},
			{Action: "next_file", Hold: time.Second},
			{Action: "preview_down", Hold: time.Second},
			{Action: "preview_down", Wait: -scroll},
			{Action: "next_file", Hold: time.Second},
			{Action: "prev_file", Hold: time.Second},
			{Action: "next_file"},
		}},
		// A walk round the screen as it starts: the tabs, then a pull
		// request read from the list, its diff and back. The docs show it
		// where they show gh-desk as a whole.
		{Name: "tour", Title: "A walk round gh-desk as it starts", Steps: []Step{
			{Action: "tab_2", Hold: time.Second},
			{Action: "tab_3"},
			{Action: "tab_4"},
			{Action: "tab_1", Hold: time.Second},
			{Action: "down"},
			{Action: "down", Wait: -scroll},
			{Action: "preview_tab", Hold: time.Second},
			{Action: "next_file", Hold: time.Second},
			{Action: "preview_down"},
			{Action: "preview_tab", Hold: time.Second},
		}},
		{Name: "rules", Title: "The rules that select pull requests", Steps: []Step{
			{Action: "select_rule"},
		}},
		// The rules are those of the config file a first launch writes:
		// "dependency bots" is the third.
		{Name: "merge", Title: "The dependency updates selected by a rule, about to be merged", Steps: []Step{
			{Action: "fold_bots"},
			{Action: "select_rule"},
			{Action: "down"},
			{Action: "down"},
			{Action: "choose"},
			{Action: "merge", Wait: time.Second},
		}},
		// i opens the conversation suggesting a review, which tab fills in.
		{Name: "claude", Title: "Claude's review of a pull request", Steps: []Step{
			{Action: "ask_claude"},
			{Action: "fill_suggestion", Wait: time.Second},
			{Action: "accept"},
		}},
		{Name: "triage", Title: "A plan from Claude for what to merge", Steps: []Step{
			{Action: "triage"},
			{Text: "merge the bots with passing checks"},
			{Action: "accept"},
		}},
		{Name: "inbox", Title: "The pull requests in your notifications", Steps: []Step{
			{Action: "tab_4"},
		}},
		{Name: "help", Title: "Every key of the list", Steps: []Step{
			{Action: "help"},
		}},
	}
}

// FindScenario is the scenario named name.
func FindScenario(name string) (Scenario, bool) {
	for _, s := range Scenarios() {
		if s.Name == name {
			return s, true
		}
	}
	return Scenario{}, false
}

// Pressed is a step as it is typed: the bytes a terminal sends, and what
// the page shows of it.
type Pressed struct {
	Bytes []byte
	// Key is the key as the hints write it, or the text typed in quotes.
	Key string
	// Help is what the action does, as the hints say; empty for a key no
	// action is bound to and for text.
	Help string
	Wait time.Duration
	Hold time.Duration
}

// Press turns the steps into what is typed, with the keys of km.
func (s Scenario) Press(km keymap.Map) ([]Pressed, error) {
	var out []Pressed
	for i, st := range s.Steps {
		var p Pressed
		switch {
		case st.Action != "":
			ks := km.Keys(st.Action)
			if len(ks) == 0 {
				return nil, fmt.Errorf("scenario %s, step %d: %q has no key", s.Name, i, st.Action)
			}
			b, err := Encode(ks[0])
			if err != nil {
				return nil, fmt.Errorf("scenario %s, step %d: %w", s.Name, i, err)
			}
			p = Pressed{Bytes: b, Key: ks[0], Help: km.Help(st.Action)}
		case st.Key != "":
			b, err := Encode(st.Key)
			if err != nil {
				return nil, fmt.Errorf("scenario %s, step %d: %w", s.Name, i, err)
			}
			p = Pressed{Bytes: b, Key: st.Key, Help: keymap.Fixed[st.Key]}
		default:
			p = Pressed{Bytes: []byte(st.Text), Key: fmt.Sprintf("%q", st.Text)}
		}
		p.Wait, p.Hold = st.Wait, st.Hold
		out = append(out, p)
	}
	return out, nil
}

// named are the bytes a terminal sends for the keys that are not a
// character, written as Bubble Tea reports them.
var named = map[string]string{
	"enter":     "\r",
	"tab":       "\t",
	"shift+tab": "\x1b[Z",
	"space":     " ",
	"esc":       "\x1b",
	"backspace": "\x7f",
	"up":        "\x1b[A",
	"down":      "\x1b[B",
	"right":     "\x1b[C",
	"left":      "\x1b[D",
	"home":      "\x1b[H",
	"end":       "\x1b[F",
	"pgup":      "\x1b[5~",
	"pgdown":    "\x1b[6~",
}

// Encode is what a terminal sends when key, as Bubble Tea writes a key
// press, is pressed.
func Encode(key string) ([]byte, error) {
	if s, ok := named[key]; ok {
		return []byte(s), nil
	}
	if c, ok := strings.CutPrefix(key, "ctrl+"); ok && len(c) == 1 && c[0] >= 'a' && c[0] <= 'z' {
		return []byte{c[0] - 'a' + 1}, nil
	}
	if r := []rune(key); len(r) == 1 && r[0] > ' ' && r[0] != 0x7f {
		return []byte(key), nil
	}
	return nil, fmt.Errorf("no bytes for the key %q", key)
}

// Pace is the time between two steps, for a visitor to see each.
const Pace = 700 * time.Millisecond

// scroll is taken from the pace of a key pressed again and again, such as
// scrolling, so it reads as one movement rather than separate steps.
const scroll = 400 * time.Millisecond

// FastPace is the pace of the steps a replay goes through on its way to
// the step it starts at, so that the screen gets there at once while every
// step is still typed as it was: time for the TUI to take the key. A step
// that waits for something the world does (an answer, a popup that reads)
// is given FastWait more; the world is hurried meanwhile, so it answers at
// once.
const (
	FastPace = 40 * time.Millisecond
	FastWait = 120 * time.Millisecond
)

// Delay is how long the step waits after the one before: the pace, its
// own wait, and the moment to read the one before.
func (p Pressed) Delay() time.Duration { return Pace + p.Wait + p.Hold }

// Events are what a playing scenario tells the page as it goes, for its
// progress bar and its caption. A nil func is not called.
type Events struct {
	// Ready is told once the world has answered the first reads of the
	// lists: the steps start from here, with these delays.
	Ready func(steps []Pressed)
	// Step is told of step i just before it is typed.
	Step func(i int, p Pressed)
	// Done is told after the last step is typed.
	Done func()
}

// Play types the steps through typed once the world has answered the first
// reads of the lists. The steps before from are typed at FastPace, to
// replay the scenario from step from; the rest at their Delay. With
// hurried, the world answers at once until step from, the first reads
// too: a seek, which the page shows only when it gets there. A value on
// paused holds (true) or lets go (false) the wait for the next step, as a
// video is paused. Play stops when ctx is done: when the visitor takes
// over, or the page plays it again.
func (s Scenario) Play(ctx context.Context, w *World, km keymap.Map, from int, hurried bool, paused <-chan bool, typed func([]byte), ev Events) error {
	steps, err := s.Press(km)
	if err != nil {
		return err
	}
	w.Hurry(hurried)
	defer w.Hurry(false)
	select {
	case <-w.Ready():
	case <-ctx.Done():
		return ctx.Err()
	}
	if from == 0 {
		w.Hurry(false)
	}
	if ev.Ready != nil {
		ev.Ready(steps)
	}
	for i, p := range steps {
		d := p.Delay()
		switch {
		case i < from && p.Wait > 0:
			d = FastPace + FastWait
		case i < from:
			d = FastPace
		case i == from:
			w.Hurry(false)
		}
		if err := wait(ctx, d, paused); err != nil {
			return err
		}
		if ev.Step != nil {
			ev.Step(i, p)
		}
		typed(p.Bytes)
	}
	if ev.Done != nil {
		ev.Done()
	}
	return nil
}

// wait waits d, not counting the time it is held by paused.
func wait(ctx context.Context, d time.Duration, paused <-chan bool) error {
	t := time.NewTimer(d)
	defer t.Stop()
	end, held := time.Now().Add(d), false
	var left time.Duration
	for {
		var fire <-chan time.Time
		if !held {
			fire = t.C
		}
		select {
		case <-fire:
			return nil
		case p := <-paused:
			switch {
			case p && !held:
				held, left = true, time.Until(end)
				t.Stop()
			case !p && held:
				held, end = false, time.Now().Add(left)
				t.Reset(left)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
