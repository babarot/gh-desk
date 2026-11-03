//go:build js && wasm

// The demo on the site: gh-desk compiled to WebAssembly, run against the
// made-up GitHub of package demo, in a terminal the page draws with
// xterm.js.
//
// The page calls ghDesk.start(cols, rows, write) once the wasm is loaded:
// write gets what gh-desk writes to the terminal. ghDesk.input(data) gives
// it what is typed, and ghDesk.resize(cols, rows) the terminal's new size.
// When gh-desk quits, the next key starts it again with a new world.
package main

import (
	"context"
	"fmt"
	"sync"
	"syscall/js"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/babarot/gh-desk/internal/app"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui"
	"github.com/babarot/gh-desk/site/demo"
)

// input is what is typed, waiting to be read. Writes never block: they
// come from a JavaScript event, which must return for anything else to
// run.
type input struct {
	mu   sync.Mutex
	cond *sync.Cond
	buf  []byte
}

func newInput() *input {
	in := &input{}
	in.cond = sync.NewCond(&in.mu)
	return in
}

func (in *input) Write(p []byte) {
	in.mu.Lock()
	in.buf = append(in.buf, p...)
	in.mu.Unlock()
	in.cond.Broadcast()
}

func (in *input) Read(p []byte) (int, error) {
	in.mu.Lock()
	defer in.mu.Unlock()
	for len(in.buf) == 0 {
		in.cond.Wait()
	}
	n := copy(p, in.buf)
	in.buf = in.buf[n:]
	return n, nil
}

// output hands what gh-desk writes to the page.
type output struct{ write js.Value }

func (o output) Write(p []byte) (int, error) {
	o.write.Invoke(string(p))
	return len(p), nil
}

// terminal is the page's terminal: its size, and whether gh-desk is
// running in it.
type terminal struct {
	mu         sync.Mutex
	cols, rows int
	program    *tea.Program
	// in is what gh-desk reads, new for each run: a run that quit may
	// still be reading the last one.
	in *input
	// typed is told of a key while gh-desk is not running.
	typed chan struct{}
}

func main() {
	t := &terminal{typed: make(chan struct{}, 1)}
	started := make(chan output, 1)
	api := js.Global().Get("Object").New()
	api.Set("start", js.FuncOf(func(_ js.Value, args []js.Value) any {
		t.mu.Lock()
		t.cols, t.rows = args[0].Int(), args[1].Int()
		t.mu.Unlock()
		started <- output{args[2]}
		return nil
	}))
	api.Set("input", js.FuncOf(func(_ js.Value, args []js.Value) any {
		t.mu.Lock()
		in := t.in
		t.mu.Unlock()
		if in != nil {
			in.Write([]byte(args[0].String()))
		} else {
			select {
			case t.typed <- struct{}{}:
			default:
			}
		}
		return nil
	}))
	api.Set("resize", js.FuncOf(func(_ js.Value, args []js.Value) any {
		t.mu.Lock()
		t.cols, t.rows = args[0].Int(), args[1].Int()
		p, w, h := t.program, t.cols, t.rows
		t.mu.Unlock()
		if p != nil {
			go p.Send(tea.WindowSizeMsg{Width: w, Height: h})
		}
		return nil
	}))
	js.Global().Set("ghDesk", api)
	js.Global().Call("dispatchEvent", js.Global().Get("Event").New("gh-desk-ready"))

	out := <-started
	for {
		if err := t.run(out); err != nil {
			fmt.Fprintf(out, "\r\ngh-desk: %v\r\n", err)
		}
		fmt.Fprint(out, "\r\n\x1b[2mgh-desk quit. Press any key to start it again.\x1b[0m\r\n")
		<-t.typed
		fmt.Fprint(out, "\x1b[2J\x1b[H")
	}
}

// run runs gh-desk until it quits, in a new world.
func (t *terminal) run(out output) error {
	set, km, cfg, err := app.Demo()
	if err != nil {
		return err
	}
	w := demo.New(time.Now)
	set.Viewer = demo.Viewer
	set.Host = "github.com"
	set.CanWait = true
	log := logging.Discard()
	// The world moves in seconds, so the runner looks often.
	rn := runner.New(w.Runner(), log, runner.Options{Settings: set.Plan, Viewer: demo.Viewer, CanWait: true,
		ChecksTimeout: cfg.Merge.ChecksTimeout.D, BlockedTimeout: cfg.Merge.BlockedTimeout.D,
		PollFast: 3 * time.Second, PollSlow: 5 * time.Second, Recheck: time.Second, RecheckFor: 10 * time.Second})
	m := tui.New(w.Source(), rn, w.Claude(), km, log, set)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.mu.Lock()
	in := newInput()
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out),
		tea.WithWindowSize(t.cols, t.rows), tea.WithColorProfile(colorprofile.TrueColor),
		tea.WithEnvironment([]string{"TERM=xterm-256color", "COLORTERM=truecolor"}), tea.WithoutSignals())
	t.program, t.in = p, in
	t.mu.Unlock()
	_, err = p.Run()
	t.mu.Lock()
	t.program, t.in = nil, nil
	t.mu.Unlock()
	cancel()
	rn.Wait()
	return err
}
