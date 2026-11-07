//go:build js && wasm

// The playground: gh-desk compiled to WebAssembly, run against the
// made-up GitHub of demo/internal/playground, in a terminal its page draws
// with xterm.js.
//
// The page calls ghDesk.start(cols, rows, write, opts) once the wasm is
// loaded: write gets what gh-desk writes to the terminal. opts.scenario
// names a scenario of demo/internal/playground to play once gh-desk has
// read its lists, or is empty; opts.from is the step to replay it from,
// the ones before typed fast, with the world hurried when opts.hurry.
// The page is told of its progress through opts.onReady(steps), each
// step {key, help, delay} with delay in ms, opts.onStep(i) just before
// step i is typed, and opts.onDone(); opts.onUnknown() is told instead when
// no scenario has the name.
//
// ghDesk.input(data) gives gh-desk what is typed, ghDesk.takeOver() stops
// the scenario when the visitor acts, ghDesk.pause(true|false) holds and
// lets go of it, ghDesk.seek(from) plays the scenario again from step
// from in a new world, in this same module, and ghDesk.resize(cols, rows)
// gives the terminal's new size. When gh-desk quits, the next key starts it again with a new world,
// without the scenario.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"syscall/js"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/babarot/gh-desk/demo/internal/playground"
	"github.com/babarot/gh-desk/internal/app"
	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui"
	"github.com/babarot/gh-desk/internal/tui/keymap"
	"github.com/babarot/gh-desk/internal/version"
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
	// stop stops the scenario playing, when one is.
	stop context.CancelFunc
	// pause holds the scenario playing.
	pause *pauser
	// quit ends the run, for a seek.
	quit context.CancelFunc
	// seek is the step a seek asked to play from, the last one winning.
	seek chan int
}

// play is what the page asked to play: the scenario, the step to replay
// it from, and what to tell the page of its progress.
type play struct {
	scenario string
	from     int
	// seek is set for a run a seek started: the world is hurried until
	// the step, since the page shows nothing of the way.
	seek bool
	ev   playground.Events
	// unknown is told that no scenario has the name asked for, so the page
	// does not wait for one.
	unknown js.Value
}

// playOf reads the options of ghDesk.start.
func playOf(opts js.Value) play {
	var pl play
	if opts.Type() != js.TypeObject {
		return pl
	}
	if v := opts.Get("scenario"); v.Type() == js.TypeString {
		pl.scenario = v.String()
	}
	if v := opts.Get("from"); v.Type() == js.TypeNumber {
		pl.from = v.Int()
	}
	pl.seek = opts.Get("hurry").Truthy()
	fn := func(name string) (js.Value, bool) {
		v := opts.Get(name)
		return v, v.Type() == js.TypeFunction
	}
	if f, ok := fn("onReady"); ok {
		pl.ev.Ready = func(steps []playground.Pressed) {
			arr := make([]any, len(steps))
			for i, p := range steps {
				arr[i] = map[string]any{"key": p.Key, "help": p.Help, "delay": p.Delay().Milliseconds()}
			}
			f.Invoke(arr)
		}
	}
	if f, ok := fn("onStep"); ok {
		pl.ev.Step = func(i int, _ playground.Pressed) { f.Invoke(i) }
	}
	if f, ok := fn("onDone"); ok {
		pl.ev.Done = func() { f.Invoke() }
	}
	if f, ok := fn("onUnknown"); ok {
		pl.unknown = f
	}
	return pl
}

// pauser hands the page's pause and play to a scenario playing, the last
// one asked winning: the page's call must return at once, and the
// scenario reads them only while it waits for a step.
type pauser struct {
	mu     sync.Mutex
	paused bool
	asked  chan struct{}
	out    chan bool
}

func newPauser() *pauser {
	p := &pauser{asked: make(chan struct{}, 1), out: make(chan bool)}
	go func() {
		for range p.asked {
			p.mu.Lock()
			v := p.paused
			p.mu.Unlock()
			p.out <- v
		}
	}()
	return p
}

func (p *pauser) set(v bool) {
	p.mu.Lock()
	p.paused = v
	p.mu.Unlock()
	select {
	case p.asked <- struct{}{}:
	default:
	}
}

func main() {
	t := &terminal{typed: make(chan struct{}, 1), pause: newPauser(), seek: make(chan int, 1)}
	type start struct {
		out  output
		play play
	}
	started := make(chan start, 1)
	api := js.Global().Get("Object").New()
	api.Set("start", js.FuncOf(func(_ js.Value, args []js.Value) any {
		t.mu.Lock()
		t.cols, t.rows = args[0].Int(), args[1].Int()
		t.mu.Unlock()
		st := start{out: output{args[2]}}
		if len(args) > 3 {
			st.play = playOf(args[3])
		}
		started <- st
		return nil
	}))
	// takeOver stops the scenario: the visitor pressed a key or clicked.
	// It is apart from input, which also carries the terminal's answers
	// to gh-desk's questions, such as its background color.
	api.Set("takeOver", js.FuncOf(func(js.Value, []js.Value) any {
		t.mu.Lock()
		if t.stop != nil {
			t.stop()
			t.stop = nil
		}
		t.mu.Unlock()
		return nil
	}))
	// seek ends the run and starts a new one, playing the scenario from
	// the step given; the page draws nothing of it until it gets there.
	api.Set("seek", js.FuncOf(func(_ js.Value, args []js.Value) any {
		from := 0
		if len(args) > 0 && args[0].Type() == js.TypeNumber {
			from = args[0].Int()
		}
		select {
		case <-t.seek:
		default:
		}
		t.seek <- from
		t.mu.Lock()
		if t.quit != nil {
			t.quit()
		}
		t.mu.Unlock()
		return nil
	}))
	api.Set("pause", js.FuncOf(func(_ js.Value, args []js.Value) any {
		t.pause.set(len(args) > 0 && args[0].Truthy())
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

	st := <-started
	out := st.out
	sc, ok := playground.FindScenario(st.play.scenario)
	if st.play.scenario != "" && !ok {
		js.Global().Get("console").Call("warn", "gh-desk: no scenario is named "+st.play.scenario)
		if st.play.unknown.Type() == js.TypeFunction {
			st.play.unknown.Invoke()
		}
	}
	pl := st.play
	for {
		var next *playground.Scenario
		if ok {
			// Played on the first run only: a run started again is the
			// visitor's, unless a seek asks for the scenario again.
			next, ok = &sc, false
		}
		err := t.run(out, next, pl)
		select {
		case from := <-t.seek:
			// A seek ended the run: the next one plays the scenario from
			// the step asked, with no word of the run that was ended.
			pl.from, pl.seek, ok = from, true, sc.Name != ""
			fmt.Fprint(out, "\x1b[2J\x1b[H")
			continue
		default:
		}
		if err != nil {
			fmt.Fprintf(out, "\r\ngh-desk: %v\r\n", err)
		}
		fmt.Fprint(out, "\r\n\x1b[2mgh-desk quit. Press any key to start it again.\x1b[0m\r\n")
		select {
		case <-t.typed:
			pl.from, pl.seek = 0, false
		case from := <-t.seek:
			pl.from, pl.seek, ok = from, true, sc.Name != ""
		}
		fmt.Fprint(out, "\x1b[2J\x1b[H")
	}
}

// run runs gh-desk until it quits, in a new world, playing sc when it is
// not nil.
func (t *terminal) run(out output, sc *playground.Scenario, pl play) error {
	// The config file a first launch writes, read without a disk.
	cfg, _, err := config.Parse("~/.config/gh-desk/config.toml", []byte(config.Template()))
	if err != nil {
		return err
	}
	set, km := app.Settings(cfg), keymap.Default()
	set.Version = version.String()
	w := playground.New(time.Now)
	set.Viewer = playground.Viewer
	set.Host = "github.com"
	set.CanWait = true
	t.mu.Lock()
	set.PaneHeight = playground.PaneHeight(t.rows)
	t.mu.Unlock()
	// There is no log file in the browser; the records are kept in memory,
	// for the TUI's log viewer.
	recs := logging.NewRecords(0, nil)
	log := logging.Memory(recs, slog.LevelInfo)
	set.LogRecords = recs
	// The world moves in seconds, so the runner looks often.
	rn := runner.New(w.Runner(), log, runner.Options{Settings: set.Plan, Viewer: playground.Viewer, CanWait: true,
		WaitTimeout: cfg.Checks.WaitTimeout.D, BlockedTimeout: cfg.Merge.BlockedTimeout.D,
		PollFast: 3 * time.Second, PollSlow: 5 * time.Second, Recheck: time.Second, RecheckFor: 10 * time.Second, Quiet: 3 * time.Second})
	m := tui.New(w.Source(), rn, w.Claude(), km, log, set)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.mu.Lock()
	t.quit = cancel
	in := newInput()
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out),
		tea.WithWindowSize(t.cols, t.rows), tea.WithColorProfile(colorprofile.TrueColor),
		tea.WithEnvironment([]string{"TERM=xterm-256color", "COLORTERM=truecolor"}), tea.WithoutSignals())
	t.program, t.in = p, in
	if sc != nil {
		var playing context.Context
		playing, t.stop = context.WithCancel(ctx)
		go func() {
			if err := sc.Play(playing, w, km, pl.from, pl.seek, t.pause.out, in.Write, pl.ev); err != nil && playing.Err() == nil {
				js.Global().Get("console").Call("warn", "gh-desk: "+err.Error())
			}
		}()
	}
	t.mu.Unlock()
	_, err = p.Run()
	t.mu.Lock()
	t.program, t.in, t.quit = nil, nil, nil
	if t.stop != nil {
		t.stop()
		t.stop = nil
	}
	t.mu.Unlock()
	cancel()
	rn.Wait()
	return err
}
