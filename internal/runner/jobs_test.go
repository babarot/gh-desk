package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

func checkRun(name string, run model.RunStatus, conclusion model.CheckState) model.Check {
	st := conclusion
	if run != model.RunCompleted {
		st = model.CheckPending
	}
	return model.Check{Name: name, State: st, Kind: model.KindCheckRun, Run: run}
}

// As gh-desk#47 was: no check is required and the tests are at work. The
// merge waits for them, then merges.
func TestMergeWaitsForJobsNotRequired(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	mi.MergeState = model.MergeUnstable
	mi.Checks = []model.Check{checkRun("test (ubuntu-latest)", "IN_PROGRESS", "")}
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n > 3 {
			mi.Checks, mi.MergeState = []model.Check{checkRun("test (ubuntu-latest)", model.RunCompleted, model.CheckSuccess)}, model.MergeClean
		}
	}
	events := runBatch(t, f, Merge, it)[it.PR.Key]
	if e := last(events); e.Result != Merged {
		t.Fatalf("last event = %+v (calls %v)", e, f.calls)
	}
	waited := slices.ContainsFunc(events, func(e Event) bool {
		return e.Step == StepWaitChecks && slices.Equal(e.Checks, []string{"test (ubuntu-latest)"})
	})
	if !waited {
		t.Errorf("did not say it waits for the job: %+v", events)
	}
	if f.reads["1"] <= 3 {
		t.Errorf("merged after %d reads, before the job ended", f.reads["1"])
	}
}

// A merge that has to approve approves once the jobs passed, not before:
// an approval is not taken back.
func TestMergeApprovesAfterTheJobs(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBlocked
	mi.Checks = []model.Check{checkRun("build", "IN_PROGRESS", "")}
	f := newFake(mi)
	var reviewedAt int
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if reviewedAt == 0 && slices.ContainsFunc(f.calls, func(c string) bool { return strings.HasPrefix(c, "review ") }) {
			reviewedAt = n
		}
		if n > 3 {
			mi.Checks = []model.Check{checkRun("build", model.RunCompleted, model.CheckSuccess)}
		}
		if reviewedAt > 0 {
			mi.MergeState = model.MergeClean
		}
	}
	if e := last(runBatch(t, f, Merge, it)[it.PR.Key]); e.Result != Merged || !e.Approved {
		t.Fatalf("last event = %+v (calls %v)", e, f.calls)
	}
	if reviewedAt <= 4 {
		t.Errorf("approved by read %d, before the job ended at read 4", reviewedAt)
	}
}

// A job that fails while the merge waits stops it before the approval.
func TestMergeWithAFailedJobDoesNotApprove(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBlocked
	mi.Checks = []model.Check{checkRun("build", "IN_PROGRESS", "")}
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n > 3 {
			mi.Checks = []model.Check{checkRun("build", model.RunCompleted, model.CheckFailure)}
		}
	}
	e := last(runBatch(t, f, Merge, it)[it.PR.Key])
	if e.Result != Skipped || e.Text != "a job failed: build" {
		t.Errorf("last event = %+v", e)
	}
	if len(f.calls) > 0 {
		t.Errorf("wrote %v", f.calls)
	}
}

// Marking a draft ready may start workflows of its own, on top of the runs
// of its opening: the merge waits the quiet time, sees the job that
// starts, and merges after it.
func TestMergeWaitsForJobsStartedByReady(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	mi.Draft, mi.MergeState = true, model.MergeDraft
	f := newFake(mi)
	var readyAt int
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if mi.Draft {
			return
		}
		if readyAt == 0 {
			readyAt = n
			mi.MergeState = model.MergeClean
		}
		// The workflow run of ready for review shows a read later, and
		// its job ends two reads after.
		switch n - readyAt {
		case 1, 2:
			mi.Checks = []model.Check{checkRun("test", "QUEUED", "")}
		case 3:
			mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}
		}
	}
	if e := last(runBatch(t, f, Merge, it)[it.PR.Key]); e.Result != Merged {
		t.Fatalf("last event = %+v (calls %v)", e, f.calls)
	}
	if n := f.reads["1"] - readyAt; n < 3 {
		t.Errorf("merged %d reads after ready, before the job it started ended", n)
	}
}

// A merge resumed at launch waits the quiet time even when the head has
// runs: gh-desk may have marked it ready just before it quit, and the
// workflows that starts may not show yet.
func TestResumeWaitsForJobsNotShownYet(t *testing.T) {
	mi, _ := pr("1", "api", 1, "me")
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		// Read 1 resumes and read 2 decides; the job shows after.
		switch n {
		case 3, 4:
			mi.Checks = []model.Check{checkRun("test", "QUEUED", "")}
		case 5:
			mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}
		}
	}
	st := newStore()
	p := model.Pending{Key: mi.Key, ID: "1", Head: "h-1", Author: "me", Readied: true}
	if err := st.Put(p); err != nil {
		t.Fatal(err)
	}
	_, ch := newRunner(f, st).Resume(t.Context(), []model.Pending{p})
	if e := last(drain(t, ch)); e.Result != Merged {
		t.Fatalf("last event = %+v (calls %v)", e, f.calls)
	}
	if f.reads["1"] < 5 {
		t.Errorf("merged after %d reads, before the job not shown at first ended", f.reads["1"])
	}
}

func approveWait(it Item) Item {
	it.Wait = true
	return it
}

// An approval asked to wait is sent once the jobs at work on the head
// passed, and recorded while it waits.
func TestApprovalWaitsForJobs(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
	f := newFake(mi)
	st := newStore()
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n == 2 && len(st.now) == 0 {
			t.Error("not recorded while it waits")
		}
		if n > 3 {
			mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}
		}
	}
	_, ch := newRunner(f, st).StartReview(t.Context(), ReviewText{Event: model.EventApprove}, []Item{approveWait(it)})
	es := drain(t, ch)
	if e := last(es); e.Result != Approved {
		t.Fatalf("last event = %+v (calls %v)", e, f.calls)
	}
	if f.reads["1"] <= 3 {
		t.Errorf("approved after %d reads, before the job ended", f.reads["1"])
	}
	if !slices.ContainsFunc(es, func(e Event) bool { return e.Step == StepWaitChecks && slices.Equal(e.Checks, []string{"test"}) }) {
		t.Errorf("did not say it waits for the job: %+v", es)
	}
	if len(st.now) != 0 {
		t.Errorf("record left: %v", st.now)
	}
}

// A job that fails while the approval waits stops it, unless the user let
// failing ones in.
func TestApprovalWithAFailedJob(t *testing.T) {
	for _, include := range []bool{false, true} {
		mi, it := pr("1", "api", 1, "alice")
		mi.Checks = []model.Check{checkRun("lint", "IN_PROGRESS", "")}
		f := newFake(mi)
		f.onRead = func(_ string, mi *model.MergeInfo, n int) {
			if n > 2 {
				mi.Checks = []model.Check{checkRun("lint", model.RunCompleted, model.CheckFailure)}
			}
		}
		it = approveWait(it)
		it.IncludeFailing = include
		e := last(runReview(t, f, ReviewText{Event: model.EventApprove}, it)[it.PR.Key])
		switch {
		case !include && (e.Result != Skipped || e.Text != "a job failed: lint" || len(f.calls) > 0):
			t.Errorf("failing left out: last event = %+v, calls %v", e, f.calls)
		case include && e.Result != Approved:
			t.Errorf("failing let in: last event = %+v", e)
		}
	}
}

// An approval with a body waits as one without does, and is sent with
// its body once the jobs passed; the body is kept in the record while it
// waits, and never logged.
func TestApprovalWithABodyWaits(t *testing.T) {
	const body = "Looks good once the tests pass"
	mi, it := pr("1", "api", 1, "alice")
	mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
	f := newFake(mi)
	st := newStore()
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n == 2 && st.now[mi.Key].Body != body {
			t.Errorf("record = %+v; want the body kept", st.now[mi.Key])
		}
		if n > 3 {
			mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}
		}
	}
	var buf bytes.Buffer
	r := New(f, logging.New(&buf, slog.LevelDebug), Options{Settings: settings, Viewer: "me", Store: st, CanWait: true,
		WaitTimeout: time.Second, PollFast: 5 * time.Millisecond, PollSlow: 5 * time.Millisecond, Quiet: 20 * time.Millisecond})
	_, ch := r.StartReview(t.Context(), ReviewText{Event: model.EventApprove, Body: body}, []Item{approveWait(it)})
	if e := last(drain(t, ch)); e.Result != Approved || f.bodies["1"] != body {
		t.Fatalf("last event = %+v, body sent %q", e, f.bodies["1"])
	}
	if f.reads["1"] <= 3 {
		t.Errorf("approved after %d reads, before the job ended", f.reads["1"])
	}
	if len(st.now) != 0 {
		t.Errorf("record left: %v", st.now)
	}
	if strings.Contains(buf.String(), body) {
		t.Error("the body is in the log")
	}
}

// An approval with a body that waited is sent with its body at the next
// launch.
func TestResumeApprovalWithABody(t *testing.T) {
	mi, _ := pr("1", "api", 1, "alice")
	f := newFake(mi)
	st := newStore()
	p := model.Pending{Kind: model.PendingApprove, Body: "LGTM", Key: mi.Key, ID: "1", Head: "h-1", Author: "alice"}
	if err := st.Put(p); err != nil {
		t.Fatal(err)
	}
	_, ch := newRunner(f, st).Resume(t.Context(), []model.Pending{p})
	if e := last(drain(t, ch)); e.Result != Approved || f.bodies["1"] != "LGTM" {
		t.Errorf("last event = %+v, body sent %q", e, f.bodies["1"])
	}
}

// Another gh-desk holding the record of pending merges leaves no approval
// waiting here, since nothing would go on with it after quitting.
func TestApprovalThatWouldWaitIsLockedOut(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
	f := newFake(mi)
	r := New(f, logging.Discard(), Options{Settings: settings, Viewer: "me", WaitTimeout: time.Second,
		PollFast: 5 * time.Millisecond, Quiet: 20 * time.Millisecond})
	_, ch := r.StartReview(t.Context(), ReviewText{Event: model.EventApprove}, []Item{approveWait(it)})
	if e := last(drain(t, ch)); e.Result != Skipped || !e.Locked || len(f.calls) > 0 {
		t.Errorf("last event = %+v, calls %v", e, f.calls)
	}
}

// An approval that waited goes on at the next launch, and approves once
// the jobs passed.
func TestResumeApproval(t *testing.T) {
	mi, _ := pr("1", "api", 1, "alice")
	mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n > 3 {
			mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}
		}
	}
	st := newStore()
	p := model.Pending{Kind: model.PendingApprove, Key: mi.Key, ID: "1", Head: "h-1", Author: "alice"}
	if err := st.Put(p); err != nil {
		t.Fatal(err)
	}
	_, ch := newRunner(f, st).Resume(t.Context(), []model.Pending{p})
	if e := last(drain(t, ch)); e.Result != Approved || len(st.now) != 0 {
		t.Errorf("last event = %+v, left %v (calls %v)", e, st.now, f.calls)
	}
	if f.reads["1"] <= 3 {
		t.Errorf("approved after %d reads, before the job ended", f.reads["1"])
	}
}

// waitLines are the wait lines of a log, as "msg step key=value ...", with
// the attributes a wait carries.
func waitLines(t *testing.T, log string) []string {
	t.Helper()
	var out []string
	for line := range strings.Lines(log) {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatal(err)
		}
		msg, _ := rec["msg"].(string)
		if !strings.HasPrefix(msg, "wait ") {
			continue
		}
		s := fmt.Sprint(msg, " ", rec[logging.KeyStep])
		for _, k := range []string{logging.KeyEnded, logging.KeyStarted, logging.KeyJobs, "result"} {
			if v, ok := rec[k]; ok {
				s += fmt.Sprintf(" %s=%v", k, v)
			}
		}
		out = append(out, s)
	}
	return out
}

// A wait on jobs names them when it starts, and logs a line when one ends,
// with how, or one begins; the last to end is told by the end of the wait.
// A read that finds them as they were logs nothing.
func TestWaitLogsTheJobsAsTheyChange(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	mi.MergeState = model.MergeUnstable
	mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", ""), checkRun("lint", "IN_PROGRESS", "")}
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		switch {
		case n == 4:
			mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", ""), checkRun("lint", model.RunCompleted, model.CheckSuccess)}
		case n == 6:
			mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess), checkRun("lint", model.RunCompleted, model.CheckSuccess),
				checkRun("gate", "QUEUED", "")}
		case n >= 8:
			mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess), checkRun("lint", model.RunCompleted, model.CheckSuccess),
				checkRun("gate", model.RunCompleted, model.CheckSuccess)}
			mi.MergeState = model.MergeClean
		}
	}
	var buf bytes.Buffer
	runLogged(t, f, logging.New(&buf, slog.LevelInfo), func(ctx context.Context, r *Runner) <-chan Event {
		_, ch := r.Start(ctx, Merge, []Item{it})
		return ch
	})
	want := []string{
		"wait start jobs jobs=test, lint",
		"wait progress jobs ended=lint: success jobs=test",
		"wait progress jobs ended=test: success started=gate jobs=gate",
		"wait end jobs ended=gate: success result=ready",
	}
	if got := waitLines(t, buf.String()); !slices.Equal(got, want) {
		t.Errorf("wait lines =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// The quiet time after marking a draft ready is a wait of its own, on the
// workflows that may still start.
func TestQuietTimeIsAWaitOnWorkflows(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	mi.Draft, mi.MergeState = true, model.MergeDraft
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, _ int) {
		if !mi.Draft {
			mi.MergeState = model.MergeClean
		}
	}
	var buf bytes.Buffer
	var events []Event
	runLogged(t, f, logging.New(&buf, slog.LevelInfo), func(ctx context.Context, r *Runner) <-chan Event {
		_, ch := r.Start(ctx, Merge, []Item{it})
		out := make(chan Event)
		go func() {
			for e := range ch {
				events = append(events, e)
				out <- e
			}
			close(out)
		}()
		return out
	})
	want := []string{"wait start workflows", "wait end workflows result=ready"}
	if got := waitLines(t, buf.String()); !slices.Equal(got, want) {
		t.Errorf("wait lines = %q, want %q", got, want)
	}
	if !slices.ContainsFunc(events, func(e Event) bool { return e.Step == StepWaitWorkflows }) {
		t.Error("did not say it waits for workflows")
	}
}

// A wait on one job tells how it ended when the wait ends.
func TestWaitOnOneJobTellsHowItEnded(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	mi.MergeState = model.MergeUnstable
	mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n > 3 {
			mi.Checks, mi.MergeState = []model.Check{checkRun("test", model.RunCompleted, model.CheckFailure)}, model.MergeClean
		}
	}
	var buf bytes.Buffer
	runLogged(t, f, logging.New(&buf, slog.LevelInfo), func(ctx context.Context, r *Runner) <-chan Event {
		_, ch := r.Start(ctx, Merge, []Item{it})
		return ch
	})
	want := []string{"wait start jobs jobs=test", "wait end jobs ended=test: failure result=skipped"}
	if got := waitLines(t, buf.String()); !slices.Equal(got, want) {
		t.Errorf("wait lines = %q, want %q", got, want)
	}
}

// Reading again while GitHub works out whether a pull request can be
// merged is logged as a wait on its mergeability.
func TestRecheckIsAWait(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	mi.MergeState = model.MergeUnknown
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n > 2 {
			mi.MergeState = model.MergeClean
		}
	}
	var buf bytes.Buffer
	runLogged(t, f, logging.New(&buf, slog.LevelInfo), func(ctx context.Context, r *Runner) <-chan Event {
		_, ch := r.Start(ctx, Merge, []Item{it})
		return ch
	})
	want := []string{"wait start mergeable", "wait end mergeable result=ready"}
	if got := waitLines(t, buf.String()); !slices.Equal(got, want) {
		t.Errorf("wait lines = %q, want %q", got, want)
	}
}

// Watching auto-merge logs GitHub's merge state when it starts and each
// time it changes, so the log tells what auto-merge waits on.
func TestWatchLogsTheMergeState(t *testing.T) {
	mi, _ := pr("1", "api", 1, "renovate")
	mi.MergeState, mi.AutoMerge = model.MergeBlocked, true
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		switch {
		case n == 3:
			mi.MergeState = model.MergeUnstable
		case n >= 5:
			mi.State = model.StateMerged
		}
	}
	st := newStore()
	p := model.Pending{Key: mi.Key, ID: "1", Head: "h-1", Author: "renovate", AuthorIsBot: true, AutoMerge: true, Watching: true}
	if err := st.Put(p); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r := New(f, logging.New(&buf, slog.LevelInfo), Options{Settings: settings, Viewer: "me", Store: st, CanWait: true,
		WaitTimeout: time.Second, WatchTimeout: time.Second, PollFast: 5 * time.Millisecond, PollSlow: 5 * time.Millisecond, Quiet: 20 * time.Millisecond})
	_, ch := r.Resume(t.Context(), []model.Pending{p})
	if e := last(drain(t, ch)); e.Result != Merged {
		t.Fatalf("last event = %+v", e)
	}
	var got []string
	for line := range strings.Lines(buf.String()) {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatal(err)
		}
		if msg, _ := rec["msg"].(string); strings.HasPrefix(msg, "watch ") {
			s := msg
			for _, k := range []string{logging.KeyMergeState, logging.KeyWas} {
				if v, ok := rec[k]; ok {
					s += fmt.Sprintf(" %s=%v", k, v)
				}
			}
			got = append(got, s)
		}
	}
	want := []string{"watch start merge_state=blocked", "watch progress merge_state=unstable was=blocked", "watch end"}
	if !slices.Equal(got, want) {
		t.Errorf("watch lines = %q, want %q", got, want)
	}
}

// A base that moves during the wait leaves the pull request behind, once
// its jobs ended. Its branch is updated, as at the start of a merge, and it
// does not wait out merge.blocked_timeout as if another rule held it.
func TestBehindDuringTheWaitUpdatesTheBranch(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	mi.MergeState = model.MergeUnstable
	mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
	f := newFake(mi)
	f.commits["h-1-updated"] = Commit{Parents: []string{"h-1", "base"}, ViaWeb: true, Author: "me"}
	var updatedAt int
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		switch {
		case mi.HeadOID == "h-1-updated":
			if updatedAt == 0 {
				updatedAt = n
				mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
			}
			if n > updatedAt+8 {
				mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}
				mi.MergeState = model.MergeClean
			}
		case n == 6:
			// Behind while the job still runs: no update yet.
			mi.MergeState = model.MergeBehind
		case n == 10:
			mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}
		}
	}
	evs := runBatch(t, f, Merge, it)
	if e := last(evs[it.PR.Key]); e.Result != Merged {
		t.Fatalf("last event = %+v (calls %v)", e, f.calls)
	}
	if !slices.Contains(f.calls, "merge acme/api#1 h-1-updated MERGE") {
		t.Errorf("did not merge the updated head: %v", f.calls)
	}
}

// A failed job stops a pull request that fell behind, without updating it.
func TestBehindDuringTheWaitWithAFailedJobIsNotUpdated(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	mi.MergeState = model.MergeUnstable
	mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		switch n {
		case 6:
			mi.MergeState = model.MergeBehind
		case 10:
			mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckFailure)}
		}
	}
	evs := runBatch(t, f, Merge, it)
	if e := last(evs[it.PR.Key]); e.Result != Skipped {
		t.Errorf("last event = %+v (calls %v)", e, f.calls)
	}
	if strings.Contains(strings.Join(f.calls, ","), "update") {
		t.Errorf("updated a pull request whose job failed: %v", f.calls)
	}
}
