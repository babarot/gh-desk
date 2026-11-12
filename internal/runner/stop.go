package runner

import (
	"context"
	"sync"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

// stopper is how the user stops the job on one pull request (ADR-122).
// Quitting and stopping both end the job's waits, but only a stop forgets
// its record, so the next launch does not resume it.
type stopper struct {
	// ctx is what the job's waits watch: the batch's context, canceled
	// too by a stop. Reads and writes to GitHub take the batch's context
	// alone, so a stop never cuts a write short.
	ctx    context.Context
	cancel context.CancelFunc
	// mu guards stopped and handed, and orders a stop's forgetting of the
	// record against the job's writing it, so a record forgotten is never
	// written back.
	mu      sync.Mutex
	stopped bool
	// handed is set once the pull request is GitHub's to merge (auto-merge
	// on, or in the merge queue): a stop leaves it alone from then on.
	handed bool
}

func newStopper(ctx context.Context) *stopper {
	ctx, cancel := context.WithCancel(ctx)
	return &stopper{ctx: ctx, cancel: cancel}
}

// Stop asks the jobs on keys to stop at their next wait, before any write
// they have not started, and forgets their records, so the next launch
// does not go on with them. A write already on its way is sent; what was
// sent stays. A pull request already handed to auto-merge or a merge queue
// is left alone, as is one no job of this runner is on. It returns the
// keys it asked to stop.
func (r *Runner) Stop(keys []model.PRKey) []model.PRKey {
	store, _ := r.waiting()
	var out []model.PRKey
	for _, k := range keys {
		r.mu.Lock()
		s := r.stops[k]
		r.mu.Unlock()
		if s == nil {
			continue
		}
		s.mu.Lock()
		if s.handed || s.stopped {
			s.mu.Unlock()
			continue
		}
		s.stopped = true
		s.cancel()
		if store != nil {
			if err := store.Remove(k); err != nil {
				r.log.Warn("could not forget the pending merge", logging.KeyPR, k.String(), "err", err.Error())
			}
		}
		s.mu.Unlock()
		r.log.Info("stop", logging.KeyPR, k.String())
		out = append(out, k)
	}
	return out
}

// halted reports whether the user stopped the job. Checked before every
// write: none starts after a stop.
func (j *job) halted() bool {
	s := j.stop
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped && !s.handed
}

// stoppedByUser ends the job as the user stopped it, after what done says
// was done.
func (j *job) stoppedByUser(done string) {
	j.byUser = true
	j.say(Stopped, done+"stopped by you")
}

// finish is called once by whoever runs the job last, when it returns: a
// job the user stopped that has not said how it ended says so now. Quitting
// says nothing, and keeps the record for the next launch.
func (j *job) finish() {
	if !j.ended && j.halted() {
		j.stoppedByUser("")
	}
}

// handOver records that the pull request is GitHub's to merge now, and
// watched: a stop no longer applies to it, and its record is written even
// when a stop came while the write that handed it over was on its way, so
// the next launch watches it again.
func (j *job) handOver(queued bool) {
	j.pend.Watching = true
	if queued {
		j.pend.Queued = true
	}
	if s := j.stop; s != nil {
		s.mu.Lock()
		s.handed = true
		s.mu.Unlock()
	}
	j.save()
}
