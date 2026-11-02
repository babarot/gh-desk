// Package chat holds the conversations with Claude about pull requests:
// what each one holds, keeping them between launches and between
// gh-triage, and their locks (ADR-031). How a conversation is drawn, what
// the keys do in it and which one is on screen are internal/tui's. Only
// this package changes a conversation; see ADR-033.
package chat

import (
	"cmp"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/babarot/gh-triage/internal/claude"
	"github.com/babarot/gh-triage/internal/logging"
	"github.com/babarot/gh-triage/internal/model"
)

// Store keeps the conversations with Claude between launches and between
// gh-triage, one a pull request, each with a lock held while Claude Code
// works on its session. See ADR-031.
type Store interface {
	// Load reads the conversation about k; ok is false when there is none.
	Load(k model.PRKey) (r Record, ok bool, err error)
	// Save replaces the conversation about r.Key whole.
	Save(r Record) error
	// Keys are the pull requests that have a conversation kept.
	Keys() ([]model.PRKey, error)
	// Lock takes the lock of k without waiting; held is false when
	// another gh-triage holds it, and who names that one when it can.
	Lock(k model.PRKey) (unlock func(), held bool, who string, err error)
}

// Record is a conversation as it is kept.
type Record struct {
	Key     model.PRKey
	Session string
	Started bool
	Dir     string
	// Head is the head of the pull request at the last question.
	Head  string
	Model string
	Cost  float64
	// Rev rises by one at each write, and never goes down while the
	// conversation is kept.
	Rev   int
	Turns []Turn
}

// Turn is a question or an answer.
type Turn struct {
	You   bool
	Text  string
	Steps []claude.Step
	Err   bool
	// Head is the head of the pull request a question was asked at.
	Head string
	At   time.Time
	// Cost is what an answer cost.
	Cost float64
}

// Info is what the pane says of a conversation kept: how many questions
// it has, and the head of the pull request at the last one.
type Info struct {
	Questions int
	Head      string
}

func (r Record) info() Info {
	n := 0
	for _, t := range r.Turns {
		if t.You {
			n++
		}
	}
	return Info{Questions: n, Head: r.Head}
}

// Status is where a conversation says what went wrong: the status line.
type Status interface {
	Fail(text string)
	Refuse(text string)
}

// Conversation is a conversation with Claude about one pull request. It
// stays when the screen is left, and goes on when it is opened again; when
// kept, also after gh-triage quits and in another gh-triage.
type Conversation struct {
	pr model.PR
	// session is made when a question starts it, as an ID Claude Code may
	// have taken is not used again.
	session string
	// dir is the worktree, once made.
	dir string
	// started is set once Claude Code has the session.
	started bool
	// head is the head of the pull request at the last question.
	head  string
	turns []Turn
	// running is set while a question is on its way, steps are what
	// Claude did for it so far, and since is when it was asked.
	running bool
	steps   []claude.Step
	since   time.Time
	cost    float64
	model   string

	// persist is set while the conversation is kept; a write that failed
	// clears it for this run. rev is the rev of the last write that
	// succeeded, 0 when none has.
	persist bool
	rev     int
	// lock releases the conversation's lock, held while Claude Code works
	// on its session: a question on its way, or Claude Code in the
	// terminal.
	lock func()
	// elsewhere names the gh-triage that holds the lock, when another one
	// does, as last found at probed.
	elsewhere string
	probed    time.Time
}

// PR is the pull request as the list had it when the conversation was
// last opened.
func (c *Conversation) PR() model.PR { return c.pr }

// Session is the Claude Code session, once a question has made one.
func (c *Conversation) Session() string { return c.session }

// Dir is the worktree Claude reads, once made.
func (c *Conversation) Dir() string { return c.dir }

// Started reports whether Claude Code has the session.
func (c *Conversation) Started() bool { return c.started }

// Head is the head of the pull request at the last question.
func (c *Conversation) Head() string { return c.head }

// Turns are the questions and answers so far, copied with the tools used
// for each, so they cannot be changed from outside.
func (c *Conversation) Turns() []Turn {
	ts := slices.Clone(c.turns)
	for i := range ts {
		ts[i].Steps = slices.Clone(ts[i].Steps)
	}
	return ts
}

// Running reports whether a question is on its way.
func (c *Conversation) Running() bool { return c.running }

// Steps are what Claude did so far for the question on its way.
func (c *Conversation) Steps() []claude.Step { return slices.Clone(c.steps) }

// Since is when the question on its way was asked.
func (c *Conversation) Since() time.Time { return c.since }

// Cost is what the conversation cost so far.
func (c *Conversation) Cost() float64 { return c.cost }

// Model is the model that answered last.
func (c *Conversation) Model() string { return c.model }

// Elsewhere names the gh-triage working on the conversation, when another
// one is, as last found at Probed.
func (c *Conversation) Elsewhere() string { return c.elsewhere }

// Probed is when the lock was last tried, to find Elsewhere.
func (c *Conversation) Probed() time.Time { return c.probed }

// SetPR takes the pull request as the list has it now, whose head may
// have moved.
func (c *Conversation) SetPR(p model.PR) { c.pr = p }

// AddStep adds what Claude did to the question on its way.
func (c *Conversation) AddStep(s claude.Step) { c.steps = append(c.steps, s) }

// Prepared takes the worktree made for the question on its way, and the
// new session when the one kept was started in another worktree; renewed
// is set then.
func (c *Conversation) Prepared(dir, session string) (renewed bool) {
	c.dir = dir
	if session == "" {
		return false
	}
	c.session, c.started = session, false
	return true
}

// Finish ends the question on its way, answered or not.
func (c *Conversation) Finish() { c.running = false }

// Answered takes what Claude said of the session: Claude Code keeps the
// session from the first answer on.
func (c *Conversation) Answered(a claude.Answer) {
	c.started = true
	c.cost += a.Cost
	if a.Model != "" {
		c.model = a.Model
	}
}

// Reply adds an answer, or what went wrong when err is set, after the
// tools Claude used for it.
func (c *Conversation) Reply(text string, err bool, cost float64, at time.Time) {
	c.turns = append(c.turns, Turn{Text: text, Steps: c.steps, Err: err, At: at, Cost: cost})
	c.steps = nil
}

// Unlock releases the conversation's lock, once Claude Code is done with
// its session.
func (c *Conversation) Unlock() {
	if c.lock != nil {
		c.lock()
		c.lock = nil
	}
}

func (c *Conversation) record() Record {
	return Record{Key: c.pr.Key, Session: c.session, Started: c.started, Dir: c.dir, Head: c.head,
		Model: c.model, Cost: c.cost, Rev: c.rev, Turns: slices.Clone(c.turns)}
}

// load puts what is kept of the conversation in c, leaving what belongs
// to this gh-triage (the lock, the question on its way).
func (c *Conversation) load(r Record) {
	c.session, c.started, c.dir, c.head, c.model, c.cost, c.rev = r.Session, r.Started, r.Dir, r.Head, r.Model, r.Cost, r.Rev
	c.turns = slices.Clone(r.Turns)
}

// State keeps the conversations and takes their locks, and moves a
// question along.
type State struct {
	// store keeps the conversations; nil keeps them only while gh-triage
	// runs.
	store  Store
	log    *slog.Logger
	now    func() time.Time
	status Status
	// info is what the pane says of each conversation kept, its questions
	// -1 until it is read; problem the conversations kept that could not
	// be read, said once.
	info    map[model.PRKey]Info
	problem map[model.PRKey]bool
	// asking counts the questions to Claude on their way, which quitting
	// waits for a moment, so Claude Code closes the sessions it keeps.
	asking *sync.WaitGroup
}

// NewState keeps the conversations in store, or only while gh-triage runs
// when store is nil.
func NewState(store Store, log *slog.Logger, now func() time.Time, status Status) *State {
	return &State{store: store, log: log, now: now, status: status,
		info: map[model.PRKey]Info{}, problem: map[model.PRKey]bool{}, asking: &sync.WaitGroup{}}
}

// New is a conversation about p, not yet asked anything.
func (s *State) New(p model.PR) *Conversation {
	return &Conversation{pr: p, persist: s.store != nil}
}

// Asking counts the questions on their way: one is added when a question
// is sent, and done when its answer or its error comes.
func (s *State) Asking() *sync.WaitGroup { return s.asking }

// kept reports whether the conversation is kept on disk.
func (s *State) kept(c *Conversation) bool { return s.store != nil && c.persist }

// Sync makes c what is kept of it, when what is kept is newer, unless this
// gh-triage holds its lock and is working on it: then what is kept has not
// changed.
func (s *State) Sync(c *Conversation) {
	if c.lock != nil {
		return
	}
	s.Reload(c)
}

// Reload makes c what is kept of it, when what is kept is newer: written by
// another gh-triage, or started over there. A conversation that was kept
// and is gone was removed by gh triage gc, and is dropped here too, so it
// is not written back. Right after its lock is taken, it reads what
// another gh-triage wrote before letting go.
func (s *State) Reload(c *Conversation) {
	if !s.kept(c) {
		return
	}
	k := c.pr.Key
	r, ok, err := s.store.Load(k)
	switch {
	case err != nil:
		// Read as none, and written over at the next question.
		if !s.problem[k] {
			s.problem[k] = true
			s.status.Fail(fmt.Sprintf("could not read the conversation kept of %s: %v", k, err))
		}
		s.log.Warn("could not read a conversation kept", logging.KeyPR, k.String(), "err", err.Error())
	case !ok:
		if c.rev > 0 {
			c.load(Record{Key: k})
			delete(s.info, k)
		}
	case r.Rev > c.rev || r.Session != c.session:
		c.load(r)
		s.info[k] = r.info()
	}
}

// Probe finds out whether another gh-triage is working on c's session, by
// taking its lock for a moment: a lock cannot be looked at otherwise.
func (s *State) Probe(c *Conversation) {
	c.elsewhere, c.probed = "", s.now()
	if s.store == nil || c.lock != nil {
		return
	}
	unlock, held, who, err := s.store.Lock(c.pr.Key)
	switch {
	case err != nil:
		s.log.Warn("could not try the lock of a conversation", logging.KeyPR, c.pr.Key.String(), "err", err.Error())
	case !held:
		c.elsewhere = cmp.Or(who, "another gh-triage")
	default:
		unlock()
	}
}

// Lock takes c's lock unless this gh-triage holds it. It reports false,
// having said why, when another gh-triage holds it. A lock that cannot be
// taken at all (the state directory cannot be written) leaves the
// conversation in this gh-triage alone, no longer kept.
func (s *State) Lock(c *Conversation) bool {
	if s.store == nil || c.lock != nil {
		return true
	}
	unlock, held, who, err := s.store.Lock(c.pr.Key)
	switch {
	case err != nil:
		s.log.Warn("could not take the lock of a conversation", logging.KeyPR, c.pr.Key.String(), "err", err.Error())
		s.stopKeeping(c, err)
		return true
	case !held:
		c.elsewhere = cmp.Or(who, "another gh-triage")
		s.status.Refuse("Claude is working on this conversation in another gh-triage (" + c.elsewhere + ")")
		return false
	}
	c.lock, c.elsewhere = unlock, ""
	return true
}

// save keeps c, under its lock. A write that fails stops keeping it for
// this run, so a later one cannot write over what another gh-triage wrote
// meanwhile.
func (s *State) save(c *Conversation) {
	if !s.kept(c) {
		return
	}
	r := c.record()
	r.Rev = c.rev + 1
	if err := s.store.Save(r); err != nil {
		s.log.Warn("could not keep a conversation", logging.KeyPR, c.pr.Key.String(), "err", err.Error())
		s.stopKeeping(c, err)
		return
	}
	s.log.Debug("conversation kept", logging.KeyPR, c.pr.Key.String(), "rev", r.Rev)
	c.rev = r.Rev
	s.info[c.pr.Key] = r.info()
}

func (s *State) stopKeeping(c *Conversation, err error) {
	if !c.persist {
		return
	}
	c.persist = false
	s.status.Fail(fmt.Sprintf("could not keep the conversation of %s: %v", c.pr.Key, err))
}

// LoadKeys finds the pull requests that have a conversation kept.
func (s *State) LoadKeys() {
	if s.store == nil {
		return
	}
	keys, err := s.store.Keys()
	if err != nil {
		s.log.Warn("could not list the conversations kept", "err", err.Error())
		return
	}
	for _, k := range keys {
		s.info[k] = Info{Questions: -1}
	}
}

// KeptInfo is what the pane says of the conversation kept about k, read
// the first time it is asked for; ok is false when it has no question.
func (s *State) KeptInfo(k model.PRKey) (Info, bool) {
	ci, ok := s.info[k]
	if !ok {
		return Info{}, false
	}
	if ci.Questions < 0 {
		r, found, err := s.store.Load(k)
		if err != nil || !found {
			delete(s.info, k)
			return Info{}, false
		}
		ci = r.info()
		s.info[k] = ci
	}
	return ci, ci.Questions > 0
}

// Begin puts a question on its way in c, under its lock, and keeps it
// before it is answered, so a question cut short by quitting is still
// there. It reports false, having said why, when it cannot be asked, and
// the head the last question was asked at when the head has moved since.
func (s *State) Begin(c *Conversation, shown string) (moved string, ok bool) {
	if c.running {
		s.status.Refuse("Claude is still answering")
		return "", false
	}
	if !s.Lock(c) {
		return "", false
	}
	s.Reload(c)
	if !c.started {
		c.session = claude.NewSession()
	}
	head := c.pr.HeadOID
	if c.started && c.head != "" && c.head != head {
		moved = c.head
	}
	c.turns = append(c.turns, Turn{You: true, Text: shown, Head: head, At: s.now()})
	c.head = head
	c.running, c.steps, c.since = true, nil, s.now()
	s.save(c)
	return moved, true
}

// Keep keeps c once its question is answered, or has failed, and frees
// the session for another gh-triage.
func (s *State) Keep(c *Conversation) {
	c.steps = nil
	s.save(c)
	c.Unlock()
}

// Restart starts c over, under its lock, which the first question takes
// on. What is kept is replaced rather than removed, so its rev goes on
// rising and another gh-triage sees its own as older; Claude Code keeps
// the old session. It reports false, having said why, when another
// gh-triage holds the lock.
func (s *State) Restart(c *Conversation) bool {
	if !s.Lock(c) {
		return false
	}
	s.Reload(c)
	c.load(Record{Key: c.pr.Key, Rev: c.rev})
	s.save(c)
	s.log.Info("conversation started over", logging.KeyPR, c.pr.Key.String())
	return true
}
