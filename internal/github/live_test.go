package github

import (
	"context"
	"os"
	"testing"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

// TestLiveMergeInfo reads a public pull request from GitHub with gh's
// login. It only reads. Run it with GH_DESK_LIVE=owner/repo#n.
func TestLiveMergeInfo(t *testing.T) {
	target := os.Getenv("GH_DESK_LIVE")
	if target == "" {
		t.Skip("set GH_DESK_LIVE=owner/repo#n to read a pull request from GitHub")
	}
	key, err := model.ParsePRKey(target)
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(logging.Discard(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	ps, err := c.PullRequests(context.Background(), []model.PRKey{key})
	if err != nil || len(ps) != 1 {
		t.Fatalf("read %s: %v %v", key, ps, err)
	}
	id := ps[key].ID
	for _, details := range []bool{false, true} {
		got, err := c.MergeInfo(context.Background(), []string{id}, details)
		if err != nil {
			t.Fatal(err)
		}
		mi := got[id]
		t.Logf("details=%v: %+v", details, mi)
		if mi.Key != key || mi.HeadOID == "" || mi.MergeState == "" || len(mi.Repo.Methods) == 0 {
			t.Errorf("merge info incomplete: %+v", mi)
		}
		if details && (mi.Files == nil || mi.Checks == nil) {
			t.Errorf("details missing: %+v", mi)
		}
	}
}
