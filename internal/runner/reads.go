package runner

import (
	"context"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

// waiter is a job asleep until its next read.
type waiter struct {
	id   string
	head string
	// light is whether the light read may leave it asleep: only jobs at
	// work kept it waiting at its last full read.
	light bool
	every time.Duration
	// due is when it is to be woken or read; force is when it is woken
	// whatever the light read says.
	due, force time.Time
	wake       chan struct{}
}

// repoReads are the waits of one repository, which share one light read.
// Its fields are guarded by the runner's mu, but kick.
type repoReads struct {
	waiters map[*waiter]bool
	// kick makes the loop look at the waiters again.
	kick chan struct{}
	// cancel stops the read under way, once nobody waits for it.
	cancel context.CancelFunc
}

func (rr *repoReads) poke() {
	select {
	case rr.kick <- struct{}{}:
	default:
	}
}

// awaitRead sleeps until w should read its pull request, and reports false
// when ctx ends first.
func (r *Runner) awaitRead(ctx context.Context, repo string, w *waiter) bool {
	r.mu.Lock()
	rr := r.reads[repo]
	if rr == nil {
		rr = &repoReads{waiters: map[*waiter]bool{}, kick: make(chan struct{}, 1)}
		r.reads[repo] = rr
		// A job of a batch not ended is here, so the count is above zero.
		r.active.Go(func() { r.readLoop(context.WithoutCancel(ctx), repo, rr) })
	}
	rr.waiters[w] = true
	r.mu.Unlock()
	rr.poke()
	select {
	case <-w.wake:
		return true
	case <-ctx.Done():
		r.mu.Lock()
		delete(rr.waiters, w)
		if len(rr.waiters) == 0 && rr.cancel != nil {
			rr.cancel()
		}
		r.mu.Unlock()
		rr.poke()
		return false
	}
}

// readLoop serves a repository's waits until none is left.
func (r *Runner) readLoop(ctx context.Context, repo string, rr *repoReads) {
	for {
		r.mu.Lock()
		if len(rr.waiters) == 0 {
			delete(r.reads, repo)
			r.mu.Unlock()
			return
		}
		var next time.Time
		for w := range rr.waiters {
			if next.IsZero() || w.due.Before(next) {
				next = w.due
			}
		}
		r.mu.Unlock()
		timer := time.NewTimer(time.Until(next))
		select {
		case <-timer.C:
			r.readWaits(ctx, repo, rr)
		case <-rr.kick:
			timer.Stop()
		}
	}
}

// readWaits wakes the waiters that are due. Those the light read may leave
// asleep are read together, due or not, so their reads stay one read: it
// only tells whom to wake, and what decides a merge is the job's full read.
func (r *Runner) readWaits(ctx context.Context, repo string, rr *repoReads) {
	now := time.Now()
	r.mu.Lock()
	var ids []string
	var read []*waiter
	due := false
	for w := range rr.waiters {
		switch {
		case !w.light && !w.due.After(now), !w.force.After(now):
			rr.wakeUp(w)
		case w.light:
			read = append(read, w)
			ids = append(ids, w.id)
			due = due || !w.due.After(now)
		}
	}
	if !due {
		r.mu.Unlock()
		return
	}
	rctx, cancel := context.WithCancel(ctx)
	defer cancel()
	rr.cancel = cancel
	r.mu.Unlock()

	got, err := r.gh.MergeInfo(rctx, ids, false)
	r.log.Debug("waits read", "repo", repo, "prs", len(ids), "ok", err == nil)

	now = time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	rr.cancel = nil
	for _, w := range read {
		if !rr.waiters[w] {
			continue
		}
		mi, ok := got[w.id]
		if err != nil || !ok || !stillAtWork(mi, w.head) {
			rr.wakeUp(w)
			continue
		}
		w.due = now.Add(w.every)
	}
}

// readSoon has the waits of a repository read now, since its base may have
// moved. Called with the runner's mu held.
func (r *Runner) readSoon(repo string) {
	rr := r.reads[repo]
	if rr == nil {
		return
	}
	now := time.Now()
	for w := range rr.waiters {
		w.due = now
	}
	rr.poke()
}

// wakeUp ends w's sleep. Called with the runner's mu held.
func (rr *repoReads) wakeUp(w *waiter) {
	delete(rr.waiters, w)
	close(w.wake)
}

// stillAtWork reports whether a light read shows nothing that a full read
// is needed for: the pull request is open on head, with its checks not
// finished.
func stillAtWork(mi model.MergeInfo, head string) bool {
	return mi.State == model.StateOpen && mi.HeadOID == head &&
		(mi.Rollup == model.CheckPending || mi.Rollup == model.CheckExpected)
}

// onlyJobsAtWork reports whether what has not finished in a full read is
// all jobs at work, so that the rollup turns when they end. A commit status
// waiting for an approval, or a job waiting for its environment, keeps the
// rollup pending after the jobs end: the light read would not wake such a
// pull request.
func onlyJobsAtWork(mi model.MergeInfo) bool {
	if mi.ChecksPartial {
		return false
	}
	atWork := false
	for _, c := range mi.Checks {
		switch {
		case c.Kind == model.KindCheckRun && c.Run == model.RunCompleted:
		case c.Kind == model.KindCheckRun && c.Run != model.RunWaiting:
			atWork = true
		case c.Kind == model.KindCheckRun:
			return false
		case c.State == model.CheckPending || c.State == model.CheckExpected:
			return false
		}
	}
	return atWork
}
