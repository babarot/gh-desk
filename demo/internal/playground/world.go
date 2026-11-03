// Package playground is a made-up GitHub, and a Claude that answers from a
// script, for the playground on the site: gh-desk runs in the browser
// against them, with nothing sent anywhere. Every repository, person and
// pull request in it is invented.
//
// It is apart from demo/internal/recording, which is the world of the recordings:
// a recording follows a script, while visitors to the site go anywhere,
// so every pull request here has its diff and Claude's answers, and the
// triage works (ADR-047).
//
// The world moves on its own a little, so that the screen has something
// to show: checks still running finish a while after the start, and a
// merge waiting for them goes on when they do.
package playground

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
)

// Viewer is who the visitor is in the demo.
const Viewer = "octocat"

// World is the made-up GitHub. It is safe for concurrent use: the TUI and
// the runner read and write it from their own goroutines.
type World struct {
	mu     sync.Mutex
	now    func() time.Time
	start  time.Time
	prs    []*pull
	notifs []model.Notification
	// pause is how long a call takes, so the spinners are seen.
	pause func(d time.Duration)
}

// pull is a pull request and what GitHub knows about it besides the row.
type pull struct {
	model.PR
	in       []string
	body     string
	checks   []check
	comments []model.Comment
	diff     string
	files    []string
	behind   bool
	auto     bool
	methods  []model.MergeMethod
	answer   string
	followUp string
	draft    draftReview
	// updates counts the branch updates, which move the head.
	updates int
}

// check is a check run; one still running at the start turns into state
// after it has run for after.
type check struct {
	name     string
	state    model.CheckState
	required bool
	after    time.Duration
	// started is when it started again after a branch update; zero is
	// the start of the world.
	started time.Time
}

type draftReview struct {
	event model.ReviewEvent
	body  string
}

// New returns the world as it is at now(): the pull requests of
// fixtures, aged from now.
func New(now func() time.Time) *World {
	w := &World{now: now, start: now(), pause: func(d time.Duration) { time.Sleep(d) }}
	for _, f := range fixtures() {
		w.prs = append(w.prs, f.pull(w.start))
	}
	w.notifs = notifications(w.start)
	return w
}

// Instant makes every call return at once, for tests.
func (w *World) Instant() { w.pause = func(time.Duration) {} }

func (w *World) find(k model.PRKey) *pull {
	for _, p := range w.prs {
		if p.Key == k {
			return p
		}
	}
	return nil
}

func (w *World) byID(id string) *pull {
	for _, p := range w.prs {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// settle brings p up to now: checks that have finished running, and a
// merge that auto-merge was waiting for.
func (w *World) settle(p *pull) {
	now := w.now()
	for i := range p.checks {
		c := &p.checks[i]
		if c.after > 0 {
			from := c.started
			if from.IsZero() {
				from = w.start
			}
			if now.Sub(from) >= c.after {
				c.after = 0
			}
		}
	}
	p.Checks = p.rollup()
	if p.auto && p.State == model.StateOpen && p.mergeState() == model.MergeClean {
		w.merge(p)
	}
}

func (p *pull) checkState(c check) model.CheckState {
	if c.after > 0 {
		return model.CheckPending
	}
	return c.state
}

func (p *pull) rollup() model.CheckState {
	if len(p.checks) == 0 {
		return ""
	}
	got := model.CheckSuccess
	for _, c := range p.checks {
		switch p.checkState(c) {
		case model.CheckFailure, model.CheckError:
			return model.CheckFailure
		case model.CheckPending:
			got = model.CheckPending
		}
	}
	return got
}

func (p *pull) decision() model.ReviewDecision {
	approved := false
	for _, r := range p.Reviews {
		switch r.State {
		case model.ReviewChangesRequested:
			return model.DecisionChangesRequested
		case model.ReviewApproved:
			approved = true
		}
	}
	if approved {
		return model.DecisionApproved
	}
	return model.DecisionReviewRequired
}

func (p *pull) mergeState() model.MergeState {
	switch {
	case p.Draft:
		return model.MergeDraft
	case p.behind:
		return model.MergeBehind
	}
	for _, c := range p.checks {
		if !c.required {
			continue
		}
		if s := p.checkState(c); s != model.CheckSuccess && s != model.CheckSkipped && s != model.CheckNeutral {
			return model.MergeBlocked
		}
	}
	if p.decision() != model.DecisionApproved {
		return model.MergeBlocked
	}
	if p.rollup() == model.CheckFailure {
		return model.MergeUnstable
	}
	return model.MergeClean
}

func (w *World) merge(p *pull) {
	p.State = model.StateMerged
	p.auto = false
	p.Updated = w.now()
}

// row is p as a list shows it, with the heavy fields read.
func (p *pull) row() model.PR {
	r := p.PR
	r.Decision = p.decision()
	r.Checks = p.rollup()
	r.Comments = len(p.comments)
	r.HeavyHead = r.HeadOID
	r.Requested = slices.Clone(p.Requested)
	r.Reviews = slices.Clone(p.Reviews)
	r.Labels = slices.Clone(p.Labels)
	return r
}

// light is p as a search gives it: without the heavy fields, which
// Heavy reads after.
func (p *pull) light() model.PR {
	r := p.row()
	r.SetHeavy(model.Heavy{})
	r.HeavyHead = ""
	return r
}

// tabOf is which of the demo's tabs a search is: the tabs of the config
// file written on first run.
func tabOf(query string) string {
	has := func(q string) bool { return slices.Contains(strings.Fields(query), q) }
	switch {
	case has("review-requested:@me"):
		return "review"
	case has("author:@me"):
		return "mine"
	case has("involves:@me"):
		return "involved"
	}
	return ""
}

// Source is the world as the TUI reads it.
type Source struct{ w *World }

func (w *World) Source() Source { return Source{w} }

func (s Source) Viewer(context.Context) (string, error) {
	s.w.pause(150 * time.Millisecond)
	return Viewer, nil
}

func (s Source) SearchPage(_ context.Context, query string, first int, after string) ([]model.PR, int, string, []string, error) {
	s.w.pause(500 * time.Millisecond)
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	tab := tabOf(query)
	var out []model.PR
	for _, p := range s.w.prs {
		s.w.settle(p)
		if p.State == model.StateOpen && slices.Contains(p.in, tab) {
			out = append(out, p.light())
		}
	}
	slices.SortStableFunc(out, func(a, b model.PR) int { return b.Updated.Compare(a.Updated) })
	return out, len(out), "", nil, nil
}

func (s Source) Notifications(context.Context, int) ([]model.Notification, error) {
	s.w.pause(400 * time.Millisecond)
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	return slices.Clone(s.w.notifs), nil
}

func (s Source) PullRequests(_ context.Context, keys []model.PRKey) (map[model.PRKey]model.PR, error) {
	s.w.pause(300 * time.Millisecond)
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	out := map[model.PRKey]model.PR{}
	for _, k := range keys {
		if p := s.w.find(k); p != nil {
			s.w.settle(p)
			out[k] = p.row()
		}
	}
	return out, nil
}

func (s Source) Detail(_ context.Context, k model.PRKey) (model.Detail, error) {
	s.w.pause(250 * time.Millisecond)
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	p := s.w.find(k)
	if p == nil {
		return model.Detail{}, fmt.Errorf("%s is not in the demo", k)
	}
	s.w.settle(p)
	d := model.Detail{Body: p.body, Comments: slices.Clone(p.comments)}
	for _, c := range p.checks {
		d.Checks = append(d.Checks, model.Check{Name: c.name, State: p.checkState(c), Required: c.required})
	}
	return d, nil
}

func (s Source) Diff(_ context.Context, pr model.PR) ([]byte, error) {
	s.w.pause(300 * time.Millisecond)
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	p := s.w.find(pr.Key)
	if p == nil {
		return nil, fmt.Errorf("%s is not in the demo", pr.Key)
	}
	return []byte(p.diff), nil
}

func (s Source) Comment(_ context.Context, url string) (model.Comment, error) {
	s.w.pause(200 * time.Millisecond)
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	for _, n := range s.w.notifs {
		if n.LatestCommentURL == url {
			if p := s.w.find(n.Key); p != nil && len(p.comments) > 0 {
				return p.comments[len(p.comments)-1], nil
			}
		}
	}
	return model.Comment{}, errors.New("the comment is not in the demo")
}

func (s Source) Review(_ context.Context, pr model.PR, event model.ReviewEvent, body string) error {
	s.w.pause(600 * time.Millisecond)
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	return s.w.review(pr.Key, event, body)
}

func (w *World) review(k model.PRKey, event model.ReviewEvent, body string) error {
	p := w.find(k)
	if p == nil {
		return fmt.Errorf("%s is not in the demo", k)
	}
	if p.State != model.StateOpen {
		return fmt.Errorf("%s is %s", k, strings.ToLower(string(p.State)))
	}
	if p.Author == Viewer && event != model.EventComment {
		return errors.New("you cannot review your own pull request")
	}
	st := model.ReviewCommented
	switch event {
	case model.EventApprove:
		st = model.ReviewApproved
	case model.EventRequestChanges:
		st = model.ReviewChangesRequested
	}
	p.Reviews = slices.DeleteFunc(p.Reviews, func(r model.Review) bool { return r.Author == Viewer })
	p.Reviews = append(p.Reviews, model.Review{Author: Viewer, State: st})
	p.Requested = slices.DeleteFunc(p.Requested, func(r model.Reviewer) bool { return !r.Team && r.Login == Viewer })
	if strings.TrimSpace(body) != "" {
		p.comments = append(p.comments, model.Comment{Author: Viewer, Body: body, Created: w.now()})
	}
	p.Updated = w.now()
	return nil
}

func (s Source) PostComment(_ context.Context, pr model.PR, body string) error {
	s.w.pause(500 * time.Millisecond)
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	p := s.w.find(pr.Key)
	if p == nil {
		return fmt.Errorf("%s is not in the demo", pr.Key)
	}
	p.comments = append(p.comments, model.Comment{Author: Viewer, Body: body, Created: s.w.now()})
	p.Updated = s.w.now()
	return nil
}

func (s Source) MarkDone(_ context.Context, n model.Notification) error {
	s.w.pause(300 * time.Millisecond)
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	s.w.notifs = slices.DeleteFunc(s.w.notifs, func(x model.Notification) bool { return x.ThreadID == n.ThreadID })
	return nil
}

// ErrNoBrowser is what opening a page gives: the demo's repositories are
// made up, so there is no page to open.
var ErrNoBrowser = errors.New("the demo's pull requests are made up, so there is no page to open")

func (s Source) Browse(string) error { return ErrNoBrowser }

func (s Source) MergeInfo(_ context.Context, ids []string, details bool) (map[string]model.MergeInfo, error) {
	s.w.pause(350 * time.Millisecond)
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	return s.w.mergeInfo(ids, details), nil
}

func (w *World) mergeInfo(ids []string, details bool) map[string]model.MergeInfo {
	out := map[string]model.MergeInfo{}
	for _, id := range ids {
		p := w.byID(id)
		if p == nil {
			continue
		}
		w.settle(p)
		mi := model.MergeInfo{Key: p.Key, ID: p.ID, State: p.State, Draft: p.Draft, HeadOID: p.HeadOID,
			BaseOID: oid(p.Key.Repository() + "@" + p.BaseRef), MergeState: p.mergeState(), Decision: p.decision(),
			AutoMerge: p.auto, Rollup: p.rollup(), Repo: model.RepoMerge{Methods: p.methods, AutoMerge: true},
			Reviews: slices.Clone(p.Reviews)}
		if details {
			mi.Checks = []model.Check{}
			for _, c := range p.checks {
				mi.Checks = append(mi.Checks, model.Check{Name: c.name, State: p.checkState(c), Required: c.required})
			}
			mi.Files = slices.Clone(p.files)
		}
		out[id] = mi
	}
	return out
}

func (s Source) Bodies(_ context.Context, ids []string) (map[string]string, error) {
	s.w.pause(250 * time.Millisecond)
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	out := map[string]string{}
	for _, id := range ids {
		if p := s.w.byID(id); p != nil {
			out[id] = p.body
		}
	}
	return out, nil
}

func (s Source) Heavy(_ context.Context, ids []string) (map[string]model.Heavy, error) {
	s.w.pause(700 * time.Millisecond)
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	out := map[string]model.Heavy{}
	for _, id := range ids {
		if p := s.w.byID(id); p != nil {
			s.w.settle(p)
			out[id] = p.row().Heavy()
		}
	}
	return out, nil
}

// Runner is the world as the runner approves and merges in it.
type Runner struct{ w *World }

func (w *World) Runner() Runner { return Runner{w} }

func (r Runner) MergeInfo(ctx context.Context, ids []string, details bool) (map[string]model.MergeInfo, error) {
	return r.w.Source().MergeInfo(ctx, ids, details)
}

func (r Runner) Review(ctx context.Context, pr model.PR, event model.ReviewEvent, body string) error {
	return r.w.Source().Review(ctx, pr, event, body)
}

// errHeadMoved is what GitHub says when the head given is not the head.
var errHeadMoved = errors.New("the head of the pull request moved")

func (r Runner) Merge(_ context.Context, key model.PRKey, _, head string, _ model.MergeMethod) error {
	r.w.pause(800 * time.Millisecond)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	p := r.w.find(key)
	if p == nil {
		return fmt.Errorf("%s is not in the demo", key)
	}
	r.w.settle(p)
	switch {
	case p.HeadOID != head:
		return errHeadMoved
	case p.State != model.StateOpen:
		return fmt.Errorf("%s is %s", key, strings.ToLower(string(p.State)))
	case p.mergeState() != model.MergeClean && p.mergeState() != model.MergeUnstable:
		return fmt.Errorf("%s cannot be merged now (%s)", key, strings.ToLower(string(p.mergeState())))
	}
	r.w.merge(p)
	return nil
}

func (r Runner) EnableAutoMerge(_ context.Context, key model.PRKey, _, head string, _ model.MergeMethod) error {
	r.w.pause(700 * time.Millisecond)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	p := r.w.find(key)
	if p == nil {
		return fmt.Errorf("%s is not in the demo", key)
	}
	r.w.settle(p)
	switch {
	case p.HeadOID != head:
		return errHeadMoved
	case p.mergeState() == model.MergeClean:
		return runner.ErrCleanStatus
	}
	p.auto = true
	return nil
}

func (r Runner) UpdateBranch(_ context.Context, key model.PRKey, _, head string) error {
	r.w.pause(900 * time.Millisecond)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	p := r.w.find(key)
	if p == nil {
		return fmt.Errorf("%s is not in the demo", key)
	}
	if p.HeadOID != head {
		return errHeadMoved
	}
	p.updates++
	p.HeadOID = oid(fmt.Sprintf("%s update %d", p.Key, p.updates))
	p.behind = false
	// The checks run again on the new head.
	for i := range p.checks {
		p.checks[i].after, p.checks[i].started = 12*time.Second, r.w.now()
	}
	p.Updated = r.w.now()
	return nil
}

func (r Runner) MarkReady(_ context.Context, key model.PRKey, _, head string) error {
	r.w.pause(500 * time.Millisecond)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	p := r.w.find(key)
	if p == nil {
		return fmt.Errorf("%s is not in the demo", key)
	}
	if p.HeadOID != head {
		return errHeadMoved
	}
	p.Draft = false
	p.Updated = r.w.now()
	return nil
}

func (r Runner) CommitInfo(_ context.Context, key model.PRKey, oid string) (runner.Commit, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	p := r.w.find(key)
	if p != nil && p.updates > 0 && p.HeadOID == oid {
		return runner.Commit{Parents: []string{"a", "b"}, ViaWeb: true, Author: Viewer}, nil
	}
	author := ""
	if p != nil {
		author = p.Author
	}
	return runner.Commit{Parents: []string{"a"}, Author: author}, nil
}

// oid is a made-up commit ID for s.
func oid(s string) string { return fmt.Sprintf("%x", sha1.Sum([]byte(s))) }
