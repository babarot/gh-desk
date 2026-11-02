package tui

import (
	"cmp"
	"fmt"
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
		r.Turns = append(r.Turns, ChatTurn{You: t.you, Text: t.text, Steps: t.steps, Err: t.err, Head: t.head, At: t.at})
	}
	return r
}

// load puts what is kept of the conversation in c, leaving what belongs
// to this screen (the lock, the question on its way, the text typed).
func (c *chat) load(r ChatRecord) {
	c.session, c.started, c.dir, c.head, c.model, c.cost, c.rev = r.Session, r.Started, r.Dir, r.Head, r.Model, r.Cost, r.Rev
	c.turns = nil
	for _, t := range r.Turns {
		c.turns = append(c.turns, turn{you: t.You, text: t.Text, steps: t.Steps, err: t.Err, head: t.Head, at: t.At})
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

// kept reports whether the conversation is kept on disk.
func (m *Model) kept(c *chat) bool { return m.set.Chats != nil && c.persist }

// syncChat makes c what is kept of it, when what is kept is newer, unless
// this gh-triage holds its lock and is working on it: then what is kept
// has not changed.
func (m *Model) syncChat(c *chat) {
	if c.lock != nil {
		return
	}
	m.reloadChat(c)
}

// reloadChat makes c what is kept of it, when what is kept is newer:
// written by another gh-triage, or started over there. A conversation that
// was kept and is gone was removed by gh triage gc, and is dropped here
// too, so it is not written back. Right after its lock is taken, it reads
// what another gh-triage wrote before letting go.
func (m *Model) reloadChat(c *chat) {
	if !m.kept(c) {
		return
	}
	k := c.pr.Key
	r, ok, err := m.set.Chats.Load(k)
	switch {
	case err != nil:
		// Read as none, and written over at the next question.
		if !m.chatProblem[k] {
			m.chatProblem[k] = true
			m.fail(fmt.Sprintf("could not read the conversation kept of %s: %v", k, err))
		}
		m.log.Warn("could not read a conversation kept", logging.KeyPR, k.String(), "err", err.Error())
	case !ok:
		if c.rev > 0 {
			c.load(ChatRecord{Key: k})
			delete(m.chatInfo, k)
		}
	case r.Rev > c.rev || r.Session != c.session:
		c.load(r)
		m.chatInfo[k] = r.info()
	}
}

// probeChat finds out whether another gh-triage is working on c's session,
// by taking its lock for a moment: a lock cannot be looked at otherwise.
func (m *Model) probeChat(c *chat) {
	c.elsewhere, c.probed = "", m.set.Now()
	if m.set.Chats == nil || c.lock != nil {
		return
	}
	unlock, held, who, err := m.set.Chats.Lock(c.pr.Key)
	switch {
	case err != nil:
		m.log.Warn("could not try the lock of a conversation", logging.KeyPR, c.pr.Key.String(), "err", err.Error())
	case !held:
		c.elsewhere = cmp.Or(who, "another gh-triage")
	default:
		unlock()
	}
}

// lockChat takes c's lock unless this gh-triage holds it. It reports false,
// having said why, when another gh-triage holds it. A lock that cannot be
// taken at all (the state directory cannot be written) leaves the
// conversation in this gh-triage alone, no longer kept.
func (m *Model) lockChat(c *chat) bool {
	if m.set.Chats == nil || c.lock != nil {
		return true
	}
	unlock, held, who, err := m.set.Chats.Lock(c.pr.Key)
	switch {
	case err != nil:
		m.log.Warn("could not take the lock of a conversation", logging.KeyPR, c.pr.Key.String(), "err", err.Error())
		m.stopKeeping(c, err)
		return true
	case !held:
		c.elsewhere = cmp.Or(who, "another gh-triage")
		m.refuse("Claude is working on this conversation in another gh-triage (" + c.elsewhere + ")")
		return false
	}
	c.lock, c.elsewhere = unlock, ""
	return true
}

// unlockChat releases c's lock.
func (c *chat) unlockChat() {
	if c.lock != nil {
		c.lock()
		c.lock = nil
	}
}

// saveChat keeps c, under its lock. A write that fails stops keeping it
// for this run, so a later one cannot write over what another gh-triage
// wrote meanwhile.
func (m *Model) saveChat(c *chat) {
	if !m.kept(c) {
		return
	}
	r := c.record()
	r.Rev = c.rev + 1
	if err := m.set.Chats.Save(r); err != nil {
		m.log.Warn("could not keep a conversation", logging.KeyPR, c.pr.Key.String(), "err", err.Error())
		m.stopKeeping(c, err)
		return
	}
	m.log.Debug("conversation kept", logging.KeyPR, c.pr.Key.String(), "rev", r.Rev)
	c.rev = r.Rev
	m.chatInfo[c.pr.Key] = r.info()
}

func (m *Model) stopKeeping(c *chat, err error) {
	if !c.persist {
		return
	}
	c.persist = false
	m.fail(fmt.Sprintf("could not keep the conversation of %s: %v", c.pr.Key, err))
}

// loadChatKeys finds the pull requests that have a conversation kept.
func (m *Model) loadChatKeys() {
	if m.set.Chats == nil {
		return
	}
	keys, err := m.set.Chats.Keys()
	if err != nil {
		m.log.Warn("could not list the conversations kept", "err", err.Error())
		return
	}
	for _, k := range keys {
		m.chatInfo[k] = chatInfo{questions: -1}
	}
}

// keptInfo is what the pane says of the conversation kept about k, read
// the first time it is asked for.
func (m *Model) keptInfo(k model.PRKey) (chatInfo, bool) {
	ci, ok := m.chatInfo[k]
	if !ok {
		return chatInfo{}, false
	}
	if ci.questions < 0 {
		r, found, err := m.set.Chats.Load(k)
		if err != nil || !found {
			delete(m.chatInfo, k)
			return chatInfo{}, false
		}
		ci = r.info()
		m.chatInfo[k] = ci
	}
	return ci, ci.questions > 0
}

// watchEvery is how often a conversation on screen is read again while
// another gh-triage works on it. A constant: reading is a stat and a small
// file.
const watchEvery = time.Second

// watchChat reads the conversation on screen again while another
// gh-triage works on it, as the spinner turns, so its answer shows when
// it comes. It only reads; nothing is written on a timer.
func (m *Model) watchChat() {
	c := m.chat()
	if m.view != viewChat || c == nil || c.elsewhere == "" || m.set.Now().Sub(c.probed) < watchEvery {
		return
	}
	// Probed first: found free, the reading after it has the answer.
	m.probeChat(c)
	m.syncChat(c)
}
