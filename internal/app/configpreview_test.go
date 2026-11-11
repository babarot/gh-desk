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

// Each row a rule says yes of, or cannot tell, comes with what each of its
// conditions makes of it, in the words of the config file; the rows it says
// no of come with none. The same pull request in the inbox and in a tab is
// judged per row: only the inbox's has the reason.
func TestPreviewSaysWhereARuleMatched(t *testing.T) {
	cfg := model.PR{Key: previewKey(1), Author: "app/renovate", Title: "Configure Renovate", State: model.StateOpen,
		Labels: []string{"deps"}, HeadOID: "h", HeavyHead: "h"}
	fix := model.PR{Key: previewKey(2), Author: "alice", Title: "Fix the cache", State: model.StateMerged, HeadOID: "h"}
	in := &listcache.Inbox{Read: previewNow, PRs: []model.PR{cfg}, Notifs: []model.Notification{{Key: cfg.Key, Reason: "subscribed"}}}
	p := cachedPreview(t, []model.PR{cfg, fix}, in)
	j := judged(t, p, `
[[exclude]]
title = "Con"
reasons = ["subscribed"]

[[exclude]]
state = ["merged"]
labels = ["deps"]
checks = ["passing"]
decision = ["none"]
`)
	e := j.Exclude[0]
	if !slices.Equal(e.Yes, []int{2}) || len(e.Unknown) != 0 || len(e.Why) != 1 {
		t.Fatalf("the title and the reason: %+v", e)
	}
	got := e.Why[2]
	if len(got) != 2 || got[0].Key != "title" || got[0].State != "yes" || got[0].Title == nil || *got[0].Title != [3]string{"", "Con", "figure Renovate"} ||
		got[1].Key != "reasons" || got[1].State != "yes" || !slices.Equal(got[1].Values, []string{"subscribed"}) {
		t.Errorf("why %+v", got)
	}
	e = j.Exclude[1]
	if len(e.Yes) != 0 || !slices.Equal(e.Unknown, []int{1}) {
		t.Fatalf("heavy fields not read: %+v", e)
	}
	got = e.Why[1]
	if len(got) != 4 || got[0].Key != "state" || got[0].State != "yes" || !slices.Equal(got[0].Values, []string{"merged"}) ||
		got[1].Key != "decision" || got[1].State != "unknown" || got[1].Need != "heavy" {
		t.Errorf("why %+v", got)
	}
	// The heavy fields read: a pull request with no checks passes, and says
	// it has none.
	fix.HeavyHead, fix.Labels = "h", []string{"Deps"}
	j = judged(t, cachedPreview(t, []model.PR{fix}, nil), "[[exclude]]\nlabels = [\"deps\"]\nchecks = [\"passing\"]\n")
	got = j.Exclude[0].Why[0]
	if len(got) != 2 || !slices.Equal(got[0].Values, []string{"Deps"}) || !slices.Equal(got[0].Matched, []string{"deps"}) ||
		got[1].Key != "checks" || !slices.Equal(got[1].Values, []string{"none"}) {
		t.Errorf("why %+v", got)
	}
}

// Each row of a tab comes with the section it is in and what that
// section's conditions make of it; one in other as what no section takes
// comes with none.
func TestPreviewSaysWhyARowIsInItsSection(t *testing.T) {
	bot := model.PR{Key: previewKey(1), Author: "app/renovate", AuthorIsBot: true, State: model.StateOpen, HeadOID: "h", HeavyHead: "h"}
	person := model.PR{Key: previewKey(2), Author: "alice", State: model.StateOpen, HeadOID: "h", HeavyHead: "h"}
	p := cachedPreview(t, []model.PR{bot, person}, nil)
	j := judged(t, p, `
[[tabs]]
name = "r"
query = "is:open"
sections = [ { name = "bots", bot = true, show = "folded" } ]

[[tabs]]
kind = "inbox"
`)
	pl := j.Tabs[0]
	if !slices.Equal(pl.Names, []string{"bots", "other"}) || !slices.Equal(pl.At, []int{0, 1}) {
		t.Fatalf("placement %+v", pl)
	}
	if w := pl.Why[0]; len(w) != 1 || w[0].Key != "bot" || w[0].State != "yes" {
		t.Errorf("why of the bot's = %+v", w)
	}
	if _, ok := pl.Why[1]; ok {
		t.Errorf("the row in other has a why: %+v", pl.Why[1])
	}
}
