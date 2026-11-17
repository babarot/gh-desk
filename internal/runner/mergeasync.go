package runner

import (
	"fmt"
	"time"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

// merged ends the job as merged by method.
func (j *job) merged(method model.MergeMethod) {
	j.ended = true
	j.emit(Event{Key: j.it.PR.Key, Result: Merged, Text: "merged", Approved: j.approved, Readied: j.readied, Method: method})
	j.log.Info("pr end", logging.KeyStep, resultWord[Merged], "method", string(method))
	j.forget()
}

// goneAfter is how long after asking a merge GitHub may still not know
// it: a constant, as it is GitHub's own delay.
const goneAfter = time.Minute

// mergeAsync asks GitHub's asynchronous merge API to merge the stacked pull
// request, or to put it in the merge queue, and follows the request to its
// result (ADR-129). Only the lowest open pull request of a stack gets here,
// so nothing but it is merged. It reports whether it was merged.
func (j *job) mergeAsync(req model.MergeRequest) bool {
	if j.halted() {
		return false
	}
	if req.Queue {
		j.step(StepEnqueue, "adding to the merge queue")
	} else {
		j.step(StepMerge, "merging")
	}
	deadline := time.Now().Add(j.r.opt.MergeFor)
	for {
		res, err := j.r.gh.MergeAsync(j.ctx, j.it.PR.Key, j.head, req)
		switch {
		case err != nil:
			return j.refused(err)
		case res.Status != model.MergePending || res.ID != "":
			return j.followMerge(res, req.Method)
		}
		// A request asked before is still pending, or the answer was
		// lost and GitHub may have taken this one; GitHub does not name
		// it. It is asked again until GitHub takes it or says why it
		// cannot. The head is the confirmed one, so asking again merges
		// nothing else. A stop asks nothing more (ADR-122): what was
		// sent stays GitHub's, and the pull request tells how it ended.
		if j.halted() {
			j.afterStop()
			return false
		}
		if time.Now().After(deadline) {
			j.unsure("a merge asked before is still pending")
			return false
		}
		if !j.sleepIn(j.waitCtx(), j.r.opt.PollFast) {
			if j.halted() {
				j.afterStop()
			}
			return false
		}
	}
}

// afterStop ends a merge the user stopped while a request may be GitHub's:
// the pull request is read to tell whether it merged.
func (j *job) afterStop() {
	mi, ok := j.read(false)
	switch {
	case !ok:
	case mi.State == model.StateMerged:
		j.say(Merged, "merged")
	case mi.InQueue:
		j.say(Queued, "in the merge queue")
	default:
		j.stoppedByUser("a merge asked may still be GitHub's · ")
	}
}

// refused ends a merge GitHub refused, after reading the pull request
// again to tell why: it was merged meanwhile, or is in the merge queue.
func (j *job) refused(err error) bool {
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

// followMerge follows a merge asked of GitHub's asynchronous merge API to
// its result, holding the lane, for up to MergeFor. Its ID is recorded, so
// the next launch follows it if gh-desk quits meanwhile. A stop does not
// end it: the merge is GitHub's once asked.
func (j *job) followMerge(res model.MergeResult, method model.MergeMethod) bool {
	// A request this job just asked may not show at once; one from the
	// record, already there, was asked long before.
	fresh := j.pend.MergeRequest != res.ID
	if res.Status == model.MergePending {
		j.handed()
		j.pend.MergeRequest = res.ID
		j.save()
		j.step(StepMerge, "GitHub is merging")
	}
	asked := time.Now()
	deadline := asked.Add(j.r.opt.MergeFor)
	for res.Status == model.MergePending {
		if time.Now().After(deadline) {
			j.unsure("GitHub has not said how the merge ended")
			return false
		}
		if !j.sleepIn(j.ctx, j.r.opt.PollFast) {
			return false
		}
		next, err := j.r.gh.MergeStatus(j.ctx, j.it.PR.Key, res.ID)
		switch {
		case err != nil:
			// Read again at the next turn, until MergeFor: the request
			// is kept, as it may still be GitHub's.
			j.log.Warn("could not read the merge", "err", short(err))
			continue
		case next.Status == model.MergeGone && fresh && time.Since(asked) < goneAfter:
			// Just asked, GitHub may not show the request yet.
			continue
		case next.Status == model.MergeGone:
			// GitHub keeps a result for a while only: read the pull
			// request instead, and go on from what it says.
			j.pend.MergeRequest = ""
			j.save()
			mi, ok := j.read(false)
			switch {
			case !ok:
				return false
			case mi.State == model.StateMerged:
				j.merged(method)
				return true
			case mi.InQueue:
				j.queued()
				return false
			}
			j.unsure("GitHub no longer knows the merge asked")
			return false
		}
		res = next
	}
	j.pend.MergeRequest = ""
	switch res.Status {
	case model.MergeMerged:
		j.merged(method)
		return true
	case model.MergeEnqueued:
		j.queued()
	case model.MergeFailed:
		j.say(Failed, "merge failed: "+res.Message)
	default:
		j.unsure(fmt.Sprintf("GitHub said the merge is %q", res.Status))
	}
	return false
}

// handed records that what the job asked is GitHub's to finish: a stop no
// longer applies to it.
func (j *job) handed() {
	if s := j.stop; s != nil {
		s.mu.Lock()
		s.handed = true
		s.mu.Unlock()
	}
}
