package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/listcache"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

func previewPR(id string, n int, checks model.CheckState) model.PR {
	return model.PR{ID: id, Key: model.PRKey{Owner: "acme", Repo: "api", Number: n}, Title: "Change " + id, Author: "alice",
		State: model.StateOpen, HeadOID: "head" + id, Checks: checks, Decision: model.DecisionReviewRequired,
		Reviews: []model.Review{{Author: "me", State: model.ReviewApproved}}}
}

func TestThePreviewWritesNothing(t *testing.T) {
	ctx := context.Background()
	s := previewSource{}
	for name, err := range map[string]error{
		"review":    s.Review(ctx, model.PR{}, model.EventApprove, "lgtm"),
		"mark done": s.MarkDone(ctx, model.Notification{}),
		"browse":    s.Browse("https://github.com/acme/api/pull/1"),
	} {
		if !errors.Is(err, errPreviewWrites) {
			t.Errorf("%s: %v", name, err)
		}
	}
	items := []runner.Item{{PR: previewPR("1", 1, "")}, {PR: previewPR("2", 2, "")}}
	r := previewRunner{}
	for name, ch := range map[string]<-chan runner.Event{
		"merge":   second(r.Start(ctx, runner.Merge, items)),
		"review":  second(r.StartReview(ctx, runner.ReviewText{}, items)),
		"close":   second(r.StartClose(ctx, runner.CloseText{}, items)),
		"comment": second(r.StartComment(ctx, "hi", items)),
		"resume":  second(r.Resume(ctx, []model.Pending{{Key: items[0].PR.Key}})),
	} {
		n := 0
		for ev := range ch {
			n++
			if ev.Result != runner.Skipped {
				t.Errorf("%s: %s ends %v", name, ev.Key, ev.Result)
			}
		}
		if n == 0 {
			t.Errorf("%s: nothing is told", name)
		}
	}
}

func second[A, B any](_ A, b B) B { return b }

func TestThePreviewReadsNothing(t *testing.T) {
	ctx := context.Background()
	s := previewSource{}
	_, err1 := s.Viewer(ctx)
	_, _, _, _, err2 := s.SearchPage(ctx, "is:pr", 10, "")
	_, err3 := s.Diff(ctx, model.PR{})
	_, err4 := s.Heavy(ctx, []string{"1"})
	for _, err := range []error{err1, err2, err3, err4} {
		if !errors.Is(err, errPreviewReads) {
			t.Errorf("a read is answered: %v", err)
		}
	}
}

func TestThePreviewMergeInfoTakesTheMost(t *testing.T) {
	ok, running := previewPR("1", 1, model.CheckSuccess), previewPR("2", 2, model.CheckPending)
	s := previewSource{byID: map[string]model.PR{"1": ok, "2": running}}
	got, err := s.MergeInfo(context.Background(), []string{"1", "2", "gone"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("answered %d, want 2", len(got))
	}
	mi := got["1"]
	if mi.MergeState != model.MergeClean || !mi.Repo.AutoMerge || len(mi.Repo.Methods) != 3 || mi.ChecksPartial {
		t.Errorf("not the most a merge may do: %+v", mi)
	}
	if len(mi.Reviews) != 1 || mi.Rollup != model.CheckSuccess || mi.HeadOID != "head1" {
		t.Errorf("what the list holds is lost: %+v", mi)
	}
	if !got["2"].ChecksPartial {
		t.Error("checks still running are not waited for")
	}
}

func TestThePreviewListsKeepNothing(t *testing.T) {
	l := savedLists(listcache.Saved{Lists: []listcache.List{{Query: "review", PRs: []model.PR{previewPR("1", 1, "")}}}})
	if err := l.PutList(listcache.List{Query: "mine"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := l.List("mine"); ok {
		t.Error("a list was kept")
	}
	if got, ok := l.List("review"); !ok || len(got.PRs) != 1 {
		t.Errorf("the list last read is not there: %+v", got)
	}
	every := &previewLists{every: &listcache.List{PRs: []model.PR{previewPR("1", 1, "")}}}
	if got, ok := every.List("anything"); !ok || got.Query != "anything" || len(got.PRs) != 1 {
		t.Errorf("the made-up pull requests are not in every tab: %+v", got)
	}
}

func TestThePreviewSettingsAreSafe(t *testing.T) {
	t.Setenv("EDITOR", "vim")
	shown, err := samplePreview(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	term := newPreviewTerminal(shown, config.Default(), keymap.Default())
	set := term.set
	if !set.Offline || set.Editor != "" || !set.CanWait {
		t.Errorf("Offline %v, Editor %q, CanWait %v", set.Offline, set.Editor, set.CanWait)
	}
	if set.Layout != nil || set.Chats != nil || set.History != nil || set.Lock != nil || set.Release != nil {
		t.Error("the preview keeps something")
	}
	if err := term.Apply([]byte("[theme]\ndark = \"nope\"\n")); err == nil {
		t.Error("a file that does not read is taken")
	}
	if err := term.Apply([]byte("[keys]\nno_such_action = [\"x\"]\n")); err == nil {
		t.Error("keys of no action are taken")
	}
}

// screenBuffer is what gh-desk draws, read while it runs.
type screenBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *screenBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *screenBuffer) has(what string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Contains(s.b.String(), what)
}

func waitDrawn(t *testing.T, out *screenBuffer, what string) {
	t.Helper()
	for range 300 {
		if out.has(what) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%q was not drawn", what)
}

func TestThePreviewTerminalFollowsTheFile(t *testing.T) {
	shown, err := samplePreview(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	term := newPreviewTerminal(shown, config.Default(), keymap.Default())
	ctx := t.Context()
	var out screenBuffer
	done := make(chan error, 1)
	go func() { done <- term.Run(ctx, &out) }()
	waitDrawn(t, &out, "review")
	if err := term.Apply([]byte("[[tabs]]\nname = \"zqx\"\nquery = \"is:pr\"\n\n[[tabs]]\nname = \"inbox\"\nkind = \"inbox\"\n")); err != nil {
		t.Fatal(err)
	}
	waitDrawn(t, &out, "zqx")

	// Another run ends this one, and starts with the file last applied.
	var again screenBuffer
	done2 := make(chan error, 1)
	go func() { done2 <- term.Run(ctx, &again) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("the run ended by another says %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a run started did not end the one before")
	}
	waitDrawn(t, &again, "zqx")
	term.Input([]byte("q"))
	select {
	case err := <-done2:
		if err != nil {
			t.Errorf("gh-desk quit with %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("q did not quit gh-desk")
	}
}
