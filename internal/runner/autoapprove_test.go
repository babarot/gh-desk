package runner

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/model"
)

// autoWait is an approval a rule asked for, which waits for the jobs.
func autoWait(it Item) Item {
	it.Wait, it.Auto = true, true
	return it
}

func approved(f *fakeGH) bool {
	return slices.ContainsFunc(f.calls, func(c string) bool { return strings.HasPrefix(c, "review") || strings.HasPrefix(c, "approve") })
}

// A rule's approval waits for the jobs, then is sent, and is recorded as
// a rule's while it waits.
func TestAutoApprovalWaitsAndIsRecordedAsARules(t *testing.T) {
	mi, it := pr("1", "api", 1, "renovate")
	mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
	f := newFake(mi)
	st := newStore()
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n == 2 {
			if p, ok := st.now[mi.Key]; !ok || !p.Auto {
				t.Errorf("recorded %+v, %v; want a rule's approval", p, ok)
			}
		}
		if n > 3 {
			mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}
		}
	}
	_, ch := newRunner(f, st).StartReview(t.Context(), ReviewText{Event: model.EventApprove}, []Item{autoWait(it)})
	if e := last(drain(t, ch)); e.Result != Approved {
		t.Fatalf("last event = %+v (calls %v)", e, f.calls)
	}
}

// What changes while a rule's approval waits, the head staying, is judged
// again before it is sent: turned back into a draft, the approval
// dismissed by a person, an exclusion that now matches, or one that cannot
// be told.
func TestAutoApprovalJudgedAgainBeforeItIsSent(t *testing.T) {
	tests := []struct {
		name     string
		change   func(mi *model.MergeInfo)
		excluded func(context.Context, model.PR, model.MergeInfo) (string, error)
		why      string
	}{
		{name: "draft", change: func(mi *model.MergeInfo) { mi.Draft = true }, why: "draft"},
		{name: "dismissed", change: func(mi *model.MergeInfo) {
			mi.ViewerReview = &model.ViewerReview{State: model.ReviewDismissed, Commit: mi.HeadOID}
		}, why: "dismissed on this head"},
		{name: "status failed", change: func(mi *model.MergeInfo) {
			mi.Checks = append(mi.Checks, model.Check{Name: "ci/external", State: model.CheckFailure, Kind: model.KindStatus, Required: true})
		}, why: "checks failing"},
		{name: "labeled", excluded: func(_ context.Context, _ model.PR, mi model.MergeInfo) (string, error) {
			if len(mi.Checks) > 0 && mi.Checks[0].Run == model.RunCompleted {
				return "hold", nil
			}
			return "", nil
		}, why: "excluded: hold"},
		{name: "unknown", excluded: func(context.Context, model.PR, model.MergeInfo) (string, error) {
			return "", errors.New("read failed")
		}, why: "could not tell whether it is excluded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mi, it := pr("1", "api", 1, "renovate")
			mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
			f := newFake(mi)
			f.onRead = func(_ string, mi *model.MergeInfo, n int) {
				if n > 3 {
					mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}
					if tt.change != nil {
						tt.change(mi)
					}
				}
			}
			st := newStore()
			r := newRunner(f, st)
			r.opt.Excluded = tt.excluded
			_, ch := r.StartReview(t.Context(), ReviewText{Event: model.EventApprove}, []Item{autoWait(it)})
			e := last(drain(t, ch))
			if e.Result != Skipped || !strings.Contains(e.Text, tt.why) || approved(f) {
				t.Errorf("last event = %+v (calls %v)", e, f.calls)
			}
			if len(st.now) != 0 {
				t.Errorf("record left: %v", st.now)
			}
		})
	}
}

// A person's approval goes on as before: a draft is approved when the user
// confirmed it, and no exclusion is asked.
func TestConfirmedApprovalIsNotJudgedAsARules(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.Draft = true
	f := newFake(mi)
	r := newRunner(f, newStore())
	r.opt.Excluded = func(context.Context, model.PR, model.MergeInfo) (string, error) { return "hold", nil }
	_, ch := r.StartReview(t.Context(), ReviewText{Event: model.EventApprove}, []Item{it})
	if e := last(drain(t, ch)); e.Result != Approved {
		t.Errorf("last event = %+v (calls %v)", e, f.calls)
	}
}

// A rule's approval resumed from the record is judged as a rule's.
func TestResumedAutoApprovalIsJudgedAsARules(t *testing.T) {
	mi, _ := pr("1", "api", 1, "renovate")
	f := newFake(mi)
	st := newStore()
	p := model.Pending{Kind: model.PendingApprove, Auto: true, Key: mi.Key, ID: "1", Head: "h-1", Author: "renovate"}
	if err := st.Put(p); err != nil {
		t.Fatal(err)
	}
	r := newRunner(f, st)
	r.opt.Excluded = func(context.Context, model.PR, model.MergeInfo) (string, error) { return "hold", nil }
	_, ch := r.Resume(t.Context(), []model.Pending{p})
	if e := last(drain(t, ch)); e.Result != Skipped || approved(f) || len(st.now) != 0 {
		t.Errorf("last event = %+v, left %v (calls %v)", e, st.now, f.calls)
	}
}
