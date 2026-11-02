package tui

import (
	"cmp"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/babarot/gh-triage/internal/claude"
	"github.com/babarot/gh-triage/internal/logging"
	"github.com/babarot/gh-triage/internal/model"
)

// Chats keep the conversations with Claude between launches and between
// gh-triage, one a pull request, each with a lock held while Claude Code
// works on its session. See ADR-031.
type Chats interface {
	// Load reads the conversation about k; ok is false when there is none.
	Load(k model.PRKey) (r ChatRecord, ok bool, err error)
	// Save replaces the conversation about r.Key whole.
	Save(r ChatRecord) error
	// Keys are the pull requests that have a conversation kept.
	Keys() ([]model.PRKey, error)
	// Lock takes the lock of k without waiting; held is false when
	// another gh-triage holds it, and who names that one when it can.
	Lock(k model.PRKey) (unlock func(), held bool, who string, err error)
}

// ChatRecord is a conversation as it is kept.
type ChatRecord struct {
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
	Turns []ChatTurn
}

// ChatTurn is a question or an answer as it is kept.
type ChatTurn struct {
	You   bool
	Text  string
	Steps []claude.Step
	Err   bool
	Head  string
	At    time.Time
	Cost  float64
}

// chatInfo is what the pane says of a conversation kept.
type chatInfo struct {
	questions int
	head      string
}

func (c *chat) record() ChatRecord {
	r := ChatRecord{Key: c.pr.Key, Session: c.session, Started: c.started, Dir: c.dir, Head: c.head,
		Model: c.model, Cost: c.cost, Rev: c.rev}
	for _, t := range c.turns {
		r.Turns = append(r.Turns, ChatTurn{You: t.you, Text: t.text, Steps: t.steps, Err: t.err, Head: t.head, At: t.at, Cost: t.cost})
	}
	return r
}

// load puts what is kept of the conversation in c, leaving what belongs
// to this screen (the lock, the question on its way, the text typed).
func (c *chat) load(r ChatRecord) {
	c.session, c.started, c.dir, c.head, c.model, c.cost, c.rev = r.Session, r.Started, r.Dir, r.Head, r.Model, r.Cost, r.Rev
	c.turns = nil
	for _, t := range r.Turns {
		c.turns = append(c.turns, turn{you: t.You, text: t.Text, steps: t.Steps, err: t.Err, head: t.Head, at: t.At, cost: t.Cost})
	}
}

func (r ChatRecord) info() chatInfo {
	n := 0
	for _, t := range r.Turns {
		if t.You {
			n++
		}
	}
	return chatInfo{questions: n, head: r.Head}
}

// unlockChat releases c's lock.
func (c *chat) unlockChat() {
	if c.lock != nil {
		c.lock()
		c.lock = nil
	}
}

// answered takes what Claude said of the session: Claude Code keeps the
// session from the first answer on.
func (c *chat) answered(a claude.Answer) {
	c.started = true
	c.cost += a.Cost
	if a.Model != "" {
		c.model = a.Model
	}
}

// reply adds an answer, or what went wrong when err is set, after the
// tools Claude used for it.
func (c *chat) reply(text string, err bool, cost float64, at time.Time) {
	c.turns = append(c.turns, turn{text: text, steps: c.steps, err: err, at: at, cost: cost})
	c.steps = nil
}

// chatStatus is where the conversations say what went wrong: the status
// line.
type chatStatus interface {
	fail(text string)
	refuse(text string)
}

// chatState is the conversations with Claude in this gh-triage and what is
// known of those kept: what they hold, keeping them and their locks. How
// they are drawn and what the keys do in them is the Model's.
type chatState struct {
	// store keeps the conversations; nil keeps them only while gh-triage
	// runs.
	store  Chats
	log    *slog.Logger
	now    func() time.Time
	status chatStatus
	// all are the conversations, by pull request; key is the one on
	// screen.
	all map[model.PRKey]*chat
	key model.PRKey
	// info is what the pane says of each conversation kept, its questions
	// -1 until it is read; problem the conversations kept that could not
	// be read, said once.
	info    map[model.PRKey]chatInfo
	problem map[model.PRKey]bool
	// asking counts the questions to Claude on their way, which quitting
	// waits for a moment, so Claude Code closes the sessions it keeps.
	asking *sync.WaitGroup
}

func newChatState(store Chats, log *slog.Logger, now func() time.Time, status chatStatus) *chatState {
	return &chatState{store: store, log: log, now: now, status: status, all: map[model.PRKey]*chat{},
		info: map[model.PRKey]chatInfo{}, problem: map[model.PRKey]bool{}, asking: &sync.WaitGroup{}}
}

// current is the conversation on screen, nil when none was opened.
func (s *chatState) current() *chat { return s.all[s.key] }

// kept reports whether the conversation is kept on disk.
func (s *chatState) kept(c *chat) bool { return s.store != nil && c.persist }

// sync makes c what is kept of it, when what is kept is newer, unless this
// gh-triage holds its lock and is working on it: then what is kept has not
// changed.
func (s *chatState) sync(c *chat) {
	if c.lock != nil {
		return
	}
	s.reload(c)
}

// reload makes c what is kept of it, when what is kept is newer: written by
// another gh-triage, or started over there. A conversation that was kept
// and is gone was removed by gh triage gc, and is dropped here too, so it
// is not written back. Right after its lock is taken, it reads what
// another gh-triage wrote before letting go.
func (s *chatState) reload(c *chat) {
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
			s.status.fail(fmt.Sprintf("could not read the conversation kept of %s: %v", k, err))
		}
		s.log.Warn("could not read a conversation kept", logging.KeyPR, k.String(), "err", err.Error())
	case !ok:
		if c.rev > 0 {
			c.load(ChatRecord{Key: k})
			delete(s.info, k)
		}
	case r.Rev > c.rev || r.Session != c.session:
		c.load(r)
		s.info[k] = r.info()
	}
}

// probe finds out whether another gh-triage is working on c's session, by
// taking its lock for a moment: a lock cannot be looked at otherwise.
func (s *chatState) probe(c *chat) {
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

// lock takes c's lock unless this gh-triage holds it. It reports false,
// having said why, when another gh-triage holds it. A lock that cannot be
// taken at all (the state directory cannot be written) leaves the
// conversation in this gh-triage alone, no longer kept.
func (s *chatState) lock(c *chat) bool {
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
		s.status.refuse("Claude is working on this conversation in another gh-triage (" + c.elsewhere + ")")
		return false
	}
	c.lock, c.elsewhere = unlock, ""
	return true
}

// unlock releases the lock of the conversation about k, once Claude Code
// in the terminal is done with it.
func (s *chatState) unlock(k model.PRKey) {
	if c, ok := s.all[k]; ok {
		c.unlockChat()
	}
}

// save keeps c, under its lock. A write that fails stops keeping it for
// this run, so a later one cannot write over what another gh-triage wrote
// meanwhile.
func (s *chatState) save(c *chat) {
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

func (s *chatState) stopKeeping(c *chat, err error) {
	if !c.persist {
		return
	}
	c.persist = false
	s.status.fail(fmt.Sprintf("could not keep the conversation of %s: %v", c.pr.Key, err))
}

// loadKeys finds the pull requests that have a conversation kept.
func (s *chatState) loadKeys() {
	if s.store == nil {
		return
	}
	keys, err := s.store.Keys()
	if err != nil {
		s.log.Warn("could not list the conversations kept", "err", err.Error())
		return
	}
	for _, k := range keys {
		s.info[k] = chatInfo{questions: -1}
	}
}

// keptInfo is what the pane says of the conversation kept about k, read
// the first time it is asked for.
func (s *chatState) keptInfo(k model.PRKey) (chatInfo, bool) {
	ci, ok := s.info[k]
	if !ok {
		return chatInfo{}, false
	}
	if ci.questions < 0 {
		r, found, err := s.store.Load(k)
		if err != nil || !found {
			delete(s.info, k)
			return chatInfo{}, false
		}
		ci = r.info()
		s.info[k] = ci
	}
	return ci, ci.questions > 0
}

// begin puts a question on its way in c, under its lock, and keeps it
// before it is answered, so a question cut short by quitting is still
// there. It reports false, having said why, when it cannot be asked, and
// the head the last question was asked at when the head has moved since.
func (s *chatState) begin(c *chat, shown string) (moved string, ok bool) {
	if c.running {
		s.status.refuse("Claude is still answering")
		return "", false
	}
	if !s.lock(c) {
		return "", false
	}
	s.reload(c)
	if !c.started {
		c.session = claude.NewSession()
	}
	head := c.pr.HeadOID
	if c.started && c.head != "" && c.head != head {
		moved = c.head
	}
	c.turns = append(c.turns, turn{you: true, text: shown, head: head, at: s.now()})
	c.head = head
	c.running, c.steps, c.scroll, c.since = true, nil, 0, s.now()
	s.save(c)
	return moved, true
}

// step adds what Claude did to the question on its way about k.
func (s *chatState) step(k model.PRKey, st claude.Step) {
	if c, ok := s.all[k]; ok {
		c.steps = append(c.steps, st)
	}
}

// prepared takes the worktree made for the question about k, and the new
// session when the one kept was started in another worktree; renewed is
// set then.
func (s *chatState) prepared(k model.PRKey, dir, session string) (renewed bool) {
	c, ok := s.all[k]
	if !ok {
		return false
	}
	c.dir = dir
	if session == "" {
		return false
	}
	c.session, c.started = session, false
	return true
}

// end ends the question on its way in c: kept with its answer, the
// session is then free for another gh-triage.
func (s *chatState) end(c *chat) {
	s.save(c)
	c.unlockChat()
}

// restart starts c over, under its lock, which the first question takes
// on. What is kept is replaced rather than removed, so its rev goes on
// rising and another gh-triage sees its own as older; Claude Code keeps
// the old session. It reports false, having said why, when another
// gh-triage holds the lock.
func (s *chatState) restart(c *chat) bool {
	if !s.lock(c) {
		return false
	}
	s.reload(c)
	c.load(ChatRecord{Key: c.pr.Key, Rev: c.rev})
	c.draft, c.scroll = nil, 0
	s.save(c)
	s.log.Info("conversation started over", logging.KeyPR, c.pr.Key.String())
	return true
}

// answering are the pull requests Claude is answering about, in order,
// leaving out shown, the one on screen when there is one.
func (s *chatState) answering(shown *model.PRKey) []model.PRKey {
	var keys []model.PRKey
	for k, c := range s.all {
		if c.running && (shown == nil || *shown != k) {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(a, b model.PRKey) int { return strings.Compare(a.String(), b.String()) })
	return keys
}
