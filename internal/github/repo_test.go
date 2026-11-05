package github

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

func TestCommitChecksReadsEveryPage(t *testing.T) {
	f := &fake{rules: []rule{
		{gqlHas(`"after":null`), `{"data": {"rateLimit": {"cost": 1, "remaining": 4999}, "repository": {"object": {"oid": "abc", "statusCheckRollup": {"contexts": {
  "pageInfo": {"hasNextPage": true, "endCursor": "c1"},
  "nodes": [{"__typename": "CheckRun", "databaseId": 126664958582, "name": "test", "status": "COMPLETED", "conclusion": "FAILURE",
    "detailsUrl": "https://github.com/babarot/gh-desk/actions/runs/1/job/126664958582", "isRequired": true,
    "checkSuite": {"app": {"slug": "github-actions"}, "workflowRun": {"workflow": {"name": "CI"}}}}]}}}}}}`},
		{gqlHas(`"after":"c1"`), `{"data": {"rateLimit": {"cost": 1, "remaining": 4998}, "repository": {"object": {"oid": "abc", "statusCheckRollup": {"contexts": {
  "pageInfo": {"hasNextPage": false, "endCursor": ""},
  "nodes": [{"__typename": "StatusContext", "context": "deploy", "state": "PENDING", "description": "waiting", "targetUrl": "https://example.com"}]}}}}}}`},
	}}
	c := newClient(t, f, logging.Discard())
	cs, err := c.CommitChecks(context.Background(), "babarot", "gh-desk", "head", "abc", 47)
	if err != nil {
		t.Fatal(err)
	}
	want := []model.RepoCheck{
		{ID: 126664958582, Name: "test", Workflow: "CI", App: "github-actions", State: model.CheckFailure, Required: true,
			URL: "https://github.com/babarot/gh-desk/actions/runs/1/job/126664958582"},
		{Name: "deploy", State: model.CheckPending, URL: "https://example.com", Description: "waiting"},
	}
	if cs.OID != "abc" || len(cs.Checks) != 2 || cs.Checks[0] != want[0] || cs.Checks[1] != want[1] {
		t.Errorf("checks = %+v", cs)
	}
	for _, s := range f.sent {
		if !strings.Contains(s.body, `"owner":"babarot"`) || !strings.Contains(s.body, `"name":"gh-desk"`) || !strings.Contains(s.body, `"withPR":true`) {
			t.Errorf("sent %s", s.body)
		}
	}
}

func TestJobLogWithoutALog(t *testing.T) {
	f := &fake{}
	c := newClient(t, f, logging.Discard())
	log, ok, err := c.JobLog(context.Background(), "babarot", "gh-desk", 9)
	if err != nil || ok || log != nil {
		t.Errorf("JobLog = %q, %v, %v", log, ok, err)
	}
	if len(f.sent) != 1 || f.sent[0].path != "/repos/babarot/gh-desk/actions/jobs/9/logs" {
		t.Errorf("sent %+v", f.sent)
	}
}

func TestIssueLeavesOutWhatOnlyTheViewerSees(t *testing.T) {
	f := &fake{rules: []rule{{gqlHas("issueOrPullRequest"), `{"data": {"rateLimit": {"cost": 1, "remaining": 4999}, "repository": {"nameWithOwner": "babarot/gh-desk", "issueOrPullRequest": {
  "__typename": "PullRequest", "number": 47, "title": "t", "state": "MERGED", "body": "b", "author": {"login": "babarot"},
  "labels": {"nodes": []}, "comments": {"totalCount": 0, "nodes": []},
  "reviews": {"nodes": [{"author": {"login": "x"}, "state": "APPROVED", "body": ""}]},
  "reviewThreads": {"nodes": [
    {"path": "a.go", "line": 3, "isResolved": false, "comments": {"nodes": [{"author": {"login": "me"}, "body": "draft", "state": "PENDING"}]}},
    {"path": "b.go", "line": 5, "isResolved": true, "comments": {"nodes": [{"author": {"login": "x"}, "body": "nit", "state": "SUBMITTED"}]}}]},
  "files": {"totalCount": 1, "nodes": [{"path": "a.go", "additions": 1, "deletions": 0}]},
  "closingIssuesReferences": {"nodes": [{"number": 12, "repository": {"nameWithOwner": "other/private"}}]}}}}}`}}}
	c := newClient(t, f, logging.Discard())
	d, err := c.Issue(context.Background(), "babarot", "gh-desk", 47)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Threads) != 1 || d.Threads[0].Path != "b.go" || d.State != "merged" || !d.PR {
		t.Errorf("issue = %+v", d)
	}
	if len(d.Closes) != 1 || d.Closes[0] != (model.IssueRef{Repo: "other/private", Number: 12}) {
		t.Errorf("closes = %+v; the server leaves out other repositories, the client reports them", d.Closes)
	}
	q := f.sent[0].body
	if strings.Contains(q, "timelineItems") || !strings.Contains(q, "states: [APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED]") ||
		strings.Contains(q, "closingIssuesReferences(first: 20) { nodes { number repository { nameWithOwner } title") {
		t.Errorf("query = %s", q)
	}
	if f.sent[0].method != http.MethodPost {
		t.Errorf("method = %s", f.sent[0].method)
	}
}
