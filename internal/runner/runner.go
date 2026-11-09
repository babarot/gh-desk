// Package runner reviews, merges and marks ready pull requests, many at
// once, and follows them until each is merged, skipped or stopped. It decides each
// step with internal/plan from information read just before the step, and
// acts only on the head the user confirmed.
//
// Pull requests of different repositories go side by side. Merges of one
// repository wait for their jobs side by side, and take a lane for the
// writes that may move the base, since merging one can leave the next
// behind its base or in conflict; the other actions on a repository go one
// at a time. Progress is reported as Events on a channel;
// the runner knows nothing of the TUI.
package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
)

// GitHub is what the runner needs from GitHub.
type GitHub interface {
	MergeInfo(ctx context.Context, ids []string, details bool) (map[string]model.MergeInfo, error)
	Review(ctx context.Context, pr model.PR, event model.ReviewEvent, body string) error
	Merge(ctx context.Context, key model.PRKey, id, head string, method model.MergeMethod) error
	EnableAutoMerge(ctx context.Context, key model.PRKey, id, head string, method model.MergeMethod) error
	Enqueue(ctx context.Context, key model.PRKey, id, head string) error
	UpdateBranch(ctx context.Context, key model.PRKey, id, head string) error
	MarkReady(ctx context.Context, key model.PRKey, id, head string) error
	ConvertToDraft(ctx context.Context, key model.PRKey, id, head string) error
	PostComment(ctx context.Context, pr model.PR, body string) error
	Close(ctx context.Context, key model.PRKey, id, head string) error
	DeleteBranch(ctx context.Context, key model.PRKey, refID, head string) error
	// BasedOn reads the open pull requests whose base each branch is.
	BasedOn(ctx context.Context, branches []model.Branch) (map[model.Branch][]int, error)
	CommitInfo(ctx context.Context, key model.PRKey, oid string) (Commit, error)
}

// Commit is what tells a commit gh-desk made by updating a branch from
// one someone pushed.
type Commit struct {
	Parents []string
	// ViaWeb is set on a commit GitHub made, not one pushed.
	ViaWeb bool
	Author string
}

// ErrCleanStatus is what EnableAutoMerge returns when the pull request can
// be merged now; the runner merges it directly then. The app package maps
// the GitHub client's error to it.
var ErrCleanStatus = errors.New("pull request is in clean status")

// Store records the merges being waited for, so the next launch goes on
// with them. Nil records nothing.
type Store interface {
	Put(p model.Pending) error
	Remove(k model.PRKey) error
}

// Options configure a Runner.
type Options struct {
	Store Store
	// CanWait allows merges that wait (for checks, a branch update,
	// auto-merge). It is set when this gh-desk holds the record of
	// pending merges; without it nothing would go on with them after
	// quitting, so they are skipped.
	CanWait  bool
	Settings plan.Settings
	Viewer   string
	// WaitTimeout is how long to wait for the jobs at work on a head, and
	// for required checks still running before an admin merge.
	WaitTimeout time.Duration
	// BlockedTimeout is how long a pull request may stay blocked after its
	// jobs and required checks finished before it is given up as blocked
	// by another rule.
	BlockedTimeout time.Duration
	// WatchTimeout is how long a pull request put on auto-merge is
	// followed, 24h unless set: the merge is GitHub's, so this decides
	// only how long gh-desk watches for the result (ADR-006).
	WatchTimeout time.Duration
	// PollFast is how often a waiting pull request is read in its first
	// minute, PollSlow after it and while watching auto-merge.
	PollFast, PollSlow time.Duration
	// FullRead is the longest a waiting pull request goes without a full
	// read when the light read leaves it asleep; 5m unless set.
	FullRead time.Duration
	// Recheck is how often, and RecheckFor how long, a pull request whose
	// merge state GitHub is still working out is read again.
	Recheck, RecheckFor time.Duration
	// Quiet is how long after the user's yes, marking a pull request ready
	// or updating its branch, jobs not seen yet are waited for; plan.Quiet
	// unless set.
	Quiet time.Duration
	// Excluded says why an [[exclude]] rule keeps pr, read as mi, from an
	// approval a rule asked for, "" when none does; an error keeps it too
	// (ADR-093). Nil when there are no such rules.
	Excluded func(ctx context.Context, pr model.PR, mi model.MergeInfo) (string, error)
}

// Defaults fills in the intervals left zero.
func (o Options) withDefaults() Options {
	if o.PollFast == 0 {
		o.PollFast = 15 * time.Second
	}
	if o.PollSlow == 0 {
		o.PollSlow = time.Minute
	}
	if o.WatchTimeout == 0 {
		o.WatchTimeout = 24 * time.Hour
	}
	if o.FullRead == 0 {
		o.FullRead = 5 * time.Minute
	}
	if o.Recheck == 0 {
		o.Recheck = 3 * time.Second
	}
	if o.RecheckFor == 0 {
		o.RecheckFor = time.Minute
	}
	if o.Quiet == 0 {
		o.Quiet = plan.Quiet
	}
	return o
}

// Kind is what a batch does.
type Kind int

const (
	// Review sends a review, of the kind and with the text StartReview
	// is given.
	Review Kind = iota
	Merge
	// Ready marks drafts ready for review.
	Ready
	// ToDraft turns pull requests back into drafts.
	ToDraft
	// Close closes pull requests, with the comment and the deletion of
	// their branches StartClose is given.
	Close
	// Comment adds the text StartComment is given to each pull request's
	// conversation.
	Comment
)

// CloseText is what a Close batch does besides closing: the comment it
// writes first, and whether it deletes the head branches. It is held in
// memory only.
type CloseText struct {
	Body         string
	DeleteBranch bool
}

// ReviewText is the review a Review batch sends to each of its pull
// requests. It is held in memory only, but for an approval that waits for
// the jobs: that is recorded for the next launch with its body.
type ReviewText struct {
	Event model.ReviewEvent
	Body  string
}

// Item is a pull request the user confirmed, with the head they saw.
type Item struct {
	PR   model.PR
	Head string
	// AutoMerge is set when the user was told this pull request goes to
	// auto-merge, which also merges commits pushed later. Without it the
	// runner never turns auto-merge on: it waits and merges itself.
	AutoMerge bool
	// Wait is set on an approval the user asked to wait for the jobs at
	// work on the head; it is approved once they passed.
	Wait bool
	// IncludeFailing is set when the user let in pull requests whose
	// checks failed: a job that fails while the approval waits does not
	// stop it.
	IncludeFailing bool
	// Auto is set on an approval a rule asked for, without a confirm: a
	// draft, a head whose approval a person dismissed and a pull request
	// an exclusion matches are not approved, judged again right before
	// it is sent (ADR-093).
	Auto bool
}

// Result is where a pull request ended, or Running while it has not.
type Result int

const (
	Running Result = iota
	Approved
	Merged
	AutoMergeOn
	// Queued is a pull request in its base branch's merge queue, which
	// merges it.
	Queued
	// Readied is a draft marked ready for review.
	Readied
	// Drafted is a pull request turned back into a draft.
	Drafted
	// Reviewed is a review sent that is not an approval.
	Reviewed
	// Closed is a pull request closed; its Text says what became of its
	// branch.
	Closed
	// CommentSent is a comment GitHub took on a Comment batch.
	CommentSent
	Skipped
	Stopped
	Failed
)

// Final reports whether the result ends the pull request's part in the
// batch. AutoMergeOn and Queued do: GitHub merges it, and a later Event
// says so.
func (r Result) Final() bool { return r != Running }

// Event is a step of one pull request.
type Event struct {
	Batch  string
	Key    model.PRKey
	Result Result
	// Text says what is happening or happened, in a few words.
	Text string
	// Approved is set when an approval was sent.
	Approved bool
	// Readied is set when a draft was marked ready on its way to a merge.
	Readied bool
	// Commented is set on the one event that says the comment of a close
	// was taken, so it can be shown without reading the pull request
	// again (ADR-067).
	Commented bool
	// Closed is set on every event after the pull request was closed,
	// whatever happens next.
	Closed bool
	// Method is how it was merged, on a Merged event gh-desk made.
	Method model.MergeMethod
	// Watch is set on events of a pull request put on auto-merge or in
	// the merge queue, after its batch has ended.
	Watch bool
	// Queued is set on a Watch event of a pull request seen in the merge
	// queue.
	Queued bool
	// Locked is set on a skip because the merge would wait while another
	// gh-desk holds the record of pending merges.
	Locked bool
	// Step is the step a Running event starts or goes on with; empty
	// for one that is no step of its own, such as reading it again.
	Step Step
	// Checks names the checks a wait is for, on a wait's events.
	Checks []string
	// Forecast is set on an event that only says the steps still ahead,
	// in Plan; it carries no Text and changes nothing else.
	Forecast bool
	// Plan is the steps still ahead, on a Forecast event. It is a guess
	// made at each decision and replaced by the next one: a decision is
	// made again after marking ready and approving, and at each read
	// while waiting.
	Plan []Step
}

// Step is a step a job takes on a pull request.
type Step string

const (
	StepReady   Step = "ready"
	StepDraft   Step = "draft"
	StepApprove Step = "approve"
	// StepReview sends a review other than an approval.
	StepReview Step = "review"
	StepUpdate Step = "update-branch"
	// StepWait is a wait not yet begun, in a Plan: what it waits on is
	// known once it begins.
	StepWait       Step = "wait"
	StepWaitChecks Step = "wait-checks"
	// StepWaitWorkflows waits the quiet time for workflows that may still
	// start, before anything is seen at work.
	StepWaitWorkflows Step = "wait-workflows"
	StepWaitApproval  Step = "wait-approval-checks"
	// StepWaitBlocked waits on a rule other than checks.
	StepWaitBlocked Step = "wait-blocked"
	StepMerge       Step = "merge"
	StepAutoMerge   Step = "auto-merge"
	// StepEnqueue puts it in the merge queue.
	StepEnqueue   Step = "enqueue"
	StepComment   Step = "comment"
	StepClose     Step = "close"
	StepDeleteRef Step = "delete-branch"
)

// Waits reports whether s is a wait.
func (s Step) Waits() bool {
	return s == StepWait || s == StepWaitChecks || s == StepWaitWorkflows || s == StepWaitApproval || s == StepWaitBlocked
}

// Runner runs batches.
type Runner struct {
	gh  GitHub
	opt Options
	log *slog.Logger
	// active counts the batches not yet ended, for Wait.
	active sync.WaitGroup
	// mu guards busy, the pull requests a job is acting on or watching.
	// A second job for one of them is skipped, so two jobs never write
	// the same record.
	mu   sync.Mutex
	busy map[model.PRKey]bool
	// lanes are, by repository, the merges ready to write, in the order
	// they got ready; the first is the one at work. Guarded by mu.
	lanes map[string][]*ticket
	// merges are, by repository, the merges at work, from their claim
	// until they are released. Guarded by mu.
	merges map[string]map[model.PRKey]*activeMerge
	// reads are, by repository, the waits sharing a read. Guarded by mu.
	reads map[string]*repoReads
	// wmu guards store and canWait, which Adopt sets after the start.
	wmu     sync.Mutex
	store   Store
	canWait bool
}

// Adopt gives the runner the record of pending merges after it started
// without it, once this gh-desk took the lock from one that quit.
// Merges confirmed from then on may wait; jobs already running go on
// without it, so none waits unrecorded.
func (r *Runner) Adopt(s Store) {
	r.wmu.Lock()
	defer r.wmu.Unlock()
	r.store, r.canWait = s, true
}

// waiting is the record and whether merges may wait, taken together so a
// job never waits without the record to keep it in.
func (r *Runner) waiting() (Store, bool) {
	r.wmu.Lock()
	defer r.wmu.Unlock()
	return r.store, r.canWait
}

func (r *Runner) release(k model.PRKey) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.busy, k)
	r.untrack(k)
}

// ticket is a merge's place in the lane of its repository.
type ticket struct {
	key model.PRKey
	// turn is closed when the ticket is first in its lane.
	turn chan struct{}
	left bool
}

// enter puts a merge at the end of its repository's lane and returns the
// pull request ahead of it, if any. Called with mu held.
func (r *Runner) enter(k model.PRKey) (*ticket, *model.PRKey) {
	repo := k.Repository()
	t := &ticket{key: k, turn: make(chan struct{})}
	q := r.lanes[repo]
	r.lanes[repo] = append(q, t)
	if len(q) == 0 {
		close(t.turn)
		return t, nil
	}
	ahead := q[len(q)-1].key
	return t, &ahead
}

// leave takes t out of its lane, at its end or when it gives up waiting,
// and gives the turn to the next one.
func (r *Runner) leave(t *ticket) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.left {
		return
	}
	t.left = true
	repo := t.key.Repository()
	r.readSoon(repo)
	q := r.lanes[repo]
	i := slices.Index(q, t)
	if i < 0 {
		return
	}
	q = slices.Delete(q, i, i+1)
	if len(q) == 0 {
		delete(r.lanes, repo)
		return
	}
	r.lanes[repo] = q
	if i == 0 {
		close(q[0].turn)
	}
}

// inLane reports whether a pull request of a batch takes its repository's
// lane: a merge, or a recorded merge resumed, but not one only watched on
// auto-merge or an approval that waits, which go aside.
func inLane(kind Kind, resume map[model.PRKey]model.Pending, k model.PRKey) bool {
	p, ok := resume[k]
	return isMerge(kind, resume, k) && (!ok || !p.Watching)
}

// isMerge reports whether a pull request of a batch is being merged, one
// watched on auto-merge included.
func isMerge(kind Kind, resume map[model.PRKey]model.Pending, k model.PRKey) bool {
	if kind != Merge {
		return false
	}
	p, ok := resume[k]
	return !ok || p.Kind != model.PendingApprove
}

// Wait waits for every batch to end. After its context is canceled, a
// batch ends once its jobs have stopped writing to the record.
func (r *Runner) Wait() { r.active.Wait() }

// New returns a Runner.
func New(gh GitHub, log *slog.Logger, opt Options) *Runner {
	return &Runner{gh: gh, opt: opt.withDefaults(), log: log, busy: map[model.PRKey]bool{}, lanes: map[string][]*ticket{}, merges: map[string]map[model.PRKey]*activeMerge{}, reads: map[string]*repoReads{}, store: opt.Store, canWait: opt.CanWait}
}

// StartReview runs a batch that sends rv to each pull request, as Start
// does a batch of another kind.
func (r *Runner) StartReview(ctx context.Context, rv ReviewText, items []Item) (string, <-chan Event) {
	return r.run(ctx, Review, items, nil, rv, CloseText{})
}

// StartClose runs a batch that closes each pull request as ct says, as
// Start does a batch of another kind.
func (r *Runner) StartClose(ctx context.Context, ct CloseText, items []Item) (string, <-chan Event) {
	return r.run(ctx, Close, items, nil, ReviewText{}, ct)
}

// StartComment runs a batch that adds body to each pull request's
// conversation, as Start does a batch of another kind.
func (r *Runner) StartComment(ctx context.Context, body string, items []Item) (string, <-chan Event) {
	return r.run(ctx, Comment, items, nil, ReviewText{}, CloseText{Body: body})
}

// Start runs a batch of a merge or a ready for review and returns its ID
// and its events. The channel is
// closed when every pull request has a final result and every auto-merge
// it turned on is no longer watched. Canceling ctx stops it all.
func (r *Runner) Start(ctx context.Context, kind Kind, items []Item) (string, <-chan Event) {
	return r.run(ctx, kind, items, nil, ReviewText{}, CloseText{})
}

// Resume goes on with the merges recorded by an earlier run. Each pull
// request is read again first: one whose head moved, that was closed, or
// whose approval by gh-desk was dismissed is stopped, not merged.
func (r *Runner) Resume(ctx context.Context, ps []model.Pending) (string, <-chan Event) {
	items := make([]Item, len(ps))
	pend := map[model.PRKey]model.Pending{}
	for i, p := range ps {
		items[i] = Item{PR: model.PR{ID: p.ID, Key: p.Key, Author: p.Author, AuthorIsBot: p.AuthorIsBot, HeadOID: p.Head},
			Head: p.Head, AutoMerge: p.AutoMerge, Wait: p.Kind == model.PendingApprove, IncludeFailing: p.IncludeFailing, Auto: p.Auto}
		pend[p.Key] = p
	}
	return r.run(ctx, Merge, items, pend, ReviewText{}, CloseText{})
}

func (r *Runner) run(ctx context.Context, kind Kind, items []Item, resume map[model.PRKey]model.Pending, rv ReviewText, ct CloseText) (string, <-chan Event) {
	batch := logging.NewID()
	log := r.log.With(logging.KeyBatch, batch)
	events := make(chan Event, 64)
	byRepo := map[string][]Item{}
	var repos []string
	for _, it := range items {
		repo := it.PR.Key.Repository()
		if _, ok := byRepo[repo]; !ok {
			repos = append(repos, repo)
		}
		byRepo[repo] = append(byRepo[repo], it)
	}
	verb := map[Kind]string{Review: "review", Merge: "merge", Ready: "ready", ToDraft: "draft", Close: "close", Comment: "comment"}[kind]
	if resume != nil {
		verb = "resume"
	}
	attrs := []any{"kind", verb, "prs", len(items), "repos", len(repos)}
	if kind == Review {
		attrs = append(attrs, "event", strings.ToLower(string(rv.Event)))
	}
	if kind == Close {
		attrs = append(attrs, "comment", ct.Body != "", "delete_branch", ct.DeleteBranch)
	}
	log.Info("batch start", attrs...)
	// The user said yes now; a pull request opened or pushed just before
	// may have no workflow run yet.
	confirmed := time.Now()
	var wg sync.WaitGroup
	r.active.Add(1)
	emit := func(e Event) {
		e.Batch = batch
		select {
		case events <- e:
		case <-ctx.Done():
		}
	}
	// Every pull request is claimed, and every merge is known as at work,
	// before the batch is returned: a batch started later cannot take what
	// this one acts on, and waits for a base among its merges.
	type slot struct {
		it      Item
		claimed bool
		// merge is set on a merge at work; lane on one that takes the lane.
		merge, lane bool
		// unread says why its branches could not be read.
		unread string
	}
	slots := map[string][]*slot{}
	r.mu.Lock()
	for _, repo := range repos {
		for _, it := range byRepo[repo] {
			s := &slot{it: it, claimed: !r.busy[it.PR.Key]}
			if s.claimed {
				r.busy[it.PR.Key] = true
				s.lane = inLane(kind, resume, it.PR.Key)
				if s.merge = isMerge(kind, resume, it.PR.Key); s.merge {
					r.track(it.PR.Key)
				}
			}
			slots[repo] = append(slots[repo], s)
		}
	}
	r.mu.Unlock()
	runOne := func(s *slot) {
		it := s.it
		if ctx.Err() != nil {
			if s.claimed {
				r.release(it.PR.Key)
			}
			return
		}
		store, canWait := r.waiting()
		j := &job{r: r, ctx: ctx, log: log.With(logging.KeyPR, it.PR.Key.String()), it: it, head: it.Head, emit: emit, wg: &wg,
			store: store, canWait: canWait, record: kind == Merge && store != nil}
		j.trigger = plan.Trigger{At: confirmed, For: r.opt.Quiet}
		if resume != nil {
			// What gh-desk wrote before it quit (marking ready,
			// updating the branch) may have started workflows not
			// shown yet; when, is not recorded.
			j.trigger.Always = true
		}
		if !s.claimed {
			// Its record belongs to the job already on it.
			j.record = false
			j.say(Skipped, "already being merged or watched")
			return
		}
		switch p, ok := resume[it.PR.Key]; {
		case s.unread != "":
			// Nothing is written: which merge it waits for is not known.
			// A new merge has no record to keep.
			j.record = j.record && ok
			j.unsure("could not read it: " + s.unread)
		case ok:
			j.pend, j.approved = p, p.Approved
			j.resume()
		case kind == Review:
			j.review(rv)
		case kind == Ready:
			j.markReady()
		case kind == ToDraft:
			j.convertToDraft()
		case kind == Close:
			j.closePR(ct)
		case kind == Comment:
			j.comment(ct.Body)
		default:
			j.merge()
		}
		if !j.watching {
			r.release(it.PR.Key)
		}
	}
	for _, repo := range repos {
		ss := slots[repo]
		wg.Go(func() {
			var merges []*slot
			var prs []model.PR
			for _, s := range ss {
				if s.merge {
					merges, prs = append(merges, s), append(prs, s.it.PR)
				}
			}
			r.readBranches(ctx, prs, func(i int, why string) { merges[i].unread = why })
			for i, s := range ss {
				if i > 0 && !s.lane {
					emit(Event{Key: s.it.PR.Key, Text: fmt.Sprintf("after #%d", ss[i-1].it.PR.Key.Number)})
				}
			}
			for _, s := range ss {
				// Merges wait side by side; what moves the base takes
				// the lane.
				if s.lane {
					wg.Go(func() { runOne(s) })
				} else {
					runOne(s)
				}
			}
		})
	}
	go func() {
		wg.Wait()
		log.Info("batch end")
		close(events)
		r.active.Done()
	}()
	return batch, events
}

// job is one pull request in a batch.
type job struct {
	r    *Runner
	ctx  context.Context
	log  *slog.Logger
	it   Item
	head string
	emit func(Event)
	wg   *sync.WaitGroup
	// approved is set once this job sent an approval, at approvedAt.
	approved   bool
	approvedAt time.Time
	// readied is set once this job marked the draft ready, at readiedAt.
	readied   bool
	readiedAt time.Time
	// closed is set once this job closed the pull request.
	closed bool
	// watching is set once the pull request is handed to watch, which
	// releases it.
	watching bool
	// store and canWait are the runner's when the job was made, kept for
	// its whole run.
	store   Store
	canWait bool
	// record keeps pend in the store while the job waits.
	record bool
	pend   model.Pending
	// keep is set when the job ends without knowing how the pull request
	// stands (a read or a write failed): its record stays, so the next
	// launch reads it again and goes on.
	keep bool
	// planned is the last forecast sent, so the same is not sent again.
	planned  []Step
	sentPlan bool
	// trigger is the last thing that may have started workflows on the
	// head: the user's yes, or gh-desk marking it ready or updating its
	// branch. Jobs it starts are waited for even before they show.
	trigger plan.Trigger
}

func (j *job) say(res Result, text string) {
	j.emit(Event{Key: j.it.PR.Key, Result: res, Text: text, Approved: j.approved, Readied: j.readied, Closed: j.closed})
	if !res.Final() {
		return
	}
	j.log.Info("pr end", logging.KeyStep, resultWord[res], "why", text)
	// The record goes only with an end that is known: not when gh-desk
	// is quitting, not when a read or a write failed midway, and not when
	// GitHub's auto-merge is left to watch.
	if !watched(res) && !j.keep && j.ctx.Err() == nil {
		j.forget()
	}
}

// watched reports whether a result leaves the pull request to GitHub,
// watched with its record kept.
func watched(res Result) bool { return res == AutoMergeOn || res == Queued }

// step says the job starts or goes on with a step.
func (j *job) step(s Step, text string, checks ...string) {
	j.emit(Event{Key: j.it.PR.Key, Result: Running, Text: text, Approved: j.approved, Readied: j.readied, Closed: j.closed, Step: s, Checks: checks})
}

// forecast says the steps still ahead, when they differ from the last
// said.
func (j *job) forecast(steps []Step) {
	if j.sentPlan && slices.Equal(steps, j.planned) {
		return
	}
	j.planned, j.sentPlan = steps, true
	j.emit(Event{Key: j.it.PR.Key, Result: Running, Forecast: true, Plan: slices.Clone(steps)})
}

// ahead is what a decision leaves to do, from the step it decides on.
func ahead(d plan.Decision) []Step {
	var out []Step
	if d.Ready {
		out = append(out, StepReady)
	}
	if d.Approve {
		out = append(out, StepApprove)
	}
	last := StepMerge
	if d.Queue {
		last = StepEnqueue
	}
	switch d.Step {
	case plan.StepMerge, plan.StepEnqueue:
		out = append(out, last)
	case plan.StepAutoMerge:
		out = append(out, StepAutoMerge)
	case plan.StepUpdate:
		out = append(out, StepUpdate, StepWait, last)
	case plan.StepWait:
		out = append(out, StepWait, last)
	}
	return out
}

// lastStep is the write that merges the pull request: putting it in the
// merge queue where its base has one.
func lastStep(mi model.MergeInfo) Step {
	if mi.Queue {
		return StepEnqueue
	}
	return StepMerge
}

// locked skips a merge that would wait without the record of pending
// merges, which another gh-desk holds.
func (j *job) locked(text string) {
	j.emit(Event{Key: j.it.PR.Key, Result: Skipped, Text: text, Approved: j.approved, Readied: j.readied, Locked: true})
	j.log.Info("pr end", logging.KeyStep, resultWord[Skipped], "why", text)
}

// unsure ends the job keeping its record: something failed midway.
func (j *job) unsure(text string) {
	j.keep = true
	if j.record {
		text += " · goes on at the next launch"
	}
	j.say(Failed, text)
}

// save records where the job is, so the next launch can go on with it. It
// reports false when the record could not be written.
func (j *job) save() bool {
	if !j.record {
		return true
	}
	j.pend.Key, j.pend.ID, j.pend.Head = j.it.PR.Key, j.it.PR.ID, j.head
	j.pend.Author, j.pend.AuthorIsBot = j.it.PR.Author, j.it.PR.AuthorIsBot
	j.pend.AutoMerge = j.it.AutoMerge
	j.pend.Approved = j.pend.Approved || j.approved
	if err := j.store.Put(j.pend); err != nil {
		j.log.Warn("could not record the pending merge", "err", err.Error())
		return false
	}
	return true
}

// waits reports whether a step waits after this run's first write, which
// only a gh-desk holding the record may start.
func waits(s plan.Step) bool {
	return s == plan.StepWait || s == plan.StepUpdate || s == plan.StepAutoMerge
}

func (j *job) forget() {
	if !j.record {
		return
	}
	if err := j.store.Remove(j.it.PR.Key); err != nil {
		j.log.Warn("could not forget the pending merge", "err", err.Error())
	}
}

// resume goes on with a recorded merge after reading it again.
func (j *job) resume() {
	if j.pend.Kind == model.PendingApprove {
		j.resumeApproval()
		return
	}
	j.say(Running, "resuming")
	mi, ok := j.read(false)
	if !ok {
		return
	}
	switch mi.State {
	case model.StateMerged:
		j.say(Merged, "merged meanwhile")
		return
	case model.StateClosed:
		j.say(Stopped, "closed meanwhile")
		return
	}
	approved := approvedBy(mi, j.r.opt.Viewer)
	switch {
	case j.pend.Approved && !approved:
		j.say(Stopped, "your approval was dismissed · confirm again")
		return
	case j.pend.Approving && !approved:
		// It may have reached GitHub and been dismissed since.
		j.say(Stopped, "whether the approval was sent is not known · confirm again")
		return
	case j.pend.Approving:
		j.approved, j.pend.Approving = true, false
	}
	if j.pend.Watching {
		if mi.InQueue {
			j.say(Queued, "in the merge queue")
		} else {
			j.say(AutoMergeOn, "auto-merge on")
		}
		// Watched aside, as after turning auto-merge on, so the next
		// pull request of the repository does not wait for it.
		j.startWatch()
		return
	}
	j.merge()
}

// waitToApprove records an approval that waits for the jobs, with its
// body, and waits aside, as auto-merge is watched, so the next pull
// request of the repository is not held up by it.
func (j *job) waitToApprove(body string) {
	if !j.canWait {
		j.locked("locked by another gh-desk")
		return
	}
	j.record = j.store != nil
	j.pend.Kind, j.pend.IncludeFailing, j.pend.Body, j.pend.Auto = model.PendingApprove, j.it.IncludeFailing, body, j.it.Auto
	if !j.save() {
		j.say(Failed, "could not record the pending approval; nothing was sent")
		return
	}
	j.forecast([]Step{StepWait, StepApprove})
	j.watching = true
	j.wg.Go(func() {
		defer j.r.release(j.it.PR.Key)
		j.approveAfterJobs()
	})
}

// resumeApproval goes on with a recorded approval that waited for the
// jobs.
func (j *job) resumeApproval() {
	j.say(Running, "resuming")
	mi, ok := j.read(false)
	if !ok {
		return
	}
	approved := approvedBy(mi, j.r.opt.Viewer)
	switch {
	case approved:
		j.approved = true
		j.say(Approved, "approved")
		return
	case j.pend.Approving:
		// It may have reached GitHub and been dismissed since.
		j.say(Stopped, "whether the approval was sent is not known · confirm again")
		return
	}
	j.watching = true
	j.wg.Go(func() {
		defer j.r.release(j.it.PR.Key)
		j.approveAfterJobs()
	})
}

// approveAfterJobs reads the pull request until no job is at work on its
// head, then approves it; a job that failed stops it, unless the user let
// failing ones in. Every read checks the head is the one confirmed.
func (j *job) approveAfterJobs() {
	last := time.Now()
	w := &waitLog{log: j.log}
	for {
		mi, ok := j.read(true)
		if !ok {
			w.end("stopped")
			return
		}
		w.read(mi)
		// Failing required checks were left out by the popup unless let
		// in, as for an approval that does not wait.
		d := j.forApprove(mi)
		d = plan.AfterJobs(d, mi, j.trigger, time.Now(), j.it.IncludeFailing)
		switch d.Step {
		case plan.StepSkip:
			w.end("skipped")
			j.say(Skipped, d.Reason)
			return
		case plan.StepWait:
			if !j.waitedForChecks(mi, &last, w) {
				return
			}
			j.waitingFor(mi, w)
			j.forecast([]Step{StepApprove})
			if !j.pause(w, mi) {
				return
			}
			continue
		}
		w.end("ready")
		if !j.notExcluded(mi) {
			return
		}
		j.forecast([]Step{StepApprove})
		if j.sendApproval(j.pend.Body) {
			j.say(Approved, "approved")
		}
		return
	}
}

// forApprove decides approving the pull request read as mi: as a rule's
// approval when the item is one. A confirmed approval lets failing checks
// through here, as the popup already left them out unless the user let
// them in. A rule's is judged on them at every read, since a required
// status that fails while it waits is not a job, which the wait stops on.
func (j *job) forApprove(mi model.MergeInfo) plan.Decision {
	if j.it.Auto {
		return plan.ForAutoApprove(mi, j.it.PR, j.r.opt.Viewer, j.it.IncludeFailing, j.r.opt.Settings)
	}
	return plan.ForApprove(mi, j.it.PR, j.r.opt.Viewer, true, j.r.opt.Settings)
}

// notExcluded reports whether a rule's approval may still be sent: no
// exclusion matches the pull request, read as mi, right before it is. It
// ends the job when one does, or when that cannot be told.
func (j *job) notExcluded(mi model.MergeInfo) bool {
	if !j.it.Auto || j.r.opt.Excluded == nil {
		return true
	}
	why, err := j.r.opt.Excluded(j.ctx, j.it.PR, mi)
	switch {
	case err != nil:
		j.say(Skipped, "could not tell whether it is excluded: "+short(err))
		return false
	case why != "":
		j.say(Skipped, "excluded: "+why)
		return false
	}
	return true
}

// approvedBy reports whether login's latest review approves.
func approvedBy(mi model.MergeInfo, login string) bool {
	for _, r := range mi.Reviews {
		if r.Author == login && r.State == model.ReviewApproved {
			return true
		}
	}
	return false
}

var resultWord = map[Result]string{Running: "running", Approved: "approved", Merged: "merged", AutoMergeOn: "auto-merge-on", Queued: "queued", Readied: "readied", Drafted: "drafted", Reviewed: "reviewed", Closed: "closed", CommentSent: "comment-sent",
	Skipped: "skipped", Stopped: "stopped", Failed: "failed"}

// read reads the pull request and checks its head is the one confirmed.
func (j *job) read(details bool) (model.MergeInfo, bool) {
	got, err := j.r.gh.MergeInfo(j.ctx, []string{j.it.PR.ID}, details)
	if err != nil {
		j.unsure("could not read it: " + short(err))
		return model.MergeInfo{}, false
	}
	mi, ok := got[j.it.PR.ID]
	if !ok {
		j.unsure("could not read it")
		return model.MergeInfo{}, false
	}
	if mi.State == model.StateOpen && mi.HeadOID != j.head {
		j.say(Stopped, "head changed · confirm again")
		return model.MergeInfo{}, false
	}
	return mi, true
}

// review sends rv on the head the user confirmed, after reading the pull
// request again and deciding again whether it takes it.
func (j *job) review(rv ReviewText) {
	approve := rv.Event == model.EventApprove
	mi, ok := j.read(approve)
	if !ok {
		return
	}
	// Failing checks are let through here: the popup already left them out
	// unless the user asked to include them.
	d := plan.ForReview(mi, j.it.PR, j.r.opt.Viewer, rv.Event, true, j.r.opt.Settings)
	if approve {
		d = j.forApprove(mi)
	}
	if d.Step == plan.StepSkip {
		j.say(Skipped, d.Reason)
		return
	}
	if approve && j.it.Wait {
		d = plan.AfterJobs(d, mi, j.trigger, time.Now(), j.it.IncludeFailing)
		switch d.Step {
		case plan.StepSkip:
			j.say(Skipped, d.Reason)
			return
		case plan.StepWait:
			j.waitToApprove(rv.Body)
			return
		}
	}
	if approve {
		if !j.notExcluded(mi) {
			return
		}
		j.forecast([]Step{StepApprove})
		if j.sendApproval(rv.Body) {
			j.say(Approved, "approved")
		}
		return
	}
	word := reviewWord[rv.Event]
	j.forecast([]Step{StepReview})
	j.step(StepReview, "sending the "+word)
	pr := j.it.PR
	pr.HeadOID = j.head
	if err := j.r.gh.Review(j.ctx, pr, rv.Event, rv.Body); err != nil {
		j.unsure(word + " failed: " + short(err))
		return
	}
	j.say(Reviewed, word+" sent")
}

var reviewWord = map[model.ReviewEvent]string{model.EventComment: "review comment", model.EventRequestChanges: "request for changes"}

// markReady marks a draft ready for review, on the head the user saw.
func (j *job) markReady() {
	mi, ok := j.read(false)
	if !ok {
		return
	}
	if d := plan.ForReady(mi, j.it.PR, j.r.opt.Viewer); d.Step == plan.StepSkip {
		j.say(Skipped, d.Reason)
		return
	}
	j.forecast([]Step{StepReady})
	j.step(StepReady, "marking ready")
	if err := j.r.gh.MarkReady(j.ctx, j.it.PR.Key, j.it.PR.ID, j.head); err != nil {
		j.say(Failed, "ready for review failed: "+short(err))
		return
	}
	j.say(Readied, "ready for review")
}

// convertToDraft turns a pull request back into a draft, on the head the
// user saw.
func (j *job) convertToDraft() {
	mi, ok := j.read(false)
	if !ok {
		return
	}
	if d := plan.ForDraft(mi, j.it.PR, j.r.opt.Viewer); d.Step == plan.StepSkip {
		j.say(Skipped, d.Reason)
		return
	}
	j.forecast([]Step{StepDraft})
	j.step(StepDraft, "converting to draft")
	if err := j.r.gh.ConvertToDraft(j.ctx, j.it.PR.Key, j.it.PR.ID, j.head); err != nil {
		j.say(Failed, "convert to draft failed: "+short(err))
		return
	}
	j.say(Drafted, "converted to draft")
}

// sendApproval approves with body, which may be empty.
func (j *job) sendApproval(body string) bool {
	j.step(StepApprove, "approving")
	// Recorded as on its way first: should gh-desk stop before the
	// result is recorded, the next launch does not know whether the
	// approval reached GitHub, and stops unless it finds it there.
	j.pend.Approving = true
	if !j.save() {
		j.say(Failed, "could not record the pending merge; nothing was sent")
		return false
	}
	pr := j.it.PR
	pr.HeadOID = j.head
	if err := j.r.gh.Review(j.ctx, pr, model.EventApprove, body); err != nil {
		j.unsure("approve failed: " + short(err))
		return false
	}
	j.approved, j.pend.Approving = true, false
	j.approvedAt = time.Now()
	j.save()
	return true
}

// decide reads the pull request and decides its next step, reading again
// while GitHub works out whether it can be merged, and while what this job
// just wrote does not show yet. Every read stops the job if it finds the
// recorded approval dismissed.
func (j *job) decide() (model.MergeInfo, plan.Decision, bool) {
	deadline := time.Now().Add(j.r.opt.RecheckFor)
	// Reading again while GitHub works out whether it can be merged is a
	// wait of its own.
	w := &waitLog{log: j.log}
	for {
		mi, ok := j.read(true)
		if !ok {
			w.end("stopped")
			return mi, plan.Decision{}, false
		}
		if mi.State == model.StateMerged {
			j.say(Merged, "merged")
			return mi, plan.Decision{}, false
		}
		d := plan.ForMerge(mi, j.it.PR, j.r.opt.Viewer, j.r.opt.Settings)
		j.log.Debug("decided", logging.KeyStep, string(d.Step), logging.KeyMergeState, string(mi.MergeState), "approve", d.Approve)
		if j.unseen(d) {
			// A read just after a write may not show it yet: the
			// approval or ready this job sent looks taken back, and
			// stopping would make the user confirm again for nothing.
			w.at("visible", mi, nil)
			j.say(Running, "checking")
			if !j.sleep(j.r.opt.Recheck) {
				w.end("canceled")
				return mi, d, false
			}
			continue
		}
		if j.dismissed(d) || j.redrafted(d) {
			w.end("stopped")
			return mi, d, false
		}
		if d.Step != plan.StepRecheck {
			w.end("ready")
			return mi, d, true
		}
		if time.Now().After(deadline) {
			w.end("timed-out")
			j.say(Skipped, "GitHub could not tell whether it can be merged")
			return mi, d, false
		}
		w.at("mergeable", mi, nil)
		j.say(Running, "checking")
		if !j.sleep(j.r.opt.Recheck) {
			w.end("canceled")
			return mi, d, false
		}
	}
}

func (j *job) merge() {
	mi, d, ok := j.decide()
	if !ok {
		return
	}
	switch d.Step {
	case plan.StepSkip:
		j.say(Skipped, d.Reason)
		return
	case plan.StepQueued:
		j.queued()
		return
	}
	if d.Step == plan.StepAutoMerge && !j.it.AutoMerge {
		// The popup did not say this one goes to auto-merge: merging it
		// here, on the confirmed head, is what the user agreed to.
		d.Step = plan.StepWait
	}
	d = j.hold(mi, d)
	if waits(d.Step) && !j.canWait {
		j.locked("locked by another gh-desk")
		return
	}
	j.forecast(ahead(d))
	// The user's approval the merge starts with is recorded too, so the
	// next launch stops if it was dismissed instead of approving again.
	j.pend.Approved = j.pend.Approved || approvedBy(mi, j.r.opt.Viewer)
	// Recorded before the first write, so a crash after it is resumed; no
	// write without a record.
	if !j.save() {
		j.say(Failed, "could not record the pending merge; nothing was sent")
		return
	}
	if d.Ready && !j.readyToMerge() {
		return
	}
	if d.Approve {
		// An approval is not taken back, so it goes once the jobs passed:
		// a job that fails stops the merge before it.
		if _, d, ok = j.jobsEnd(); !ok {
			return
		}
		if d.Step == plan.StepSkip {
			j.say(Skipped, d.Reason)
			return
		}
		if d.Approve && !j.sendApproval("") {
			return
		}
	}
	if d.Ready || d.Approve {
		// Read again: marking it ready and the approval change what is
		// next.
		if mi, d, ok = j.decide(); !ok {
			return
		}
		if d.Step == plan.StepSkip {
			j.say(Skipped, d.Reason)
			return
		}
		if d.Step == plan.StepAutoMerge && !j.it.AutoMerge {
			d.Step = plan.StepWait
		}
		d = j.hold(mi, d)
		if waits(d.Step) && !j.canWait {
			j.locked("approved or ready; locked by another gh-desk")
			return
		}
		j.forecast(ahead(d))
	}
	switch d.Step {
	case plan.StepMerge, plan.StepEnqueue, plan.StepQueued, plan.StepAutoMerge, plan.StepUpdate:
	case plan.StepWait:
		if !j.wait() {
			return
		}
	default:
		j.say(Skipped, d.Reason)
		return
	}
	j.write()
}

// write takes the lane of the repository for what may move the base, and
// comes back to wait when it finds it must wait after all.
func (j *job) write() {
	for {
		t, ok := j.enterLane()
		if !ok {
			return
		}
		again := j.inTurn()
		j.r.leave(t)
		if !again || !j.wait() {
			return
		}
	}
}

// enterLane waits for a merge the pull request is based on, then for its
// turn in the lane.
func (j *job) enterLane() (*ticket, bool) {
	switch j.r.waitForBase(j.ctx, j.it.PR.Key) {
	case baseCanceled:
		return nil, false
	case baseCycle:
		j.say(Stopped, "its base waits for it · confirm one at a time")
		return nil, false
	}
	j.r.mu.Lock()
	t, ahead := j.r.enter(j.it.PR.Key)
	j.r.mu.Unlock()
	if ahead != nil {
		j.emit(Event{Key: j.it.PR.Key, Text: fmt.Sprintf("after #%d", ahead.Number)})
	}
	select {
	case <-t.turn:
		return t, true
	case <-j.ctx.Done():
		j.r.leave(t)
		return nil, false
	}
}

// inTurn reads the pull request again and writes, holding the lane. It
// reports true when the pull request must wait after all. One whose branch
// it updated keeps the lane until it is merged or given up: another merge
// passing it would leave it behind again.
func (j *job) inTurn() (again bool) {
	updated := false
	for {
		mi, d, ok := j.decide()
		if !ok {
			return false
		}
		if d.Step == plan.StepAutoMerge && !j.it.AutoMerge {
			d.Step = plan.StepWait
		}
		d = j.hold(mi, d)
		jobs := plan.JobsRunning(mi, j.trigger, time.Now())
		// A merge it is based on may have been confirmed since it entered
		// the lane: that one goes first, and this one waits off the lane.
		if d.Step != plan.StepSkip && !jobs && j.r.basePending(j.it.PR.Key) {
			return true
		}
		switch {
		case d.Step == plan.StepSkip:
			j.say(Skipped, d.Reason)
			return false
		case d.Step == plan.StepQueued:
			j.queued()
			return false
		case d.Step == plan.StepMerge && !jobs:
			j.forecast(ahead(d))
			j.mergeDecided(d)
			return false
		case d.Step == plan.StepEnqueue && !jobs:
			j.forecast(ahead(d))
			j.enqueue()
			return false
		case d.Step == plan.StepAutoMerge && !jobs:
			j.forecast(ahead(d))
			j.autoMerge(d.Method, mi.Queue)
			return false
		}
		if waits(d.Step) && !j.canWait {
			j.locked("locked by another gh-desk")
			return false
		}
		switch {
		case d.Step == plan.StepUpdate && !jobs:
			j.forecast(ahead(d))
			if !j.updateBranch(mi.BaseOID) {
				return false
			}
			updated = true
		case !updated:
			return true
		}
		if !j.wait() {
			return false
		}
	}
}

// redrafted stops the job when a decision asks to mark the pull request
// ready although gh-desk did: someone turned it back to a draft, and
// marking it again would undo them.
func (j *job) redrafted(d plan.Decision) bool {
	if !d.Ready || (!j.pend.Readied && !j.readied) {
		return false
	}
	j.say(Stopped, "a draft again · confirm again")
	return true
}

// readyToMerge marks the draft ready for review, on the head the user saw,
// as the first step of its merge.
func (j *job) readyToMerge() bool {
	j.step(StepReady, "marking ready")
	// Recorded first, as an approval is: should gh-desk stop before the
	// result is known, a draft found at the next launch stops the merge.
	j.pend.Readied = true
	if !j.save() {
		j.say(Failed, "could not record the pending merge; nothing was sent")
		return false
	}
	if err := j.r.gh.MarkReady(j.ctx, j.it.PR.Key, j.it.PR.ID, j.head); err != nil {
		j.unsure("ready for review failed: " + short(err))
		return false
	}
	j.readied, j.readiedAt = true, time.Now()
	j.trigger = plan.Trigger{At: time.Now(), Always: true, For: j.r.opt.Quiet}
	return true
}

// unseen reports whether a decision asks again for what this job wrote a
// moment ago (RecheckFor): GitHub may answer a read just after a write from
// before it. Past that time, dismissed and redrafted take it as undone.
func (j *job) unseen(d plan.Decision) bool {
	recent := func(at time.Time) bool { return !at.IsZero() && time.Since(at) < j.r.opt.RecheckFor }
	return (d.Approve && j.approved && recent(j.approvedAt)) || (d.Ready && j.readied && recent(j.readiedAt))
}

// dismissed stops the job when a decision asks to approve although the
// user's approval was recorded: it was dismissed, and approving again
// would undo whoever dismissed it.
func (j *job) dismissed(d plan.Decision) bool {
	if !d.Approve || (!j.pend.Approved && !j.approved) {
		return false
	}
	j.say(Stopped, "your approval was dismissed · confirm again")
	return true
}

// mergeDecided merges as decided, saying first when it goes past the base
// branch's rules as an admin.
func (j *job) mergeDecided(d plan.Decision) {
	if d.Admin {
		j.log.Info("merging as admin", "method", string(d.Method))
	}
	j.mergeNow(d.Method)
}

func (j *job) mergeNow(method model.MergeMethod) bool {
	j.step(StepMerge, "merging")
	err := j.r.gh.Merge(j.ctx, j.it.PR.Key, j.it.PR.ID, j.head, method)
	if err == nil {
		j.emit(Event{Key: j.it.PR.Key, Result: Merged, Text: "merged", Approved: j.approved, Readied: j.readied, Method: method})
		j.log.Info("pr end", logging.KeyStep, resultWord[Merged], "method", string(method))
		j.forget()
		return true
	}
	// A refused merge is read again to tell why: the head moved, it was
	// merged meanwhile, or it is not mergeable yet.
	mi, ok := j.read(false)
	switch {
	case !ok:
	case mi.State == model.StateMerged:
		j.say(Merged, "merged")
		return true
	case mi.InQueue:
		j.queued()
	default:
		j.say(Failed, "merge failed: "+short(err))
	}
	return false
}

// enqueue puts the pull request in its base branch's merge queue, on the
// confirmed head, and watches it there.
func (j *job) enqueue() {
	j.step(StepEnqueue, "adding to the merge queue")
	err := j.r.gh.Enqueue(j.ctx, j.it.PR.Key, j.it.PR.ID, j.head)
	if err == nil {
		j.queued()
		return
	}
	// A refused one is read again to tell why: it was merged meanwhile,
	// or someone put it in the queue first.
	mi, ok := j.read(false)
	switch {
	case !ok:
	case mi.State == model.StateMerged:
		j.say(Merged, "merged")
	case mi.InQueue:
		j.queued()
	default:
		j.say(Failed, "could not add to the merge queue: "+short(err))
	}
}

// queued ends the job's part with the pull request in the merge queue,
// and watches it there when this gh-desk holds the record; without it the
// queue merges it unwatched.
func (j *job) queued() {
	if !j.canWait {
		j.say(Queued, "in the merge queue")
		return
	}
	j.pend.Watching, j.pend.Queued = true, true
	j.save()
	j.say(Queued, "in the merge queue")
	j.startWatch()
}

func (j *job) autoMerge(method model.MergeMethod, queue bool) {
	j.step(StepAutoMerge, "turning on auto-merge")
	err := j.r.gh.EnableAutoMerge(j.ctx, j.it.PR.Key, j.it.PR.ID, j.head, method)
	if errors.Is(err, ErrCleanStatus) {
		if queue {
			j.enqueue()
		} else {
			j.mergeNow(method)
		}
		return
	}
	if err != nil {
		j.unsure("auto-merge failed: " + short(err))
		return
	}
	j.pend.Watching = true
	j.save()
	j.say(AutoMergeOn, "auto-merge on")
	j.startWatch()
}

// startWatch watches aside; the watch releases the pull request when it
// ends.
func (j *job) startWatch() {
	j.watching = true
	j.wg.Go(func() {
		defer j.r.release(j.it.PR.Key)
		j.watch()
	})
}

// watch follows a pull request put on auto-merge or in the merge queue
// until GitHub merges it, it is closed, auto-merge is turned off, it leaves
// the queue unmerged, or the watch times out. On a branch with a merge
// queue, auto-merge puts it in the queue once it can be merged, and is
// off from then on.
func (j *job) watch() {
	last := time.Now()
	// state is GitHub's merge state as last seen; a change is logged, so
	// the log tells what GitHub's auto-merge is waiting on.
	var state model.MergeState
	var inQueue, seen bool
	for j.sleep(j.r.opt.PollSlow) {
		got, err := j.r.gh.MergeInfo(j.ctx, []string{j.it.PR.ID}, false)
		mi, ok := got[j.it.PR.ID]
		if err == nil && ok && mi.State == model.StateOpen && (mi.MergeState != state || mi.InQueue != inQueue || !seen) {
			if !seen {
				j.log.Info("watch start", logging.KeyMergeState, strings.ToLower(string(mi.MergeState)), logging.KeyInQueue, mi.InQueue)
			} else {
				j.log.Info("watch progress", logging.KeyMergeState, strings.ToLower(string(mi.MergeState)), logging.KeyWas, strings.ToLower(string(state)), logging.KeyInQueue, mi.InQueue)
			}
			state, inQueue, seen = mi.MergeState, mi.InQueue, true
		}
		say := func(res Result, text string) {
			j.emit(Event{Key: j.it.PR.Key, Result: res, Text: text, Watch: true, Queued: j.pend.Queued})
			j.log.Info("watch end", logging.KeyStep, resultWord[res], "why", text)
			j.forget()
		}
		now := time.Now()
		j.pend.Watched += now.Sub(last)
		last = now
		j.save()
		switch {
		case j.pend.Watched > j.r.opt.WatchTimeout && inQueue:
			say(Stopped, "stopped watching after "+j.r.opt.WatchTimeout.String()+" · it stays in the merge queue")
			return
		case j.pend.Watched > j.r.opt.WatchTimeout:
			say(Stopped, "stopped watching after "+j.r.opt.WatchTimeout.String()+" · auto-merge stays on")
			return
		case err != nil || !ok:
			continue
		case mi.State == model.StateMerged && j.pend.Queued:
			say(Merged, "merged by the merge queue")
			return
		case mi.State == model.StateMerged:
			say(Merged, "merged by auto-merge")
			return
		case mi.State == model.StateClosed:
			say(Stopped, "closed")
			return
		case mi.InQueue:
			j.pend.Queued = true
		case !mi.AutoMerge && j.pend.Queued:
			say(Stopped, "left the merge queue unmerged")
			return
		case !mi.AutoMerge:
			say(Stopped, "auto-merge was turned off")
			return
		case mi.MergeState == model.MergeDirty:
			say(Stopped, "auto-merge stuck: conflicts")
			return
		case mi.MergeState == model.MergeBehind && !mi.Queue:
			// GitHub's auto-merge does not update a branch; one that must
			// be up to date waits forever.
			say(Stopped, "auto-merge stuck: behind its base")
			return
		}
	}
}

// updateBranch merges the base into the branch and takes the new head as
// the confirmed one, after checking it is the commit GitHub made for this
// update: its parents are exactly the confirmed head and base, the head of
// the base branch read just before, and GitHub made it, authored by the
// viewer. Anything else (a push in between, the base moving) stops the
// pull request.
func (j *job) updateBranch(base string) bool {
	if base == "" {
		j.say(Stopped, "could not read the base branch")
		return false
	}
	j.step(StepUpdate, "updating the branch")
	if err := j.r.gh.UpdateBranch(j.ctx, j.it.PR.Key, j.it.PR.ID, j.head); err != nil {
		j.unsure("update failed: " + short(err))
		return false
	}
	deadline := time.Now().Add(j.r.opt.RecheckFor)
	for j.sleep(j.r.opt.Recheck) {
		got, err := j.r.gh.MergeInfo(j.ctx, []string{j.it.PR.ID}, false)
		mi, ok := got[j.it.PR.ID]
		if err == nil && ok && mi.HeadOID != j.head {
			c, err := j.r.gh.CommitInfo(j.ctx, j.it.PR.Key, mi.HeadOID)
			if err != nil || len(c.Parents) != 2 || !slices.Contains(c.Parents, j.head) || !slices.Contains(c.Parents, base) ||
				!c.ViaWeb || c.Author != j.r.opt.Viewer {
				j.log.Info("not the update's commit", "head", mi.HeadOID, "parents", len(c.Parents), "via_web", c.ViaWeb)
				j.say(Stopped, "head changed · confirm again")
				return false
			}
			j.log.Info("branch updated", "head", mi.HeadOID, "was", j.head)
			j.head = mi.HeadOID
			j.trigger = plan.Trigger{At: time.Now(), Always: true, For: j.r.opt.Quiet}
			return true
		}
		if time.Now().After(deadline) {
			j.say(Failed, "the branch did not update")
			return false
		}
	}
	return false
}

// hold turns a decision to merge, or to turn auto-merge on, into a wait
// while jobs may still start on the head: what ForMerge cannot know, the
// quiet time after what gh-desk or the user did last.
func (j *job) hold(mi model.MergeInfo, d plan.Decision) plan.Decision {
	if (d.Step == plan.StepMerge || d.Step == plan.StepEnqueue || d.Step == plan.StepAutoMerge) && plan.JobsRunning(mi, j.trigger, time.Now()) {
		d.Step, d.Reason = plan.StepWait, "wait for checks → merge"
		if mi.Queue {
			d.Reason = "wait for checks → merge queue"
		}
		if d.Admin {
			d.Reason, d.AdminLater = "wait for checks → merge as admin", true
		}
	}
	return d
}

// waitLog logs a wait when it starts, when what it waits on changes, and
// when it ends; while it waits on jobs or checks, also when one of them
// ends or begins, naming them. Nothing is logged at a read that finds
// them as they were.
type waitLog struct {
	log   *slog.Logger
	on    string
	since time.Time
	// names are what the wait waits on, as last logged, and last the
	// pull request as last read, which tells how they ended.
	names []string
	last  model.MergeInfo
}

// read keeps mi as the last reading, for the end of the wait to tell how
// what it waited on ended.
func (w *waitLog) read(mi model.MergeInfo) { w.last = mi }

// end logs the end of the wait, with what it waited on that ended since
// the last line: the last of the jobs ends the wait, not a line of its own.
func (w *waitLog) end(result string) {
	if w.on == "" {
		return
	}
	attrs := []any{logging.KeyStep, w.on}
	var ended []string
	for _, n := range w.names {
		if o := outcome(w.last, n); o != "" {
			ended = append(ended, n+": "+o)
		}
	}
	if len(ended) > 0 {
		attrs = append(attrs, logging.KeyEnded, strings.Join(ended, ", "))
	}
	attrs = append(attrs, "result", result, logging.KeyDurationMS, time.Since(w.since).Milliseconds())
	w.log.Info("wait end", attrs...)
	w.on, w.names = "", nil
}

// at logs the start of a wait on what, naming what it waits on; a wait
// already on what logs what of names ended, with how it ended in mi, and
// what began.
func (w *waitLog) at(what string, mi model.MergeInfo, names []string) {
	w.last = mi
	if what != w.on {
		w.end("changed")
		w.on, w.since, w.names = what, time.Now(), slices.Clone(names)
		attrs := []any{logging.KeyStep, what}
		if len(names) > 0 {
			attrs = append(attrs, logging.KeyJobs, strings.Join(names, ", "))
		}
		w.log.Info("wait start", attrs...)
		return
	}
	var ended, started []string
	for _, n := range w.names {
		if !slices.Contains(names, n) {
			o := outcome(mi, n)
			if o == "" {
				o = "gone"
			}
			ended = append(ended, n+": "+o)
		}
	}
	for _, n := range names {
		if !slices.Contains(w.names, n) {
			started = append(started, n)
		}
	}
	if len(ended) == 0 && len(started) == 0 {
		return
	}
	w.names = slices.Clone(names)
	attrs := []any{logging.KeyStep, what}
	if len(ended) > 0 {
		attrs = append(attrs, logging.KeyEnded, strings.Join(ended, ", "))
	}
	if len(started) > 0 {
		attrs = append(attrs, logging.KeyStarted, strings.Join(started, ", "))
	}
	attrs = append(attrs, logging.KeyJobs, strings.Join(names, ", "))
	w.log.Info("wait progress", attrs...)
}

// outcome is how the job or check name ended, as mi has it: its
// conclusion in lower case, or "" when it has not ended or is not there.
func outcome(mi model.MergeInfo, name string) string {
	for _, c := range mi.Checks {
		if c.Name == name && (c.Kind != model.KindCheckRun || c.Run == model.RunCompleted) && c.State != model.CheckPending {
			return strings.ToLower(string(c.State))
		}
	}
	for _, r := range mi.Runs {
		if r.Name == name && r.Status == model.RunCompleted {
			return strings.ToLower(string(r.Conclusion))
		}
	}
	return ""
}

// waitingFor says and logs what a wait for the head's work is on: the jobs
// at work, the workflows that may still start in the quiet time, or with
// neither, the required checks still running.
func (j *job) waitingFor(mi model.MergeInfo, w *waitLog) {
	now := time.Now()
	jobs := plan.RunningJobs(mi)
	switch {
	case len(jobs) > 0 || plan.JobsRunning(mi, j.trigger, now) && !plan.InQuiet(mi, j.trigger, now):
		w.at("jobs", mi, jobs)
		j.step(StepWaitChecks, "waiting for checks", jobs...)
	case plan.InQuiet(mi, j.trigger, now):
		w.at("workflows", mi, nil)
		j.step(StepWaitWorkflows, "waiting for workflows to start")
	default:
		checks := plan.RunningChecks(mi, j.r.opt.Settings.ApprovalChecks, false)
		w.at("checks", mi, checks)
		j.step(StepWaitChecks, "waiting for checks", checks...)
	}
}

// waitedForChecks adds the time since last to the time waited for checks
// and records it; it reports false, having stopped the job, once that
// passes WaitTimeout. Checks at work start the wait for other rules over;
// the quiet time alone, with nothing seen at work, does not.
func (j *job) waitedForChecks(mi model.MergeInfo, last *time.Time, w *waitLog) bool {
	now := time.Now()
	if len(plan.RunningJobs(mi)) > 0 || plan.ChecksRunning(mi, j.r.opt.Settings.ApprovalChecks) {
		j.pend.BlockedWaited = 0
	}
	j.pend.ChecksWaited += now.Sub(*last)
	*last = now
	j.save()
	if j.pend.ChecksWaited > j.r.opt.WaitTimeout {
		w.end("timed-out")
		j.say(Stopped, "checks still running after "+j.r.opt.WaitTimeout.String())
		return false
	}
	return true
}

// pause sleeps until the next read: often in the first minute of waiting,
// seldom after. While only jobs are at work, as mi read last shows, the
// waits of the repository share a light read that wakes it when they end.
// It reports false when the batch is canceled.
func (j *job) pause(w *waitLog, mi model.MergeInfo) bool {
	every := j.r.opt.PollFast
	if j.pend.ChecksWaited+j.pend.BlockedWaited > time.Minute {
		every = j.r.opt.PollSlow
	}
	now := time.Now()
	left := max(j.r.opt.WaitTimeout-j.pend.ChecksWaited, 0)
	sleeper := &waiter{id: j.it.PR.ID, head: j.head, light: onlyJobsAtWork(mi), every: every,
		due: now.Add(every), force: now.Add(min(j.r.opt.FullRead, left)), wake: make(chan struct{})}
	if !j.r.awaitRead(j.ctx, j.it.PR.Key.Repository(), sleeper) {
		w.end("canceled")
		return false
	}
	return true
}

// jobsEnd reads the pull request until no job is at work on its head, and
// returns the decision made then. A merge approves only after it: a job
// that fails is a reason to skip, given before the approval.
func (j *job) jobsEnd() (model.MergeInfo, plan.Decision, bool) {
	last := time.Now()
	w := &waitLog{log: j.log}
	for {
		mi, d, ok := j.decide()
		if !ok {
			w.end("stopped")
			return mi, d, false
		}
		w.read(mi)
		if d.Step == plan.StepSkip || !plan.JobsRunning(mi, j.trigger, time.Now()) {
			w.end("ready")
			return mi, d, true
		}
		if !j.canWait {
			j.locked("locked by another gh-desk")
			return mi, d, false
		}
		if !j.waitedForChecks(mi, &last, w) {
			return mi, d, false
		}
		j.waitingFor(mi, w)
		j.forecast(ahead(d))
		if !j.pause(w, mi) {
			return mi, d, false
		}
	}
}

// wait reads the pull request until it is ready to write: no job is at
// work on its head, and it was decided to merge, to turn on auto-merge or
// to update its branch. It reports false when the job ended, as it does
// when jobs, or the required checks, run past WaitTimeout, or when it stays
// blocked BlockedTimeout after they finished.
func (j *job) wait() bool {
	// The times waited carry on from a record, so time gh-desk was not
	// running is not counted.
	last := time.Now()
	w := &waitLog{log: j.log}
	for {
		mi, d, ok := j.decide()
		if !ok {
			w.end("stopped")
			return false
		}
		w.read(mi)
		jobs := plan.JobsRunning(mi, j.trigger, time.Now())
		switch d.Step {
		case plan.StepMerge, plan.StepEnqueue:
			if !jobs {
				w.end("ready")
				return true
			}
		case plan.StepQueued:
			w.end("queued")
			return true
		case plan.StepSkip:
			w.end("skipped")
			j.say(Skipped, d.Reason)
			return false
		case plan.StepAutoMerge:
			// The jobs are done and the popup said this one goes to
			// auto-merge: GitHub merges once its rules allow. Otherwise
			// waiting here is what was asked, so keep waiting.
			if j.it.AutoMerge && !jobs {
				w.end("ready")
				return true
			}
		case plan.StepUpdate:
			// The base moved during the wait. The jobs at work go first:
			// one that fails stops the merge without running them again.
			if !jobs {
				w.end("behind")
				return true
			}
		}
		switch {
		case jobs || plan.ChecksRunning(mi, j.r.opt.Settings.ApprovalChecks):
			if !j.waitedForChecks(mi, &last, w) {
				return false
			}
			j.waitingFor(mi, w)
			j.forecast([]Step{lastStep(mi)})
		default:
			now := time.Now()
			j.pend.BlockedWaited += now.Sub(last)
			last = now
			j.save()
			if j.pend.BlockedWaited > j.r.opt.BlockedTimeout {
				w.end("timed-out")
				j.say(Stopped, "blocked by another rule")
				return false
			}
			what, text, s := "blocked", "waiting", StepWaitBlocked
			var checks []string
			if plan.AwaitingApproval(mi.Checks, j.r.opt.Settings.ApprovalChecks) {
				what, text, s = "approval-checks", "waiting for approval checks", StepWaitApproval
				checks = plan.RunningChecks(mi, j.r.opt.Settings.ApprovalChecks, true)
			}
			w.at(what, mi, checks)
			j.step(s, text, checks...)
			j.forecast([]Step{lastStep(mi)})
		}
		if !j.pause(w, mi) {
			return false
		}
	}
}

// sleep waits d, or returns false when the batch is canceled.
func (j *job) sleep(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-j.ctx.Done():
		return false
	}
}

// short is the first line of an error, for a row.
func short(err error) string {
	s, _, _ := strings.Cut(err.Error(), "\n")
	if len(s) > 80 {
		s = s[:79] + "…"
	}
	return s
}

// comment adds body to the conversation, after reading the pull request
// again as every write does. A comment is not tied to a head, but what the
// user confirmed is: one that moved is stopped as it is for any write.
func (j *job) comment(body string) {
	if _, ok := j.read(false); !ok {
		return
	}
	j.forecast([]Step{StepComment})
	j.step(StepComment, "commenting")
	pr := j.it.PR
	pr.HeadOID = j.head
	if err := j.r.gh.PostComment(j.ctx, pr, body); err != nil {
		j.say(Failed, "comment failed: "+short(err))
		return
	}
	j.emit(Event{Key: j.it.PR.Key, Result: Running, Text: "commented", Commented: true})
	j.say(CommentSent, "commented")
}

// closePR writes the comment, closes the pull request and deletes its head
// branch, as ct asks. It reads the pull request again before each write
// and decides again (ADR-066): neither the close nor the deletion takes an
// expected head on GitHub, so this reading is the only check.
func (j *job) closePR(ct CloseText) {
	mi, ok := j.read(false)
	if !ok {
		return
	}
	based, keep := j.basedOn(mi, ct.DeleteBranch)
	d := plan.ForClose(mi, based, ct.Body != "", ct.DeleteBranch)
	if d.Skip {
		j.say(Skipped, d.Reason)
		return
	}
	if keep != "" {
		d.Delete, d.Keep = false, keep
	}
	steps := []Step{StepClose}
	if ct.Body != "" {
		steps = append([]Step{StepComment}, steps...)
	}
	if d.Delete {
		steps = append(steps, StepDeleteRef)
	}
	j.forecast(steps)
	done := ""
	if ct.Body != "" {
		j.step(StepComment, "commenting")
		pr := j.it.PR
		pr.HeadOID = j.head
		if err := j.r.gh.PostComment(j.ctx, pr, ct.Body); err != nil {
			j.say(Failed, "comment failed: "+short(err))
			return
		}
		j.emit(Event{Key: j.it.PR.Key, Result: Running, Text: "commented", Commented: true})
		done = "commented; "
		// Read again, saying that the comment went out whatever stops the
		// close, so that confirming again is not taken for a comment not
		// sent.
		got, err := j.r.gh.MergeInfo(j.ctx, []string{j.it.PR.ID}, false)
		if mi, ok = got[j.it.PR.ID]; err != nil || !ok {
			j.say(Failed, done+"could not read it again, not closed")
			return
		}
		switch {
		case mi.State != model.StateOpen:
			j.say(Stopped, done+"not open any more")
			return
		case mi.HeadOID != j.head:
			j.say(Stopped, done+"head changed · not closed")
			return
		}
	}
	j.step(StepClose, "closing")
	if err := j.r.gh.Close(j.ctx, j.it.PR.Key, j.it.PR.ID, j.head); err != nil {
		j.say(Failed, done+"close failed: "+short(err))
		return
	}
	j.closed = true
	if !ct.DeleteBranch {
		j.say(Closed, "closed")
		return
	}
	if !d.Delete {
		j.say(Closed, "closed · branch kept: "+d.Keep)
		return
	}
	// Read again: the branch may have moved, or a pull request may have
	// been based on it, since the close was decided.
	got, err := j.r.gh.MergeInfo(j.ctx, []string{j.it.PR.ID}, false)
	if mi, ok = got[j.it.PR.ID]; err != nil || !ok {
		j.say(Failed, "closed; could not read it again, branch kept")
		return
	}
	based, keep = j.basedOn(mi, true)
	if keep == "" {
		keep = plan.KeepBranch(mi, based)
	}
	if keep == "" && mi.HeadOID != j.head {
		keep = "head changed"
	}
	if keep == "" && !mi.CanDeleteHead {
		keep = "GitHub does not offer to delete it"
	}
	if keep != "" {
		j.say(Closed, "closed · branch kept: "+keep)
		return
	}
	j.step(StepDeleteRef, "deleting the branch")
	if err := j.r.gh.DeleteBranch(j.ctx, j.it.PR.Key, mi.HeadRefID, j.head); err != nil {
		j.say(Failed, "closed; deleting the branch failed: "+short(err))
		return
	}
	j.say(Closed, "closed · branch deleted")
}

// basedOn reads the open pull requests based on the head branch, when its
// deletion is asked and it is one that could be deleted. keep says why the
// branch is kept when they could not be read: not knowing whether deleting
// it closes another pull request keeps it.
func (j *job) basedOn(mi model.MergeInfo, want bool) (based []int, keep string) {
	if !want || mi.HeadInFork || mi.HeadRefID == "" || !mi.CanWrite {
		return nil, ""
	}
	got, err := j.r.gh.BasedOn(j.ctx, []model.Branch{mi.Head})
	if err != nil {
		j.log.Info("based on failed", "err", err.Error())
		return nil, "could not tell what is based on it"
	}
	return got[mi.Head], ""
}
