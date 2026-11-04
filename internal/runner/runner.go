// Package runner reviews, merges and marks ready pull requests, many at
// once, and follows them until each is merged, skipped or stopped. It decides each
// step with internal/plan from information read just before the step, and
// acts only on the head the user confirmed.
//
// Pull requests of different repositories go side by side; those of one
// repository go one at a time, since merging one can leave the next behind
// its base or in conflict. Progress is reported as Events on a channel;
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
	UpdateBranch(ctx context.Context, key model.PRKey, id, head string) error
	MarkReady(ctx context.Context, key model.PRKey, id, head string) error
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
	// ChecksTimeout is how long to wait for required checks still running.
	ChecksTimeout time.Duration
	// BlockedTimeout is how long a pull request may stay blocked after its
	// required checks finished before it is given up as blocked by another
	// rule.
	BlockedTimeout time.Duration
	// WatchTimeout is how long a pull request put on auto-merge is
	// followed, 24h unless set: the merge is GitHub's, so this decides
	// only how long gh-desk watches for the result (ADR-006).
	WatchTimeout time.Duration
	// PollFast is how often a waiting pull request is read in its first
	// minute, PollSlow after it and while watching auto-merge.
	PollFast, PollSlow time.Duration
	// Recheck is how often, and RecheckFor how long, a pull request whose
	// merge state GitHub is still working out is read again.
	Recheck, RecheckFor time.Duration
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
	if o.Recheck == 0 {
		o.Recheck = 3 * time.Second
	}
	if o.RecheckFor == 0 {
		o.RecheckFor = time.Minute
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
)

// ReviewText is the review a Review batch sends to each of its pull
// requests. It is held in memory only: a review batch is not recorded for
// the next launch.
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
}

// Result is where a pull request ended, or Running while it has not.
type Result int

const (
	Running Result = iota
	Approved
	Merged
	AutoMergeOn
	// Readied is a draft marked ready for review.
	Readied
	// Reviewed is a review sent that is not an approval.
	Reviewed
	Skipped
	Stopped
	Failed
)

// Final reports whether the result ends the pull request's part in the
// batch. AutoMergeOn does: GitHub merges it, and a later Event says so.
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
	// Method is how it was merged, on a Merged event gh-desk made.
	Method model.MergeMethod
	// Watch is set on events of a pull request put on auto-merge, after
	// its batch has ended.
	Watch bool
	// Locked is set on a skip because the merge would wait while another
	// gh-desk holds the record of pending merges.
	Locked bool
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

func (r *Runner) claim(k model.PRKey) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.busy[k] {
		return false
	}
	r.busy[k] = true
	return true
}

func (r *Runner) release(k model.PRKey) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.busy, k)
}

// Wait waits for every batch to end. After its context is canceled, a
// batch ends once its jobs have stopped writing to the record.
func (r *Runner) Wait() { r.active.Wait() }

// New returns a Runner.
func New(gh GitHub, log *slog.Logger, opt Options) *Runner {
	return &Runner{gh: gh, opt: opt.withDefaults(), log: log, busy: map[model.PRKey]bool{}, store: opt.Store, canWait: opt.CanWait}
}

// StartReview runs a batch that sends rv to each pull request, as Start
// does a batch of another kind.
func (r *Runner) StartReview(ctx context.Context, rv ReviewText, items []Item) (string, <-chan Event) {
	return r.run(ctx, Review, items, nil, rv)
}

// Start runs a batch of a merge or a ready for review and returns its ID
// and its events. The channel is
// closed when every pull request has a final result and every auto-merge
// it turned on is no longer watched. Canceling ctx stops it all.
func (r *Runner) Start(ctx context.Context, kind Kind, items []Item) (string, <-chan Event) {
	return r.run(ctx, kind, items, nil, ReviewText{})
}

// Resume goes on with the merges recorded by an earlier run. Each pull
// request is read again first: one whose head moved, that was closed, or
// whose approval by gh-desk was dismissed is stopped, not merged.
func (r *Runner) Resume(ctx context.Context, ps []model.Pending) (string, <-chan Event) {
	items := make([]Item, len(ps))
	pend := map[model.PRKey]model.Pending{}
	for i, p := range ps {
		items[i] = Item{PR: model.PR{ID: p.ID, Key: p.Key, Author: p.Author, AuthorIsBot: p.AuthorIsBot, HeadOID: p.Head},
			Head: p.Head, AutoMerge: p.AutoMerge}
		pend[p.Key] = p
	}
	return r.run(ctx, Merge, items, pend, ReviewText{})
}

func (r *Runner) run(ctx context.Context, kind Kind, items []Item, resume map[model.PRKey]model.Pending, rv ReviewText) (string, <-chan Event) {
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
	verb := map[Kind]string{Review: "review", Merge: "merge", Ready: "ready"}[kind]
	if resume != nil {
		verb = "resume"
	}
	attrs := []any{"kind", verb, "prs", len(items), "repos", len(repos)}
	if kind == Review {
		attrs = append(attrs, "event", strings.ToLower(string(rv.Event)))
	}
	log.Info("batch start", attrs...)
	var wg sync.WaitGroup
	r.active.Add(1)
	emit := func(e Event) {
		e.Batch = batch
		select {
		case events <- e:
		case <-ctx.Done():
		}
	}
	for _, repo := range repos {
		its := byRepo[repo]
		wg.Go(func() {
			for i, it := range its[1:] {
				emit(Event{Key: it.PR.Key, Text: fmt.Sprintf("after #%d", its[i].PR.Key.Number)})
			}
			for _, it := range its {
				if ctx.Err() != nil {
					return
				}
				store, canWait := r.waiting()
				j := &job{r: r, ctx: ctx, log: log.With(logging.KeyPR, it.PR.Key.String()), it: it, head: it.Head, emit: emit, wg: &wg,
					store: store, canWait: canWait, record: kind == Merge && store != nil}
				if !r.claim(it.PR.Key) {
					// Its record belongs to the job already on it.
					j.record = false
					j.say(Skipped, "already being merged or watched")
					continue
				}
				switch p, ok := resume[it.PR.Key]; {
				case ok:
					j.pend, j.approved = p, p.Approved
					j.resume()
				case kind == Review:
					j.review(rv)
				case kind == Ready:
					j.markReady()
				default:
					j.merge()
				}
				if !j.watching {
					r.release(it.PR.Key)
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
	// approved is set once this job sent an approval.
	approved bool
	// readied is set once this job marked the draft ready.
	readied bool
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
}

func (j *job) say(res Result, text string) {
	j.emit(Event{Key: j.it.PR.Key, Result: res, Text: text, Approved: j.approved, Readied: j.readied})
	if !res.Final() {
		return
	}
	j.log.Info("pr end", logging.KeyStep, resultWord[res], "why", text)
	// The record goes only with an end that is known: not when gh-desk
	// is quitting, not when a read or a write failed midway, and not when
	// GitHub's auto-merge is left to watch.
	if res != AutoMergeOn && !j.keep && j.ctx.Err() == nil {
		j.forget()
	}
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
		j.say(AutoMergeOn, "auto-merge on")
		// Watched aside, as after turning auto-merge on, so the next
		// pull request of the repository does not wait for it.
		j.startWatch()
		return
	}
	j.merge()
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

var resultWord = map[Result]string{Running: "running", Approved: "approved", Merged: "merged", AutoMergeOn: "auto-merge-on", Readied: "readied", Reviewed: "reviewed",
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
	if d.Step == plan.StepSkip {
		j.say(Skipped, d.Reason)
		return
	}
	if approve {
		if j.sendApproval(rv.Body) {
			j.say(Approved, "approved")
		}
		return
	}
	word := reviewWord[rv.Event]
	j.say(Running, "sending the "+word)
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
	if d := plan.ForReady(mi); d.Step == plan.StepSkip {
		j.say(Skipped, d.Reason)
		return
	}
	j.say(Running, "marking ready")
	if err := j.r.gh.MarkReady(j.ctx, j.it.PR.Key, j.it.PR.ID, j.head); err != nil {
		j.say(Failed, "ready for review failed: "+short(err))
		return
	}
	j.say(Readied, "ready for review")
}

// sendApproval approves with body, which may be empty.
func (j *job) sendApproval(body string) bool {
	j.say(Running, "approving")
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
	j.save()
	return true
}

// decide reads the pull request and decides its next step, reading again
// while GitHub works out whether it can be merged. Every read stops the job
// if it finds the recorded approval dismissed.
func (j *job) decide() (model.MergeInfo, plan.Decision, bool) {
	deadline := time.Now().Add(j.r.opt.RecheckFor)
	for {
		mi, ok := j.read(true)
		if !ok {
			return mi, plan.Decision{}, false
		}
		if mi.State == model.StateMerged {
			j.say(Merged, "merged")
			return mi, plan.Decision{}, false
		}
		d := plan.ForMerge(mi, j.it.PR, j.r.opt.Viewer, j.r.opt.Settings)
		j.log.Debug("decided", logging.KeyStep, string(d.Step), "merge_state", string(mi.MergeState), "approve", d.Approve)
		if j.dismissed(d) || j.redrafted(d) {
			return mi, d, false
		}
		if d.Step != plan.StepRecheck {
			return mi, d, true
		}
		if time.Now().After(deadline) {
			j.say(Skipped, "GitHub could not tell whether it can be merged")
			return mi, d, false
		}
		j.say(Running, "checking")
		if !j.sleep(j.r.opt.Recheck) {
			return mi, d, false
		}
	}
}

func (j *job) merge() {
	mi, d, ok := j.decide()
	if !ok {
		return
	}
	if d.Step == plan.StepSkip {
		j.say(Skipped, d.Reason)
		return
	}
	if d.Step == plan.StepAutoMerge && !j.it.AutoMerge {
		// The popup did not say this one goes to auto-merge: merging it
		// here, on the confirmed head, is what the user agreed to.
		d.Step = plan.StepWait
	}
	if waits(d.Step) && !j.canWait {
		j.locked("locked by another gh-desk")
		return
	}
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
	if d.Approve && !j.sendApproval("") {
		return
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
		if waits(d.Step) && !j.canWait {
			j.locked("approved or ready; locked by another gh-desk")
			return
		}
	}
	switch d.Step {
	case plan.StepMerge:
		j.mergeDecided(d)
	case plan.StepAutoMerge:
		j.autoMerge(d.Method)
	case plan.StepUpdate:
		if j.updateBranch(mi.BaseOID) {
			j.wait()
		}
	case plan.StepWait:
		j.wait()
	default:
		j.say(Skipped, d.Reason)
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
	j.say(Running, "marking ready")
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
	j.readied = true
	return true
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
	j.say(Running, "merging")
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
	default:
		j.say(Failed, "merge failed: "+short(err))
	}
	return false
}

func (j *job) autoMerge(method model.MergeMethod) {
	j.say(Running, "turning on auto-merge")
	err := j.r.gh.EnableAutoMerge(j.ctx, j.it.PR.Key, j.it.PR.ID, j.head, method)
	if errors.Is(err, ErrCleanStatus) {
		j.mergeNow(method)
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

// watch follows a pull request put on auto-merge until GitHub merges it,
// it is closed, auto-merge is turned off, or the watch times out.
func (j *job) watch() {
	last := time.Now()
	for j.sleep(j.r.opt.PollSlow) {
		got, err := j.r.gh.MergeInfo(j.ctx, []string{j.it.PR.ID}, false)
		mi, ok := got[j.it.PR.ID]
		say := func(res Result, text string) {
			j.emit(Event{Key: j.it.PR.Key, Result: res, Text: text, Watch: true})
			j.log.Info("watch end", logging.KeyStep, resultWord[res], "why", text)
			j.forget()
		}
		now := time.Now()
		j.pend.Watched += now.Sub(last)
		last = now
		j.save()
		switch {
		case j.pend.Watched > j.r.opt.WatchTimeout:
			say(Stopped, "stopped watching after "+j.r.opt.WatchTimeout.String()+" · auto-merge stays on")
			return
		case err != nil || !ok:
			continue
		case mi.State == model.StateMerged:
			say(Merged, "merged by auto-merge")
			return
		case mi.State == model.StateClosed:
			say(Stopped, "closed")
			return
		case !mi.AutoMerge:
			say(Stopped, "auto-merge was turned off")
			return
		case mi.MergeState == model.MergeDirty:
			say(Stopped, "auto-merge stuck: conflicts")
			return
		case mi.MergeState == model.MergeBehind:
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
	j.say(Running, "updating the branch")
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
			return true
		}
		if time.Now().After(deadline) {
			j.say(Failed, "the branch did not update")
			return false
		}
	}
	return false
}

// wait reads the pull request until it can be merged, then merges it. It
// gives up when required checks run past ChecksTimeout, or when it stays
// blocked BlockedTimeout after its required checks finished.
func (j *job) wait() {
	// The times waited carry on from a record, so time gh-desk was not
	// running is not counted.
	last := time.Now()
	// What it waits on and since when, for the log: a wait is logged when
	// it starts, when what it waits on changes, and when it ends.
	var on string
	var since time.Time
	end := func(result string) {
		if on != "" {
			j.log.Info("wait end", logging.KeyStep, on, "result", result, logging.KeyDurationMS, time.Since(since).Milliseconds())
			on = ""
		}
	}
	waitOn := func(what string) {
		if what == on {
			return
		}
		end("changed")
		on, since = what, time.Now()
		j.log.Info("wait start", logging.KeyStep, what)
	}
	for {
		mi, d, ok := j.decide()
		if !ok {
			end("stopped")
			return
		}
		switch d.Step {
		case plan.StepMerge:
			end("ready")
			j.mergeDecided(d)
			return
		case plan.StepSkip:
			end("skipped")
			j.say(Skipped, d.Reason)
			return
		case plan.StepAutoMerge:
			// The repository allows auto-merge after all; waiting here is
			// what was asked, so keep waiting.
		}
		running := plan.ChecksRunning(mi, j.r.opt.Settings.ApprovalChecks)
		now := time.Now()
		dt := now.Sub(last)
		last = now
		switch {
		case running:
			j.pend.BlockedWaited = 0
			j.pend.ChecksWaited += dt
			j.save()
			if j.pend.ChecksWaited > j.r.opt.ChecksTimeout {
				end("timed-out")
				j.say(Stopped, "checks still running after "+j.r.opt.ChecksTimeout.String())
				return
			}
			waitOn("checks")
			j.say(Running, "waiting for checks")
		default:
			j.pend.BlockedWaited += dt
			j.save()
			if j.pend.BlockedWaited > j.r.opt.BlockedTimeout {
				end("timed-out")
				j.say(Stopped, "blocked by another rule")
				return
			}
			what, text := "blocked", "waiting"
			if plan.AwaitingApproval(mi.Checks, j.r.opt.Settings.ApprovalChecks) {
				what, text = "approval-checks", "waiting for approval checks"
			}
			waitOn(what)
			j.say(Running, text)
		}
		every := j.r.opt.PollFast
		if j.pend.ChecksWaited+j.pend.BlockedWaited > time.Minute {
			every = j.r.opt.PollSlow
		}
		if !j.sleep(every) {
			end("canceled")
			return
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
