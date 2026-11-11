package app

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
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
	// shown are the same lists for gh-desk itself on the page.
	shown *previewLists
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
	p := &preview{viewer: saved.Viewer, shown: savedLists(saved)}
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

// SettingsPlayground is the settings page as the site shows it, with no file and the
// made-up pull requests (ADR-103).
func SettingsPlayground() ([]byte, error) {
	shown, err := samplePreview(ui.SampleTime)
	if err != nil {
		return nil, err
	}
	judged, err := shown.judge(nil, ui.SampleTime)
	if err != nil {
		return nil, err
	}
	return ui.Page(ui.Input{File: ui.File{Path: "config.toml"}, Keys: uiKeys(), Themes: tui.Palettes(), Spinners: tui.Spinners(), Lists: shown.lists, Judged: judged, Playground: true})
}

func samplePreview(now time.Time) (*preview, error) {
	prs, err := samplePRs(now)
	if err != nil {
		return nil, err
	}
	p := &preview{viewer: sampleViewer, shown: &previewLists{every: &listcache.List{Total: len(prs), Read: now, PRs: prs}}}
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
	merge := ""
	switch pr.Merge {
	case model.MergeDirty:
		merge = "dirty"
	case model.MergeBehind:
		merge = "behind"
	case model.MergeUnknown:
		merge = "unknown"
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
		Add: pr.Additions, Del: pr.Deletions, Files: pr.ChangedFiles, Draft: pr.Draft, State: state, Checks: checks, Merge: merge, Decision: decision,
		Heavy: pr.HeavyRead(), Why: t.Why, Review: t.Review, Age: t.Age,
		Updated: int(now.Sub(pr.Updated).Hours() / 24), At: float64(pr.Updated.Unix())}
}

// verdicts are the rows a rule says yes of, and those it cannot tell yet,
// with what each condition makes of those rows, by row.
type verdicts struct {
	Yes     []int          `json:"yes"`
	Unknown []int          `json:"unknown"`
	Why     map[int][]cond `json:"why"`
}

// cond is what one condition of a rule makes of a row, in the words of
// the config file.
type cond struct {
	Key string `json:"key"`
	// State is yes, no or unknown; Need what an unknown one waits for:
	// heavy, body or approval.
	State   string     `json:"state"`
	Need    string     `json:"need,omitempty"`
	Values  []string   `json:"values,omitempty"`
	Matched []string   `json:"matched,omitempty"`
	Title   *[3]string `json:"title,omitempty"`
}

// condKey is the key of the config file for the condition c.
func condKey(c plan.Cond) string {
	switch c {
	case plan.CondStates:
		return "state"
	case plan.CondDecisions:
		return "decision"
	}
	return string(c)
}

// explained is what the conditions of rule make of pr, in the words of the
// config file.
func explained(es []plan.Explained) []cond {
	out := make([]cond, 0, len(es))
	for _, e := range es {
		c := cond{Key: condKey(e.Cond), Values: e.Values, Matched: e.Matched, Need: string(e.Need)}
		switch e.Verdict {
		case plan.Yes:
			c.State = "yes"
		case plan.No:
			c.State = "no"
		default:
			c.State = "unknown"
		}
		switch e.Cond {
		case plan.CondStates, plan.CondDecisions:
			c.Values, c.Matched = lower(e.Values), lower(e.Matched)
		case plan.CondChecks:
			c.Values = checkWords(e.Values)
		case plan.CondTitle:
			if e.Verdict == plan.Yes {
				t := e.Title
				c.Title = &t
			}
		}
		out = append(out, c)
	}
	return out
}

func lower(ss []string) []string {
	var out []string
	for _, s := range ss {
		out = append(out, strings.ToLower(s))
	}
	return out
}

// checkWords are the states of checks as the config file says them; a
// pull request with no checks has none, which passes.
func checkWords(ss []string) []string {
	var out []string
	for _, s := range ss {
		switch model.CheckState(s) {
		case model.CheckSuccess:
			s = "passing"
		case model.CheckFailure, model.CheckError:
			s = "failing"
		case model.CheckPending, model.CheckExpected:
			s = "pending"
		case "":
			s = "none"
		default:
			s = strings.ToLower(s)
		}
		out = append(out, s)
	}
	return out
}

// judgement is what the rules of a file make of the rows, each rule the way
// gh-desk uses it.
type judgement struct {
	Approved    verdicts   `json:"approved"`
	Select      []verdicts `json:"select"`
	Exclude     []verdicts `json:"exclude"`
	AutoApprove []verdicts `json:"auto_approve"`
	// Tabs are what each tab's sections make of the rows, tab by tab.
	Tabs []placement `json:"tabs"`
}

// placement is the sections of a tab as its rows are laid out, other
// included (plan.Layout), and the section each row goes in, by row; Pending
// are the rows a section passed over could not tell of yet.
type placement struct {
	Names   []string `json:"names"`
	Show    []string `json:"show"`
	At      []int    `json:"at"`
	Pending []int    `json:"pending"`
	// Why is what the conditions of the section a row is in make of it,
	// by row; none for a row in other as what no section takes.
	Why map[int][]cond `json:"why"`
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
		v := verdicts{Yes: []int{}, Unknown: []int{}, Why: map[int][]cond{}}
		for i, r := range p.rows {
			if gate != nil && !gate(r.pr) {
				continue
			}
			switch rule.Judge(r.pr, p.viewer, facts(r)) {
			case plan.Yes:
				v.Yes = append(v.Yes, i)
			case plan.Pending:
				v.Unknown = append(v.Unknown, i)
			default:
				continue
			}
			v.Why[i] = explained(rule.Explain(r.pr, p.viewer, facts(r)))
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
	out.Tabs = []placement{}
	for _, t := range cfg.Tabs {
		ss, other := plan.Layout(sections(t.Sections))
		pl := placement{Names: []string{}, Show: []string{}, At: make([]int, len(p.rows)), Pending: []int{}, Why: map[int][]cond{}}
		for _, s := range ss {
			pl.Names, pl.Show = append(pl.Names, s.Name), append(pl.Show, string(s.Show))
		}
		for i, r := range p.rows {
			at, pending := plan.PlaceIn(ss, other, r.pr, p.viewer, facts(r))
			pl.At[i] = at
			if pending {
				pl.Pending = append(pl.Pending, i)
			}
			// A row the section named other takes as what no section
			// does has no reason of that section's.
			if at >= 0 && at < len(t.Sections) && ss[at].Rule.Judge(r.pr, p.viewer, facts(r)) == plan.Yes {
				pl.Why[i] = explained(ss[at].Rule.Explain(r.pr, p.viewer, facts(r)))
			}
		}
		out.Tabs = append(out.Tabs, pl)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("cannot write what the rules say: %w", err)
	}
	return b, nil
}
