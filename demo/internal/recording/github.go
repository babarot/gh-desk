package recording

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
)

// GitHub is a made-up GitHub, as the TUI and the runner ask for it. It
// answers from the demo's pull requests, a little slower than at once so
// that what is read shows, and keeps the reviews and merges made, so that
// the screen moves on as it would.
type GitHub struct {
	mu     sync.Mutex
	prs    []pr
	notifs []model.Notification
	// Speed scales every wait; 0 answers at once, as tests want.
	Speed float64
}

// NewGitHub is the demo's GitHub as of now.
func NewGitHub(now time.Time) *GitHub {
	all := prs(now)
	return &GitHub{prs: all, notifs: notifications(all), Speed: 1}
}

func (g *GitHub) wait(ctx context.Context, ms int) error {
	if g.Speed <= 0 {
		return ctx.Err()
	}
	select {
	case <-time.After(time.Duration(float64(ms)*g.Speed) * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// find runs f on the pull request with key k or node ID id, under the lock.
func (g *GitHub) find(k model.PRKey, id string, f func(*pr)) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := range g.prs {
		if g.prs[i].Key == k || (id != "" && g.prs[i].ID == id) {
			f(&g.prs[i])
			return true
		}
	}
	return false
}

func (g *GitHub) Viewer(ctx context.Context) (string, error) {
	return Viewer, g.wait(ctx, 300)
}

// SearchPage finds the open pull requests of the default config's tab
// whose query it is.
func (g *GitHub) SearchPage(ctx context.Context, query string, first int, after string) ([]model.PR, int, string, []string, error) {
	if err := g.wait(ctx, 900); err != nil {
		return nil, 0, "", nil, err
	}
	tab := "review"
	switch {
	case strings.Contains(query, "author:@me") && !strings.Contains(query, "-author:@me"):
		tab = "mine"
	case strings.Contains(query, "involves:@me"):
		tab = "involved"
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []model.PR
	for _, p := range g.prs {
		if p.State == model.StateOpen && searchOf(p.PR) == tab {
			out = append(out, p.PR)
		}
	}
	return out, len(out), "", nil, nil
}

func (g *GitHub) Notifications(ctx context.Context, limit int) ([]model.Notification, error) {
	if err := g.wait(ctx, 700); err != nil {
		return nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]model.Notification(nil), g.notifs...), nil
}

func (g *GitHub) PullRequests(ctx context.Context, keys []model.PRKey) (map[model.PRKey]model.PR, error) {
	if err := g.wait(ctx, 400); err != nil {
		return nil, err
	}
	out := map[model.PRKey]model.PR{}
	for _, k := range keys {
		g.find(k, "", func(p *pr) { out[k] = p.PR })
	}
	return out, nil
}

func (g *GitHub) Detail(ctx context.Context, k model.PRKey) (model.Detail, error) {
	if err := g.wait(ctx, 350); err != nil {
		return model.Detail{}, err
	}
	var d model.Detail
	g.find(k, "", func(p *pr) { d = model.Detail{Body: p.body, Checks: p.checks, Comments: p.comments} })
	return d, nil
}

func (g *GitHub) Diff(ctx context.Context, p model.PR) ([]byte, error) {
	if err := g.wait(ctx, 400); err != nil {
		return nil, err
	}
	var d string
	g.find(p.Key, "", func(q *pr) { d = q.diff })
	return []byte(d), nil
}

func (g *GitHub) Comment(ctx context.Context, apiURL string) (model.Comment, error) {
	return model.Comment{}, errors.New("no comment in the demo")
}

func (g *GitHub) Review(ctx context.Context, p model.PR, event model.ReviewEvent, body string) error {
	if err := g.wait(ctx, 600); err != nil {
		return err
	}
	g.find(p.Key, p.ID, func(q *pr) {
		switch event {
		case model.EventApprove:
			q.Reviews = append(q.Reviews, model.Review{Author: Viewer, State: model.ReviewApproved})
			q.Decision = model.DecisionApproved
		case model.EventRequestChanges:
			q.Reviews = append(q.Reviews, model.Review{Author: Viewer, State: model.ReviewChangesRequested})
			q.Decision = model.DecisionChangesRequested
		default:
			q.Reviews = append(q.Reviews, model.Review{Author: Viewer, State: model.ReviewCommented})
		}
		q.Requested = nil
	})
	return nil
}

func (g *GitHub) PostComment(ctx context.Context, p model.PR, body string) error {
	if err := g.wait(ctx, 500); err != nil {
		return err
	}
	g.find(p.Key, p.ID, func(q *pr) { q.Comments++ })
	return nil
}

func (g *GitHub) MarkDone(ctx context.Context, n model.Notification) error {
	if err := g.wait(ctx, 300); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for i, m := range g.notifs {
		if m.ThreadID == n.ThreadID {
			g.notifs = append(g.notifs[:i], g.notifs[i+1:]...)
			break
		}
	}
	return nil
}

// Browse opens nothing: the demo has no browser.
func (g *GitHub) Browse(url string) error { return nil }

func (g *GitHub) MergeInfo(ctx context.Context, ids []string, details bool) (map[string]model.MergeInfo, error) {
	if err := g.wait(ctx, 450); err != nil {
		return nil, err
	}
	out := map[string]model.MergeInfo{}
	for _, id := range ids {
		g.find(model.PRKey{}, id, func(p *pr) {
			mi := model.MergeInfo{Key: p.Key, ID: p.ID, State: p.State, Draft: p.Draft, HeadOID: p.HeadOID, BaseOID: "b45e000",
				MergeState: model.MergeClean, Decision: p.Decision, Rollup: p.Checks, Reviews: p.Reviews,
				Repo:     model.RepoMerge{Methods: []model.MergeMethod{model.MethodMerge, model.MethodSquash}, AutoMerge: true},
				CanClose: true, CanWrite: true, CanDeleteHead: p.State != model.StateOpen, HeadRefID: "ref:" + p.Key.String(),
				Head: model.Branch{Owner: p.Key.Owner, Repo: p.Key.Repo, Name: p.HeadRef}}
			if p.Checks != model.CheckSuccess {
				mi.MergeState = model.MergeBlocked
			}
			if details {
				mi.Checks, mi.Files = p.checks, p.files
			}
			out[id] = mi
		})
	}
	return out, nil
}

func (g *GitHub) Bodies(ctx context.Context, ids []string) (map[string]string, error) {
	if err := g.wait(ctx, 300); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, id := range ids {
		g.find(model.PRKey{}, id, func(p *pr) { out[id] = p.body })
	}
	return out, nil
}

func (g *GitHub) Heavy(ctx context.Context, ids []string) (map[string]model.Heavy, error) {
	if err := g.wait(ctx, 500); err != nil {
		return nil, err
	}
	out := map[string]model.Heavy{}
	for _, id := range ids {
		g.find(model.PRKey{}, id, func(p *pr) {
			h := p.Heavy()
			h.HeadOID = p.HeadOID
			out[id] = h
		})
	}
	return out, nil
}

// Merge merges at once: the demo's checks have all run.
func (g *GitHub) Merge(ctx context.Context, k model.PRKey, id, head string, method model.MergeMethod) error {
	if err := g.wait(ctx, 800); err != nil {
		return err
	}
	g.find(k, id, func(p *pr) { p.State = model.StateMerged })
	return nil
}

// EnableAutoMerge finds every pull request ready, as GitHub does when
// its checks have passed, so the runner merges it directly.
func (g *GitHub) EnableAutoMerge(ctx context.Context, k model.PRKey, id, head string, method model.MergeMethod) error {
	if err := g.wait(ctx, 300); err != nil {
		return err
	}
	return runner.ErrCleanStatus
}

func (g *GitHub) UpdateBranch(ctx context.Context, k model.PRKey, id, head string) error {
	return g.wait(ctx, 600)
}

func (g *GitHub) MarkReady(ctx context.Context, k model.PRKey, id, head string) error {
	if err := g.wait(ctx, 500); err != nil {
		return err
	}
	g.find(k, id, func(p *pr) { p.Draft = false })
	return nil
}

// BasedOn finds no pull request based on another: the recording has no
// stacks.
func (g *GitHub) BasedOn(ctx context.Context, bs []model.Branch) (map[model.Branch][]int, error) {
	return map[model.Branch][]int{}, g.wait(ctx, 200)
}

func (g *GitHub) Close(ctx context.Context, k model.PRKey, id, head string) error {
	if err := g.wait(ctx, 500); err != nil {
		return err
	}
	g.find(k, id, func(p *pr) { p.State = model.StateClosed })
	return nil
}

func (g *GitHub) DeleteBranch(ctx context.Context, k model.PRKey, refID, head string) error {
	return g.wait(ctx, 400)
}

func (g *GitHub) CommitInfo(ctx context.Context, k model.PRKey, oid string) (runner.Commit, error) {
	return runner.Commit{Author: Viewer}, g.wait(ctx, 200)
}
