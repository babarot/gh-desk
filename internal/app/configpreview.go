package app

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/config/ui"
	"github.com/babarot/gh-desk/internal/listcache"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/tui"
)

// sampleViewer is the account the made-up pull requests are requested of.
const sampleViewer = "me"

// previewRow is a row of the preview: what is judged, beside what is drawn.
type previewRow struct {
	pr     model.PR
	reason string
}

// preview is what the settings page shows and judges rules on.
type preview struct {
	lists  ui.Lists
	viewer string
	rows   []previewRow
}

func (p *preview) row(pr model.PR, reason string, now time.Time) ui.PR {
	p.rows = append(p.rows, previewRow{pr, reason})
	return pagePR(len(p.rows)-1, pr, p.viewer, reason, now)
}

func (p *preview) list(prs []model.PR, read, now time.Time) ui.List {
	l := ui.List{Read: read, PRs: make([]ui.PR, len(prs))}
	for i, pr := range prs {
		l.PRs[i] = p.row(pr, "", now)
	}
	return l
}

// newPreview reads the lists gh-desk last read, and never GitHub: config
// commands do not reach it (ADR-102). With none, the page has made-up pull
// requests, which it says.
func newPreview(cfg config.File, now time.Time, log *slog.Logger) (*preview, error) {
	saved, err := listcache.Last(filepath.Join(cfg.CacheDir(), "lists.json"))
	if err != nil {
		log.Warn("could not read the lists last read", "err", err.Error())
	}
	if len(saved.Lists) == 0 && saved.Inbox == nil {
		return samplePreview(now)
	}
	p := &preview{viewer: saved.Viewer}
	p.lists = ui.Lists{Viewer: saved.Viewer, Tabs: map[string]ui.List{}}
	for _, l := range saved.Lists {
		p.lists.Tabs[l.Query] = p.list(l.PRs, l.Read, now)
	}
	if in := saved.Inbox; in != nil {
		byKey := map[model.PRKey]model.PR{}
		for _, pr := range in.PRs {
			byKey[pr.Key] = pr
		}
		l := ui.List{Read: in.Read, PRs: []ui.PR{}}
		for _, n := range in.Notifs {
			if pr, ok := byKey[n.Key]; ok {
				l.PRs = append(l.PRs, p.row(pr, n.Reason, now))
			}
		}
		p.lists.Inbox = &l
	}
	return p, nil
}

func samplePreview(now time.Time) (*preview, error) {
	prs, err := samplePRs(now)
	if err != nil {
		return nil, err
	}
	p := &preview{viewer: sampleViewer}
	p.lists = ui.Lists{Viewer: sampleViewer, Sample: p.list(prs, now, now).PRs}
	return p, nil
}

// samplePRs are the made-up pull requests, as if read at now.
func samplePRs(now time.Time) ([]model.PR, error) {
	var prs []model.PR
	if err := json.Unmarshal(ui.SamplePRs, &prs); err != nil {
		return nil, err
	}
	shift := now.Sub(ui.SampleTime)
	for i := range prs {
		prs[i].Created = prs[i].Created.Add(shift)
		prs[i].Updated = prs[i].Updated.Add(shift)
	}
	return prs, nil
}

// pagePR is pr as the page draws it, in the words the rules use.
func pagePR(i int, pr model.PR, viewer, reason string, now time.Time) ui.PR {
	t := tui.Row(pr, viewer, reason, now)
	state := "open"
	switch pr.State {
	case model.StateMerged:
		state = "merged"
	case model.StateClosed:
		state = "closed"
	}
	checks := ""
	switch pr.Checks {
	case model.CheckSuccess:
		checks = "passing"
	case model.CheckFailure, model.CheckError:
		checks = "failing"
	case model.CheckPending, model.CheckExpected:
		checks = "pending"
	}
	decision := "none"
	switch pr.Decision {
	case model.DecisionApproved:
		decision = "approved"
	case model.DecisionChangesRequested:
		decision = "changes_requested"
	case model.DecisionReviewRequired:
		decision = "review_required"
	}
	return ui.PR{Index: i, Repo: pr.Key.Repository(), N: pr.Key.Number, Author: pr.Author, Bot: pr.AuthorIsBot, Title: t.Title,
		Add: pr.Additions, Del: pr.Deletions, Files: pr.ChangedFiles, Draft: pr.Draft, State: state, Checks: checks, Decision: decision,
		Heavy: pr.HeavyRead(), Group: string(plan.GroupOf(pr, viewer)), Why: t.Why, Review: t.Review, Age: t.Age,
		Updated: int(now.Sub(pr.Updated).Hours() / 24), At: float64(pr.Updated.Unix())}
}

// verdicts are the rows a rule says yes of, and those it cannot tell yet.
type verdicts struct {
	Yes     []int `json:"yes"`
	Unknown []int `json:"unknown"`
}

// judgement is what the rules of a file make of the rows, each rule the way
// gh-desk uses it.
type judgement struct {
	Approved    verdicts   `json:"approved"`
	Select      []verdicts `json:"select"`
	Exclude     []verdicts `json:"exclude"`
	AutoApprove []verdicts `json:"auto_approve"`
}

// judge reads src as a config file and judges its rules.
func (p *preview) judge(src []byte, now time.Time) ([]byte, error) {
	cfg, _, err := config.Parse("config.toml", src)
	if err != nil {
		return nil, err
	}
	facts := func(r previewRow) plan.Facts {
		// The body is not kept with the lists, nor the checks that wait
		// for an approval, so a rule that depends on them cannot tell.
		return plan.Facts{Now: now, Reason: r.reason, ApprovalKnown: len(cfg.Checks.Approval) == 0}
	}
	each := func(rule plan.Rule, gate func(model.PR) bool) verdicts {
		v := verdicts{Yes: []int{}, Unknown: []int{}}
		for i, r := range p.rows {
			if gate != nil && !gate(r.pr) {
				continue
			}
			switch rule.Judge(r.pr, p.viewer, facts(r)) {
			case plan.Yes:
				v.Yes = append(v.Yes, i)
			case plan.Pending:
				v.Unknown = append(v.Unknown, i)
			}
		}
		return v
	}
	out := judgement{Approved: each(plan.ApprovedByMe, plan.Selectable), Select: []verdicts{}, Exclude: []verdicts{}, AutoApprove: []verdicts{}}
	for _, s := range cfg.Select {
		out.Select = append(out.Select, each(rule(s.Name, s.Conditions), plan.Selectable))
	}
	for _, e := range cfg.Exclude {
		out.Exclude = append(out.Exclude, each(rule(e.Name, e.Conditions), nil))
	}
	for _, a := range cfg.AutoApprove {
		out.AutoApprove = append(out.AutoApprove, each(rule(a.Name, a.Conditions), nil))
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("cannot write what the rules say: %w", err)
	}
	return b, nil
}
