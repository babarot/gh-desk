package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/babarot/gh-desk/internal/model"
)

// The reads below are what gh-desk's MCP server gives Claude (ADR-078).
// Every one is asked of one repository, by its owner and name, which the
// caller fixes; none takes another repository from its arguments. Each
// result carries the repository GitHub returned it from, so the caller can
// leave out what came from another one.

const prCommitsQuery = `query($owner: String!, $name: String!, $number: Int!) {
  rateLimit { cost remaining resetAt }
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) { headRefOid mergeCommit { oid } }
  }
}`

// PRCommits are the commits of a pull request whose checks Claude reads:
// its head, and its merge commit once merged (empty before).
func (c *Client) PRCommits(ctx context.Context, key model.PRKey) (head, merge string, err error) {
	var resp struct {
		RateLimit  rateLimit
		Repository struct {
			PullRequest *struct {
				HeadRefOid  string
				MergeCommit *struct{ Oid string }
			}
		}
	}
	vars := map[string]any{"owner": key.Owner, "name": key.Repo, "number": key.Number}
	if err := c.query(ctx, "pr_commits", prCommitsQuery, vars, &resp, &resp.RateLimit); err != nil {
		return "", "", err
	}
	pr := resp.Repository.PullRequest
	if pr == nil {
		return "", "", fmt.Errorf("%s not found", key)
	}
	if pr.MergeCommit != nil {
		merge = pr.MergeCommit.Oid
	}
	return pr.HeadRefOid, merge, nil
}

const commitChecksQuery = `query($owner: String!, $name: String!, $expr: String!, $after: String, $number: Int!, $withPR: Boolean!) {
  rateLimit { cost remaining resetAt }
  repository(owner: $owner, name: $name) {
    object(expression: $expr) {
      ... on Commit {
        oid
        statusCheckRollup {
          contexts(first: 100, after: $after) {
            pageInfo { hasNextPage endCursor }
            nodes {
              __typename
              ... on CheckRun {
                databaseId name status conclusion detailsUrl
                isRequired(pullRequestNumber: $number) @include(if: $withPR)
                checkSuite { app { slug } workflowRun { workflow { name } } }
              }
              ... on StatusContext {
                context state description targetUrl
                isRequired(pullRequestNumber: $number) @include(if: $withPR)
              }
            }
          }
        }
      }
    }
  }
}`

// maxCheckPages bounds how many pages of a commit's checks are read, so a
// commit with an endless list cannot keep the reads going: a constant, far
// above any commit's checks.
const maxCheckPages = 10

// CommitChecks reads the checks of the commit expr names in owner/repo (an
// OID, or a branch), every page of them, labeled as label. With pr above
// zero, a check says whether it is required for that pull request.
func (c *Client) CommitChecks(ctx context.Context, owner, repo, label, expr string, pr int) (model.CommitChecks, error) {
	out := model.CommitChecks{Label: label}
	var after *string
	for range maxCheckPages {
		var resp struct {
			RateLimit  rateLimit
			Repository struct {
				Object *struct {
					Oid               string
					StatusCheckRollup *struct {
						Contexts struct {
							PageInfo struct {
								HasNextPage bool
								EndCursor   string
							}
							Nodes []struct {
								Typename    string `json:"__typename"`
								DatabaseID  int64
								Name        string
								Status      string
								Conclusion  string
								DetailsURL  string `json:"detailsUrl"`
								IsRequired  bool
								Context     string
								State       string
								Description string
								TargetURL   string `json:"targetUrl"`
								CheckSuite  *struct {
									App         *struct{ Slug string }
									WorkflowRun *struct{ Workflow struct{ Name string } }
								}
							}
						}
					}
				}
			}
		}
		vars := map[string]any{"owner": owner, "name": repo, "expr": expr, "after": after, "number": pr, "withPR": pr > 0}
		if err := c.query(ctx, "commit_checks", commitChecksQuery, vars, &resp, &resp.RateLimit); err != nil {
			return out, err
		}
		obj := resp.Repository.Object
		if obj == nil || obj.Oid == "" {
			return out, fmt.Errorf("no commit %q in %s/%s", expr, owner, repo)
		}
		out.OID = obj.Oid
		if obj.StatusCheckRollup == nil {
			return out, nil
		}
		ctxs := obj.StatusCheckRollup.Contexts
		for _, n := range ctxs.Nodes {
			switch n.Typename {
			case "CheckRun":
				st := model.CheckState(n.Conclusion)
				if n.Status != string(model.RunCompleted) {
					st = model.CheckPending
				}
				ch := model.RepoCheck{ID: n.DatabaseID, Name: n.Name, State: st, Required: n.IsRequired, URL: n.DetailsURL}
				if s := n.CheckSuite; s != nil {
					if s.App != nil {
						ch.App = s.App.Slug
					}
					if s.WorkflowRun != nil {
						ch.Workflow = s.WorkflowRun.Workflow.Name
					}
				}
				out.Checks = append(out.Checks, ch)
			case "StatusContext":
				out.Checks = append(out.Checks, model.RepoCheck{Name: n.Context, State: model.CheckState(n.State), Required: n.IsRequired,
					URL: n.TargetURL, Description: n.Description})
			}
		}
		if !ctxs.PageInfo.HasNextPage {
			return out, nil
		}
		cur := ctxs.PageInfo.EndCursor
		after = &cur
	}
	return out, nil
}

// maxAnnotations is how many of a check run's annotations are read: a
// constant, as more than this many say the same thing over.
const maxAnnotations = 50

// CheckRun reads a check run of owner/repo and its first annotations.
// GitHub answers 404 for the ID of another repository's check run.
func (c *Client) CheckRun(ctx context.Context, owner, repo string, id int64) (model.CheckRunDetail, error) {
	var run struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		HTMLURL    string `json:"html_url"`
		App        *struct {
			Slug string `json:"slug"`
		} `json:"app"`
		Output struct {
			Title            string `json:"title"`
			Summary          string `json:"summary"`
			Text             string `json:"text"`
			AnnotationsCount int    `json:"annotations_count"`
		} `json:"output"`
	}
	base := fmt.Sprintf("repos/%s/%s/check-runs/%d", url.PathEscape(owner), url.PathEscape(repo), id)
	if err := c.rest.DoWithContext(ctx, http.MethodGet, base, nil, &run); err != nil {
		return model.CheckRunDetail{}, err
	}
	d := model.CheckRunDetail{ID: run.ID, Repo: owner + "/" + repo, Name: run.Name, Status: run.Status, Conclusion: run.Conclusion, URL: run.HTMLURL,
		Title: run.Output.Title, Summary: run.Output.Summary, Text: run.Output.Text, AnnotationCount: run.Output.AnnotationsCount}
	if run.App != nil {
		d.App = run.App.Slug
	}
	if d.AnnotationCount == 0 {
		return d, nil
	}
	var anns []struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		Level     string `json:"annotation_level"`
		Title     string `json:"title"`
		Message   string `json:"message"`
	}
	if err := c.rest.DoWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/annotations?per_page=%d", base, maxAnnotations), nil, &anns); err != nil {
		return d, err
	}
	for _, a := range anns {
		d.Annotations = append(d.Annotations, model.Annotation{Path: a.Path, Line: a.StartLine, Level: a.Level, Title: a.Title, Message: a.Message})
	}
	return d, nil
}

// maxJobLog is how much of a job's log is read: a constant, as Claude is
// given only its errors and its end.
const maxJobLog = 8 << 20

// JobLog reads the log of a GitHub Actions job of owner/repo, cut at
// maxJobLog from its end. ok is false when GitHub has no log for it (a
// canceled job, an expired log): its storage answers 404, in a shape of
// its own.
func (c *Client) JobLog(ctx context.Context, owner, repo string, id int64) (log []byte, ok bool, err error) {
	path := fmt.Sprintf("repos/%s/%s/actions/jobs/%d/logs", url.PathEscape(owner), url.PathEscape(repo), id)
	resp, err := c.rest.RequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		var he *api.HTTPError
		if errors.As(err, &he) && (he.StatusCode == http.StatusNotFound || he.StatusCode == http.StatusGone) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxJobLog*4))
	if err != nil {
		return nil, false, err
	}
	if len(b) > maxJobLog {
		b = b[len(b)-maxJobLog:]
	}
	return b, true, nil
}

const searchIssuesQuery = `query($q: String!, $first: Int!) {
  rateLimit { cost remaining resetAt }
  search(query: $q, type: ISSUE, first: $first) {
    nodes {
      __typename
      ... on Issue { number title state updatedAt author { login } repository { nameWithOwner } labels(first: 10) { nodes { name } } }
      ... on PullRequest { number title state updatedAt author { login } repository { nameWithOwner } labels(first: 10) { nodes { name } } }
    }
  }
}`

// SearchIssues runs a search of issues and pull requests the caller made,
// and returns at most first of them, each with its repository.
func (c *Client) SearchIssues(ctx context.Context, q string, first int) ([]model.IssueHit, error) {
	var resp struct {
		RateLimit rateLimit
		Search    struct {
			Nodes []struct {
				Typename   string `json:"__typename"`
				Number     int
				Title      string
				State      string
				UpdatedAt  time.Time
				Author     *struct{ Login string }
				Repository struct{ NameWithOwner string }
				Labels     struct{ Nodes []struct{ Name string } }
			}
		}
	}
	if err := c.query(ctx, "search_issues", searchIssuesQuery, map[string]any{"q": q, "first": first}, &resp, &resp.RateLimit); err != nil {
		return nil, err
	}
	var out []model.IssueHit
	for _, n := range resp.Search.Nodes {
		if n.Typename != "Issue" && n.Typename != "PullRequest" {
			continue
		}
		h := model.IssueHit{Repo: n.Repository.NameWithOwner, Number: n.Number, PR: n.Typename == "PullRequest", Title: n.Title,
			State: strings.ToLower(n.State), Author: login(n.Author), Updated: n.UpdatedAt}
		for _, l := range n.Labels.Nodes {
			h.Labels = append(h.Labels, l.Name)
		}
		out = append(out, h)
	}
	return out, nil
}

// The most of each part of an issue or pull request that is read: constants,
// as Claude is told when there are more.
const (
	maxIssueComments = 50
	maxReviews       = 30
	maxThreads       = 50
	maxThreadReplies = 10
	maxFiles         = 100
	maxCloses        = 20
)

// issueQuery reads an issue or a pull request. It asks for nothing
// GitHub would fill from another repository but the repository and
// number of the issues a pull request closes; the timeline, which carries
// references from other repositories, is not read. Pending reviews and
// review comments, which only the viewer sees, are left out.
var issueQuery = fmt.Sprintf(`query($owner: String!, $name: String!, $number: Int!) {
  rateLimit { cost remaining resetAt }
  repository(owner: $owner, name: $name) {
    nameWithOwner
    issueOrPullRequest(number: $number) {
      __typename
      ... on Issue {
        number title state body author { login } labels(first: 20) { nodes { name } }
        comments(last: %[1]d) { totalCount nodes { author { login } body createdAt } }
      }
      ... on PullRequest {
        number title state body author { login } labels(first: 20) { nodes { name } }
        comments(last: %[1]d) { totalCount nodes { author { login } body createdAt } }
        reviews(last: %[2]d, states: [APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED]) { nodes { author { login } state body } }
        reviewThreads(first: %[3]d) { nodes { path line isResolved comments(first: %[4]d) { nodes { author { login } body createdAt state } } } }
        files(first: %[5]d) { totalCount nodes { path additions deletions } }
        closingIssuesReferences(first: %[6]d) { nodes { number repository { nameWithOwner } } }
      }
    }
  }
}`, maxIssueComments, maxReviews, maxThreads, maxThreadReplies, maxFiles, maxCloses)

type authored struct {
	Author    *struct{ Login string }
	Body      string
	CreatedAt time.Time
}

func login(a *struct{ Login string }) string {
	if a == nil {
		return "ghost"
	}
	return a.Login
}

func comments(ns []authored) []model.Comment {
	out := make([]model.Comment, 0, len(ns))
	for _, n := range ns {
		out = append(out, model.Comment{Author: login(n.Author), Body: n.Body, Created: n.CreatedAt})
	}
	return out
}

// Issue reads the issue or pull request number of owner/repo.
func (c *Client) Issue(ctx context.Context, owner, repo string, number int) (model.IssueDetail, error) {
	var resp struct {
		RateLimit  rateLimit
		Repository *struct {
			NameWithOwner      string
			IssueOrPullRequest *struct {
				Typename string `json:"__typename"`
				Number   int
				Title    string
				State    string
				Body     string
				Author   *struct{ Login string }
				Labels   struct{ Nodes []struct{ Name string } }
				Comments struct {
					TotalCount int
					Nodes      []authored
				}
				Reviews struct {
					Nodes []struct {
						Author *struct{ Login string }
						State  string
						Body   string
					}
				}
				ReviewThreads struct {
					Nodes []struct {
						Path       string
						Line       int
						IsResolved bool
						Comments   struct {
							Nodes []struct {
								authored
								State string
							}
						}
					}
				}
				Files struct {
					TotalCount int
					Nodes      []struct {
						Path      string
						Additions int
						Deletions int
					}
				}
				ClosingIssuesReferences struct {
					Nodes []struct {
						Number     int
						Repository struct{ NameWithOwner string }
					}
				}
			}
		}
	}
	vars := map[string]any{"owner": owner, "name": repo, "number": number}
	if err := c.query(ctx, "issue", issueQuery, vars, &resp, &resp.RateLimit); err != nil {
		return model.IssueDetail{}, err
	}
	if resp.Repository == nil || resp.Repository.IssueOrPullRequest == nil {
		return model.IssueDetail{}, fmt.Errorf("no issue or pull request #%d in %s/%s", number, owner, repo)
	}
	n := resp.Repository.IssueOrPullRequest
	d := model.IssueDetail{Repo: resp.Repository.NameWithOwner, Number: n.Number, PR: n.Typename == "PullRequest", Title: n.Title,
		State: strings.ToLower(n.State), Author: login(n.Author), Body: n.Body,
		Comments: comments(n.Comments.Nodes), CommentCount: n.Comments.TotalCount, FileCount: n.Files.TotalCount}
	for _, l := range n.Labels.Nodes {
		d.Labels = append(d.Labels, l.Name)
	}
	for _, r := range n.Reviews.Nodes {
		d.Reviews = append(d.Reviews, model.SubmittedReview{Author: login(r.Author), State: r.State, Body: r.Body})
	}
	for _, t := range n.ReviewThreads.Nodes {
		th := model.Thread{Path: t.Path, Line: t.Line, Resolved: t.IsResolved}
		for _, cm := range t.Comments.Nodes {
			if cm.State == "PENDING" {
				continue
			}
			th.Comments = append(th.Comments, model.Comment{Author: login(cm.Author), Body: cm.Body, Created: cm.CreatedAt})
		}
		if len(th.Comments) > 0 {
			d.Threads = append(d.Threads, th)
		}
	}
	for _, f := range n.Files.Nodes {
		d.Files = append(d.Files, model.ChangedFile{Path: f.Path, Additions: f.Additions, Deletions: f.Deletions})
	}
	for _, r := range n.ClosingIssuesReferences.Nodes {
		d.Closes = append(d.Closes, model.IssueRef{Repo: r.Repository.NameWithOwner, Number: r.Number})
	}
	return d, nil
}
