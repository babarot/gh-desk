package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/listcache"
	"github.com/babarot/gh-desk/internal/model"
)

var previewNow = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func previewKey(n int) model.PRKey { return model.PRKey{Owner: "o", Repo: "r", Number: n} }

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func cachedPreview(t *testing.T, prs []model.PR, in *listcache.Inbox) *preview {
	t.Helper()
	cfg := config.Default()
	cfg.Cache.Dir = t.TempDir()
	c, err := listcache.Open(filepath.Join(cfg.CacheDir(), "lists.json"), "github.com", "me")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.PutList(listcache.List{Query: "is:open", Read: previewNow, PRs: prs}); err != nil {
		t.Fatal(err)
	}
	if in != nil {
		if err := c.PutInbox(*in); err != nil {
			t.Fatal(err)
		}
	}
	p, err := newPreview(cfg, previewNow, discard())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func judged(t *testing.T, p *preview, src string) judgement {
	t.Helper()
	b, err := p.judge([]byte(src), previewNow)
	if err != nil {
		t.Fatal(err)
	}
	var j judgement
	if err := json.Unmarshal(b, &j); err != nil {
		t.Fatal(err)
	}
	return j
}

func TestPreviewWithNoListsIsMadeUp(t *testing.T) {
	cfg := config.Default()
	cfg.Cache.Dir = t.TempDir()
	p, err := newPreview(cfg, previewNow, discard())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.lists.Sample) == 0 || len(p.lists.Tabs) != 0 || p.viewer != sampleViewer {
		t.Errorf("%+v", p.lists)
	}
}

func TestPreviewOfTheListsLastRead(t *testing.T) {
	key := model.PRKey{Owner: "o", Repo: "r", Number: 1}
	pr := model.PR{Key: key, Title: "a title", State: model.StateOpen, Updated: previewNow.Add(-2 * time.Hour)}
	in := &listcache.Inbox{Read: previewNow, PRs: []model.PR{pr},
		Notifs: []model.Notification{{Key: key, Reason: "review_requested"}, {Key: model.PRKey{Owner: "o", Repo: "r", Number: 2}}}}
	p := cachedPreview(t, []model.PR{pr}, in)
	if p.lists.Sample != nil || p.lists.Viewer != "me" {
		t.Errorf("%+v", p.lists)
	}
	got := p.lists.Tabs["is:open"].PRs
	if len(got) != 1 || got[0].Title != "a title" || got[0].Age != "2h" || got[0].Heavy {
		t.Errorf("%+v", got)
	}
	if l := p.lists.Inbox; l == nil || len(l.PRs) != 1 || l.PRs[0].Why != "review" {
		t.Errorf("%+v", l)
	}
	if len(p.rows) != 2 || p.rows[1].reason != "review_requested" {
		t.Errorf("%+v", p.rows)
	}
}

// What the page counts is what S selects: a draft and a merged pull
// request meet a rule, and S still leaves them out. An exclude rule takes
// them.
func TestPreviewJudgesAsGhDeskUsesTheRule(t *testing.T) {
	open := model.PR{Key: previewKey(1), Author: "a", State: model.StateOpen, Checks: model.CheckSuccess, HeadOID: "h", HeavyHead: "h"}
	draft := open
	draft.Key, draft.Draft = previewKey(2), true
	merged := open
	merged.Key, merged.State = previewKey(3), model.StateMerged
	unread := model.PR{Key: previewKey(4), Author: "a", State: model.StateOpen, HeadOID: "h"}
	p := cachedPreview(t, []model.PR{open, draft, merged, unread}, nil)

	const rules = `
[[select]]
name = "s"
authors = ["a"]
checks = ["failing", "passing"]

[[exclude]]
authors = ["a"]
checks = ["failing", "passing"]
`
	j := judged(t, p, rules)
	if len(j.Select) != 1 || !slices.Equal(j.Select[0].Yes, []int{0}) || !slices.Equal(j.Select[0].Unknown, []int{3}) {
		t.Errorf("select %+v", j.Select)
	}
	if len(j.Exclude) != 1 || !slices.Equal(j.Exclude[0].Yes, []int{0, 1, 2}) || !slices.Equal(j.Exclude[0].Unknown, []int{3}) {
		t.Errorf("exclude %+v", j.Exclude)
	}
}

func TestPreviewCannotTellPackagesOrApproval(t *testing.T) {
	p := cachedPreview(t, []model.PR{{Key: previewKey(1), HeadOID: "h", HeavyHead: "h", State: model.StateOpen, Checks: model.CheckPending}}, nil)
	j := judged(t, p, "[[select]]\nname = \"p\"\npackages = [\"x\"]\n[[select]]\nname = \"c\"\nchecks = [\"pending\"]\n[checks]\napproval = [\"deploy\"]\n")
	if len(j.Select[0].Unknown) != 1 || len(j.Select[1].Unknown) != 1 {
		t.Errorf("%+v", j.Select)
	}
	j = judged(t, p, "[[select]]\nname = \"c\"\nchecks = [\"pending\"]\n")
	if len(j.Select[0].Yes) != 1 {
		t.Errorf("no check waits for an approval: %+v", j.Select)
	}
}

func TestPreviewOfAFileThatDoesNotRead(t *testing.T) {
	p := cachedPreview(t, []model.PR{{Key: previewKey(1)}}, nil)
	if _, err := p.judge([]byte("not toml ["), previewNow); err == nil {
		t.Error("no error")
	}
}
