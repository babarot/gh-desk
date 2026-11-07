package runner

import (
	"context"

	"github.com/babarot/gh-desk/internal/model"
)

// activeMerge is a merge at work, and the branches that tell whether
// another pull request is based on it.
type activeMerge struct {
	key  model.PRKey
	head string
	base string
	// fork is set when its head is in another repository: a pull request
	// based on a branch of that name is not based on this one.
	fork bool
	// known is closed once its branches are read or it left; done once it
	// left.
	known chan struct{}
	done  chan struct{}
	read  bool
}

// track notes k as a merge at work, its branches not known yet. Called with
// mu held.
func (r *Runner) track(k model.PRKey) {
	repo := k.Repository()
	if r.merges[repo] == nil {
		r.merges[repo] = map[model.PRKey]*activeMerge{}
	}
	r.merges[repo][k] = &activeMerge{key: k, known: make(chan struct{}), done: make(chan struct{})}
}

// untrack ends a merge at work. Called with mu held.
func (r *Runner) untrack(k model.PRKey) {
	repo := k.Repository()
	m := r.merges[repo][k]
	if m == nil {
		return
	}
	delete(r.merges[repo], k)
	if len(r.merges[repo]) == 0 {
		delete(r.merges, repo)
	}
	m.branchesKnown()
	close(m.done)
}

func (m *activeMerge) branchesKnown() {
	if !m.read {
		m.read = true
		close(m.known)
	}
}

// readBranches reads the branches of the merges of a batch in one
// repository, as they are not recorded for a resume and a list may be old.
// failed says which of them could not be read and why.
func (r *Runner) readBranches(ctx context.Context, prs []model.PR, failed func(i int, why string)) {
	if len(prs) == 0 {
		return
	}
	ids := make([]string, len(prs))
	for i, p := range prs {
		ids[i] = p.ID
	}
	got, err := r.gh.MergeInfo(ctx, ids, false)
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, p := range prs {
		mi, ok := got[p.ID]
		switch {
		case err != nil:
			failed(i, short(err))
		case !ok:
			failed(i, "not found")
		}
		m := r.merges[p.Key.Repository()][p.Key]
		if m == nil {
			continue
		}
		if err == nil && ok {
			m.head, m.base, m.fork = mi.Head.Name, mi.BaseRef, mi.HeadInFork
			m.branchesKnown()
		}
	}
}

// baseWait is how a wait for the merge a pull request is based on ended.
type baseWait int

const (
	baseDone baseWait = iota
	baseCanceled
	// baseCycle is a pull request whose bases lead back to itself.
	baseCycle
)

// waitForBase waits until the branches of the merges at work in the
// repository are known and no merge at work has k's base for its head.
func (r *Runner) waitForBase(ctx context.Context, k model.PRKey) baseWait {
	for {
		r.mu.Lock()
		on, cycle := r.waitsOn(k)
		r.mu.Unlock()
		if cycle {
			return baseCycle
		}
		if on == nil {
			return baseDone
		}
		select {
		case <-on:
		case <-ctx.Done():
			return baseCanceled
		}
	}
}

// basePending reports whether k may not write yet for a merge at work.
func (r *Runner) basePending(k model.PRKey) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	on, cycle := r.waitsOn(k)
	return on != nil || cycle
}

// waitsOn is what k must wait for, nil for nothing, or whether its bases
// lead back to it. Called with mu held.
func (r *Runner) waitsOn(k model.PRKey) (<-chan struct{}, bool) {
	ms := r.merges[k.Repository()]
	for _, m := range ms {
		if !m.read {
			return m.known, false
		}
	}
	self := ms[k]
	if self == nil {
		return nil, false
	}
	parent := baseOf(ms, self)
	if parent == nil {
		return nil, false
	}
	seen := map[*activeMerge]bool{}
	for p := parent; p != nil && !seen[p]; p = baseOf(ms, p) {
		if p == self {
			return nil, true
		}
		seen[p] = true
	}
	return parent.done, false
}

// baseOf is the merge at work whose head is m's base.
func baseOf(ms map[model.PRKey]*activeMerge, m *activeMerge) *activeMerge {
	for _, o := range ms {
		if o != m && !o.fork && o.head != "" && o.head == m.base {
			return o
		}
	}
	return nil
}
