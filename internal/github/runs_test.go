package github

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/model"
)

const detailsBody = `{"data": {
  "rateLimit": {"cost": 1, "remaining": 4999},
  "n0": {"id": "PR_1", "number": 7, "state": "OPEN", "isDraft": false, "headRefOid": "abc123", "baseRefOid": "def456",
    "mergeStateStatus": "UNSTABLE", "reviewDecision": "", "viewerCanMergeAsAdmin": false, "autoMergeRequest": null,
    "repository": {"nameWithOwner": "acme/tool", "mergeCommitAllowed": true, "squashMergeAllowed": false, "rebaseMergeAllowed": false, "autoMergeAllowed": false},
    "latestReviews": {"nodes": []},
    "commits": {"nodes": [{"commit": {"statusCheckRollup": {"state": "PENDING"}}}]},
    "commits2": {"nodes": [{"commit": {"statusCheckRollup": {"state": "PENDING", "contexts": {"pageInfo": {"hasNextPage": false}, "nodes": [
      {"__typename": "CheckRun", "name": "lint", "status": "COMPLETED", "conclusion": "SUCCESS", "isRequired": false},
      {"__typename": "CheckRun", "name": "test (ubuntu-latest)", "status": "IN_PROGRESS", "conclusion": null, "isRequired": false},
      {"__typename": "StatusContext", "context": "policy-bot: main", "state": "PENDING", "isRequired": true}
    ]}}}}]},
    "files": {"nodes": [{"path": "main.go"}]}}
}}`

const runsBody = `{"total_count": 3, "workflow_runs": [
  {"name": "Test", "status": "in_progress", "conclusion": null},
  {"name": "Lint", "status": "completed", "conclusion": "success"}
]}`

func runsPath(r *http.Request, _ string) bool {
	return r.URL.Path == "/repos/acme/tool/actions/runs" && r.URL.Query().Get("head_sha") == "abc123"
}

// Reading the details keeps what kind each check is and how far a check
// run is, and reads the workflow runs of the head.
func TestMergeInfoDetailsReadsJobs(t *testing.T) {
	f := &fake{rules: []rule{{match: gqlHas("commits2"), body: detailsBody}, {match: runsPath, body: runsBody}}}
	c := newClient(t, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
	got, err := c.MergeInfo(context.Background(), []string{"PR_1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	mi := got["PR_1"]
	want := []model.Check{
		{Name: "lint", State: model.CheckSuccess, Kind: model.KindCheckRun, Run: model.RunCompleted},
		{Name: "test (ubuntu-latest)", State: model.CheckPending, Kind: model.KindCheckRun, Run: "IN_PROGRESS"},
		{Name: "policy-bot: main", State: model.CheckPending, Required: true, Kind: model.KindStatus},
	}
	if !slices.Equal(mi.Checks, want) {
		t.Errorf("checks = %+v; want %+v", mi.Checks, want)
	}
	runs := []model.WorkflowRun{{Name: "Test", Status: "IN_PROGRESS"}, {Name: "Lint", Status: model.RunCompleted, Conclusion: model.CheckSuccess}}
	if !mi.RunsRead || !slices.Equal(mi.Runs, runs) {
		t.Errorf("runs = %+v (read %v); want %+v", mi.Runs, mi.RunsRead, runs)
	}
	// total_count is more than the runs given.
	if !mi.RunsPartial {
		t.Error("runs not marked partial")
	}
}

// Runs that cannot be read leave the rest of the pull request read.
func TestMergeInfoDetailsWithoutRuns(t *testing.T) {
	f := &fake{rules: []rule{{match: gqlHas("commits2"), body: detailsBody}}}
	c := newClient(t, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
	got, err := c.MergeInfo(context.Background(), []string{"PR_1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	mi, ok := got["PR_1"]
	if !ok || len(mi.Checks) != 3 {
		t.Fatalf("pull request not read: %+v", got)
	}
	if mi.RunsRead || mi.Runs != nil {
		t.Errorf("runs = %+v (read %v); want none read", mi.Runs, mi.RunsRead)
	}
}

// Without details, no workflow run is asked for.
func TestMergeInfoWithoutDetailsReadsNoRuns(t *testing.T) {
	light := strings.Replace(detailsBody, `"commits2"`, `"unused"`, 1)
	f := &fake{rules: []rule{{match: gqlHas("mergeStateStatus"), body: light}, {match: runsPath, body: runsBody}}}
	c := newClient(t, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := c.MergeInfo(context.Background(), []string{"PR_1"}, false); err != nil {
		t.Fatal(err)
	}
	for _, s := range f.sent {
		if strings.Contains(s.path, "/actions/runs") {
			t.Errorf("asked for runs: %s", s.path)
		}
	}
}
