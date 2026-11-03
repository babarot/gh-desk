package demo

import (
	"context"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui"
)

var (
	_ tui.Source    = (*GitHub)(nil)
	_ runner.GitHub = (*GitHub)(nil)
	_ tui.Claude    = (*Claude)(nil)
)

// Each tab of the default config finds its pull requests, and a merged one
// leaves its search, as on GitHub.
func TestSearchesAndMerge(t *testing.T) {
	ctx := context.Background()
	gh := NewGitHub(time.Now())
	gh.Speed = 0
	counts := map[string]int{}
	for _, q := range []string{
		"is:open is:pr review-requested:@me archived:false",
		"is:open is:pr author:@me archived:false",
		"is:open is:pr involves:@me -author:@me -review-requested:@me archived:false",
	} {
		prs, _, _, _, err := gh.SearchPage(ctx, q, 100, "")
		if err != nil {
			t.Fatal(err)
		}
		counts[q] = len(prs)
		if len(prs) == 0 {
			t.Errorf("%q finds nothing", q)
		}
	}
	review := "is:open is:pr review-requested:@me archived:false"
	prs, _, _, _, _ := gh.SearchPage(ctx, review, 100, "")
	if err := gh.Merge(ctx, prs[0].Key, prs[0].ID, prs[0].HeadOID, model.MethodMerge); err != nil {
		t.Fatal(err)
	}
	if after, _, _, _, _ := gh.SearchPage(ctx, review, 100, ""); len(after) != counts[review]-1 {
		t.Errorf("after a merge the search finds %d, want %d", len(after), counts[review]-1)
	}
}
