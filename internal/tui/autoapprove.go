package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
)

// AutoApprove is a rule that approves the pull requests its search finds
// and it matches, without a confirm, while gh-desk runs (ADR-093).
type AutoApprove struct {
	Rule plan.Rule
	// Query is the search the pull requests are found by.
	Query string
	// DryRun only tells what the rule would approve.
	DryRun bool
}

// How many pull requests a rule's search reads, in pages of autoPage. A
// constant (ADR-014): a search that finds more is asked to be narrowed.
const (
	autoPage = 50
	autoMax  = 200
)

// autoState is what auto-approve keeps for the session.
type autoState struct {
	// running is set while a reading goes on; another is not started.
	running bool
	// tried are the heads approved, or told of for a dry run, once.
	tried map[autoTry]bool
	// toldMany are the rules whose search found more than autoMax, told
	// once; toldOff is set once it was told that this gh-desk does not
	// approve by rules.
	toldMany map[int]bool
	toldOff  bool
}

type autoTry struct {
	key  model.PRKey
	head string
}

// autoFound is what a rule's search found.
type autoFound struct {
	prs   []model.PR
	total int
}

// autoReadMsg is the searches of the rules read, with the heavy fields
// and the bodies their rules need.
type autoReadMsg struct {
	found  []autoFound
	bodies map[string]string
	err    error
}

// autoInfoMsg is what deciding the approvals needs of the pull requests
// the rules may approve.
type autoInfoMsg struct {
	cands []autoCand
	info  map[string]model.MergeInfo
	err   error
}

// autoCand is a pull request rules may approve, with each rule that
// says Yes or cannot tell yet, in the order written: one that cannot tell
// may say No once the checks are read with their details, and the next
// one is asked then.
type autoCand struct {
	pr    model.PR
	rules []int
	// body is its description, when a rule reads it.
	body string
}

// autoApprove reads the rules' searches, unless one is being read. Only
// the gh-desk that holds the record of pending merges approves; another
// says once why it does not.
func (m *Model) autoApprove() tea.Cmd {
	if len(m.set.AutoApprove) == 0 || m.viewer == "" || m.auto.running {
		return nil
	}
	if !m.set.CanWait {
		if m.auto.toldOff {
			return nil
		}
		m.auto.toldOff = true
		why := "another gh-desk holds the record of pending merges"
		if m.set.StoreProblem != "" {
			why = m.set.StoreProblem
		}
		m.log.Info("auto-approve off", logging.KeyOp, "auto_approve", "why", why)
		return m.pushNote("–", "auto-approve is off in this gh-desk: "+why, nil)
	}
	m.auto.running = true
	rules := slices.Clone(m.set.AutoApprove)
	return func() tea.Msg { return m.readAuto(rules) }
}

// readAuto reads each rule's search, page by page up to autoMax, then the
// heavy fields of what it found and the bodies a rule on packages needs.
func (m *Model) readAuto(rules []AutoApprove) autoReadMsg {
	msg := autoReadMsg{found: make([]autoFound, len(rules))}
	var ids []string
	var bodies []string
	for i, r := range rules {
		after := ""
		for {
			prs, total, next, _, err := m.src.SearchPage(m.ctx, r.Query, autoPage, after)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.found[i].prs = append(msg.found[i].prs, prs...)
			msg.found[i].total = total
			if next == "" || len(msg.found[i].prs) >= autoMax {
				break
			}
			after = next
		}
		if len(msg.found[i].prs) > autoMax {
			msg.found[i].prs = msg.found[i].prs[:autoMax]
		}
		for _, p := range msg.found[i].prs {
			ids = append(ids, p.ID)
			if r.Rule.NeedsBody() {
				bodies = append(bodies, p.ID)
			}
		}
	}
	ids = uniq(ids)
	if len(ids) > 0 {
		heavy, err := m.src.Heavy(m.ctx, ids)
		if err != nil {
			msg.err = err
			return msg
		}
		for i := range msg.found {
			for j := range msg.found[i].prs {
				if h, ok := heavy[msg.found[i].prs[j].ID]; ok {
					msg.found[i].prs[j].SetHeavy(h)
				}
			}
		}
	}
	if bodies = uniq(bodies); len(bodies) > 0 {
		got, err := m.src.Bodies(m.ctx, bodies)
		if err != nil {
			msg.err = err
			return msg
		}
		msg.bodies = got
	}
	return msg
}

func uniq(ss []string) []string {
	slices.Sort(ss)
	return slices.Compact(ss)
}

// autoRead takes in the searches and keeps the pull requests a rule
// matches, or may once whether the pending checks only wait for an
// approval is known, then reads what deciding the approval needs.
func (m *Model) autoRead(msg autoReadMsg) tea.Cmd {
	if msg.err != nil {
		m.auto.running = false
		m.log.Warn("auto-approve could not read", logging.KeyOp, "auto_approve", "err", msg.err.Error())
		return nil
	}
	var cmds []tea.Cmd
	now := m.set.Now()
	at := map[model.PRKey]int{}
	var cands []autoCand
	for i, f := range msg.found {
		r := m.set.AutoApprove[i]
		if f.total > autoMax && !m.auto.toldMany[i] {
			m.auto.toldMany[i] = true
			cmds = append(cmds, m.pushNote("⚠", fmt.Sprintf("auto-approve %q: its search finds %d, and only the first %d are judged; narrow its query", ruleName(r.Rule), f.total, autoMax), nil))
		}
		for _, p := range f.prs {
			if m.auto.tried[autoTry{p.Key, p.HeadOID}] {
				continue
			}
			body, known := msg.bodies[p.ID]
			// Whether the pending checks only wait for an approval is read
			// with the details next; a rule waiting only for it may match.
			facts := plan.Facts{Body: body, BodyKnown: known || !r.Rule.NeedsBody(), Now: now, ApprovalKnown: len(m.set.Plan.ApprovalChecks) == 0}
			if p.Draft || p.State != model.StateOpen {
				continue
			}
			switch r.Rule.Judge(p, m.viewer, facts) {
			case plan.Yes, plan.Pending:
				j, ok := at[p.Key]
				if !ok {
					j = len(cands)
					at[p.Key] = j
					cands = append(cands, autoCand{pr: p})
				}
				cands[j].rules = append(cands[j].rules, i)
				if known {
					cands[j].body = body
				}
			}
		}
	}
	if len(cands) == 0 {
		m.auto.running = false
		return tea.Batch(cmds...)
	}
	ids := make([]string, len(cands))
	for i, c := range cands {
		ids[i] = c.pr.ID
	}
	cmds = append(cmds, func() tea.Msg {
		info, err := m.src.MergeInfo(m.ctx, ids, true)
		return autoInfoMsg{cands: cands, info: info, err: err}
	})
	return tea.Batch(cmds...)
}

// autoDecide decides each candidate on what was read again, and approves
// those that go, or tells of them for a dry run. A pull request in a
// confirm popup or a running batch is left to it this time.
func (m *Model) autoDecide(msg autoInfoMsg) tea.Cmd {
	m.auto.running = false
	if msg.err != nil {
		m.log.Warn("auto-approve could not read", logging.KeyOp, "auto_approve", "err", msg.err.Error())
		return nil
	}
	now := m.set.Now()
	var items []runner.Item
	var dry []noteItem
	rules := map[model.PRKey]string{}
	for _, c := range msg.cands {
		p := c.pr
		mi, ok := msg.info[p.ID]
		if !ok {
			continue
		}
		// The first rule that matches on what was read again approves.
		var r AutoApprove
		var why string
		for _, i := range c.rules {
			r = m.set.AutoApprove[i]
			if why = m.autoHeld(p, c.body, mi, r, now); why != notMatched {
				break
			}
		}
		log := m.log.With(logging.KeyOp, "auto_approve", logging.KeyPR, p.Key.String(), logging.KeyRule, ruleName(r.Rule), logging.KeyHead, p.HeadOID)
		if why != "" {
			log.Debug("auto-approve passed over", "why", why)
			continue
		}
		m.auto.tried[autoTry{p.Key, p.HeadOID}] = true
		if r.DryRun {
			log.Info("auto-approve would approve")
			dry = append(dry, noteItem{key: p.Key, text: "would approve · " + ruleName(r.Rule)})
			continue
		}
		log.Info("auto-approve")
		rules[p.Key] = ruleName(r.Rule)
		items = append(items, runner.Item{PR: p, Head: p.HeadOID, Wait: true, Auto: true})
	}
	var cmds []tea.Cmd
	if len(dry) > 0 {
		cmds = append(cmds, m.pushNote("–", fmt.Sprintf("auto-approve (dry run): would approve %d", len(dry)), dry))
	}
	if len(items) == 0 {
		return tea.Batch(cmds...)
	}
	rv := runner.ReviewText{Event: model.EventApprove}
	bs := &batchState{kind: runner.Review, review: rv, auto: rules, goes: map[model.PRKey]bool{}, done: map[model.PRKey]progress{}}
	for _, it := range items {
		bs.goes[it.PR.Key] = true
		bs.order = append(bs.order, it.PR.Key)
		m.hold(it.PR.Key)
		m.progress[it.PR.Key] = progress{text: "queued"}
	}
	id, ch := m.run.StartReview(m.ctx, rv, items)
	m.addBatch(id, bs)
	return tea.Batch(append(cmds, listen(id, ch))...)
}

// autoHeld says why p, read again as mi, is not approved by r now, ""
// when it is: the rule no longer matches it, an exclusion matches it or
// cannot tell, the head moved since the search, the user is at it in a
// confirm popup or a batch, or the approval itself would be skipped.
func (m *Model) autoHeld(p model.PR, body string, mi model.MergeInfo, r AutoApprove, now time.Time) string {
	if mi.HeadOID != p.HeadOID {
		return "head moved since the search"
	}
	if pg, ok := m.progress[p.Key]; ok && pg.result == runner.Running {
		return "in a batch"
	}
	if m.bulk != nil && slices.ContainsFunc(m.bulk.items, func(it bulkItem) bool { return it.pr.Key == p.Key }) {
		return "in a confirm popup"
	}
	facts := plan.Facts{Body: body, BodyKnown: true, Now: now, ApprovalKnown: true, AwaitingApproval: plan.AwaitingApproval(mi.Checks, m.set.Plan.ApprovalChecks)}
	if r.Rule.Judge(p, m.viewer, facts) != plan.Yes {
		return notMatched
	}
	if n, ok := m.unread[p.Key]; ok {
		facts.Reason = n.Reason
	}
	for _, x := range m.set.Exclude {
		switch x.Judge(p, m.viewer, facts) {
		case plan.Yes:
			return "excluded by " + ruleName(x)
		case plan.Pending:
			return "an exclusion cannot tell yet: " + ruleName(x)
		}
	}
	// Failing checks are not let in: the rule did not say so.
	if d := plan.ForAutoApprove(mi, p, m.viewer, false, m.set.Plan); d.Step == plan.StepSkip {
		return d.Reason
	}
	return ""
}

// notMatched is why a rule passes over a pull request it no longer
// matches, when the next rule that may is asked.
const notMatched = "the rule does not match"

// autoNoteTitle is the title of the notification of a batch of approvals
// rules asked for.
func autoNoteTitle(parts []string) string {
	return "auto-approve done: " + strings.Join(parts, ", ")
}
