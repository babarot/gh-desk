package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/babarot/gh-triage/internal/logging"
	"github.com/babarot/gh-triage/internal/model"
)

const infoFields = `
  id number state isDraft headRefOid mergeStateStatus reviewDecision
  autoMergeRequest { enabledAt }
  repository { nameWithOwner mergeCommitAllowed squashMergeAllowed rebaseMergeAllowed autoMergeAllowed }
  latestReviews(first: 20) { nodes { author { login } state } }
  commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }`

type commitsNode struct {
	Nodes []struct {
		Commit struct {
			StatusCheckRollup *rollupNode
		}
	}
}

type rollupNode struct {
	State    string
	Contexts *struct {
		Nodes []struct {
			Typename   string `json:"__typename"`
			Name       string
			Status     string
			Conclusion string
			Context    string
			State      string
			IsRequired bool
		}
	}
}

type infoNode struct {
	ID               string
	Number           int
	State            string
	IsDraft          bool
	HeadRefOid       string
	MergeStateStatus string
	ReviewDecision   string
	AutoMergeRequest *struct{ EnabledAt time.Time }
	Repository       struct {
		NameWithOwner      string
		MergeCommitAllowed bool
		SquashMergeAllowed bool
		RebaseMergeAllowed bool
		AutoMergeAllowed   bool
	}
	LatestReviews struct {
		Nodes []struct {
			Author *struct{ Login string }
			State  string
		}
	}
	Commits commitsNode
	Files   *struct{ Nodes []struct{ Path string } }
}

func (n infoNode) toModel() model.MergeInfo {
	owner, repo, _ := strings.Cut(n.Repository.NameWithOwner, "/")
	mi := model.MergeInfo{
		Key: model.PRKey{Owner: owner, Repo: repo, Number: n.Number}, ID: n.ID, State: model.PRState(n.State),
		Draft: n.IsDraft, HeadOID: n.HeadRefOid, MergeState: model.MergeState(n.MergeStateStatus),
		Decision: model.ReviewDecision(n.ReviewDecision), AutoMerge: n.AutoMergeRequest != nil,
		Repo: model.RepoMerge{AutoMerge: n.Repository.AutoMergeAllowed},
	}
	if n.Repository.MergeCommitAllowed {
		mi.Repo.Methods = append(mi.Repo.Methods, model.MethodMerge)
	}
	if n.Repository.SquashMergeAllowed {
		mi.Repo.Methods = append(mi.Repo.Methods, model.MethodSquash)
	}
	if n.Repository.RebaseMergeAllowed {
		mi.Repo.Methods = append(mi.Repo.Methods, model.MethodRebase)
	}
	for _, r := range n.LatestReviews.Nodes {
		if r.Author != nil {
			mi.Reviews = append(mi.Reviews, model.Review{Author: r.Author.Login, State: model.ReviewState(r.State)})
		}
	}
	if len(n.Commits.Nodes) > 0 {
		if rollup := n.Commits.Nodes[0].Commit.StatusCheckRollup; rollup != nil {
			mi.Rollup = model.CheckState(rollup.State)
			if rollup.Contexts != nil {
				mi.Checks = []model.Check{}
				for _, c := range rollup.Contexts.Nodes {
					switch c.Typename {
					case "CheckRun":
						st := model.CheckState(c.Conclusion)
						if c.Status != "COMPLETED" {
							st = model.CheckPending
						}
						mi.Checks = append(mi.Checks, model.Check{Name: c.Name, State: st, Required: c.IsRequired})
					case "StatusContext":
						mi.Checks = append(mi.Checks, model.Check{Name: c.Context, State: model.CheckState(c.State), Required: c.IsRequired})
					}
				}
			}
		}
	}
	if n.Files != nil {
		mi.Files = []string{}
		for _, f := range n.Files.Nodes {
			mi.Files = append(mi.Files, f.Path)
		}
	}
	return mi
}

// MergeInfo reads what merging needs about the pull requests with the given
// node IDs, in one query. With details it also reads each one's checks
// (with whether they are required) and its changed files, which costs more;
// a poll leaves them out. A pull request that cannot be read is left out
// of the result.
func (c *Client) MergeInfo(ctx context.Context, ids []string, details bool) (map[string]model.MergeInfo, error) {
	out := map[string]model.MergeInfo{}
	if len(ids) == 0 {
		return out, nil
	}
	var b strings.Builder
	b.WriteString("query {\n  rateLimit { cost remaining }\n")
	for i, id := range ids {
		fmt.Fprintf(&b, "  n%d: node(id: %q) { ... on PullRequest {%s\n", i, id, infoFields)
		if details {
			// isRequired needs the pull request, so each one is asked for
			// with its own ID.
			fmt.Fprintf(&b, `  commits2: commits(last: 1) { nodes { commit { statusCheckRollup { state contexts(first: 100) { nodes {
    __typename
    ... on CheckRun { name status conclusion isRequired(pullRequestId: %q) }
    ... on StatusContext { context state isRequired(pullRequestId: %q) }
  } } } } } }
  files(first: 100) { nodes { path } }
`, id, id)
		}
		b.WriteString("  } }\n")
	}
	b.WriteString("}")
	var resp map[string]jsonRaw
	name := "mergeInfo"
	if details {
		name = "mergeInfoDetails"
	}
	err := c.gql.DoWithContext(ctx, b.String(), nil, &resp)
	if raw, ok := resp["rateLimit"]; ok {
		var rl rateLimit
		if decode(raw, &rl) == nil {
			c.log.Debug("graphql", "query", name, "prs", len(ids), logging.KeyGQLCost, rl.Cost, logging.KeyGQLRemaining, rl.Remaining)
		}
	}
	for i, id := range ids {
		raw, ok := resp[fmt.Sprintf("n%d", i)]
		if !ok || raw == nil {
			continue
		}
		var n struct {
			infoNode
			Commits2 *commitsNode
		}
		if decode(raw, &n) != nil || n.ID == "" {
			continue
		}
		if n.Commits2 != nil {
			n.infoNode.Commits = *n.Commits2
		}
		out[id] = n.infoNode.toModel()
	}
	var gqlErr *api.GraphQLError
	if err != nil && !errors.As(err, &gqlErr) {
		return out, err
	}
	return out, nil
}

// ErrCleanStatus is what enabling auto-merge returns when the pull request
// can already be merged: GitHub wants it merged directly then.
var ErrCleanStatus = errors.New("pull request is in clean status")

// write sends a mutation and logs it at info with its request ID.
func (c *Client) write(ctx context.Context, step string, key model.PRKey, head, mutation string, vars map[string]any) error {
	ctx, reqID := withRequestID(ctx)
	var resp struct{}
	err := c.gql.DoWithContext(ctx, mutation, vars, &resp)
	attrs := []any{logging.KeyPR, key.String(), logging.KeyStep, step, "head", head, logging.KeyGHRequestID, *reqID}
	if err != nil {
		c.log.Info(step+" failed", append(attrs, "err", err.Error())...)
		if step == "enable-auto-merge" && strings.Contains(strings.ToLower(err.Error()), "clean status") {
			return fmt.Errorf("%w: %v", ErrCleanStatus, err)
		}
		return err
	}
	c.log.Info(step+" done", attrs...)
	return nil
}

// Merge merges the pull request only if its head is still head: GitHub
// refuses the merge when a commit was pushed after it was read.
func (c *Client) Merge(ctx context.Context, key model.PRKey, id, head string, method model.MergeMethod) error {
	return c.write(ctx, "merge", key, head,
		`mutation($id: ID!, $head: GitObjectID!, $method: PullRequestMergeMethod!) {
  mergePullRequest(input: {pullRequestId: $id, expectedHeadOid: $head, mergeMethod: $method}) { clientMutationId }
}`, map[string]any{"id": id, "head": head, "method": string(method)})
}

// EnableAutoMerge turns on GitHub's auto-merge for the pull request at head.
// GitHub merges it when its rules are met; commits pushed later are merged
// too, as GitHub's auto-merge does.
func (c *Client) EnableAutoMerge(ctx context.Context, key model.PRKey, id, head string, method model.MergeMethod) error {
	return c.write(ctx, "enable-auto-merge", key, head,
		`mutation($id: ID!, $head: GitObjectID, $method: PullRequestMergeMethod!) {
  enablePullRequestAutoMerge(input: {pullRequestId: $id, expectedHeadOid: $head, mergeMethod: $method}) { clientMutationId }
}`, map[string]any{"id": id, "head": head, "method": string(method)})
}

// UpdateBranch merges the base branch into the pull request's branch, only
// if its head is still head.
func (c *Client) UpdateBranch(ctx context.Context, key model.PRKey, id, head string) error {
	return c.write(ctx, "update-branch", key, head,
		`mutation($id: ID!, $head: GitObjectID!) {
  updatePullRequestBranch(input: {pullRequestId: $id, expectedHeadOid: $head, updateMethod: MERGE}) { clientMutationId }
}`, map[string]any{"id": id, "head": head})
}

// Commit is what tells a commit gh-triage made by updating a branch from
// one someone pushed.
type Commit struct {
	Parents []string
	// ViaWeb is set on a commit GitHub made (the web, or an API such as
	// updatePullRequestBranch), not one pushed.
	ViaWeb bool
	Author string
}

// CommitInfo reads a commit's parents, whether GitHub made it, and its
// author.
func (c *Client) CommitInfo(ctx context.Context, key model.PRKey, oid string) (Commit, error) {
	var resp struct {
		Repository struct {
			Object *struct {
				Parents         struct{ Nodes []struct{ Oid string } }
				CommittedViaWeb bool
				Author          struct{ User *struct{ Login string } }
			}
		}
	}
	err := c.query(ctx, "commit", `query($owner: String!, $name: String!, $oid: GitObjectID!) {
  repository(owner: $owner, name: $name) { object(oid: $oid) { ... on Commit {
    parents(first: 5) { nodes { oid } } committedViaWeb author { user { login } }
  } } }
}`, map[string]any{"owner": key.Owner, "name": key.Repo, "oid": oid}, &resp, nil)
	if err != nil {
		return Commit{}, err
	}
	o := resp.Repository.Object
	if o == nil {
		return Commit{}, fmt.Errorf("%s: commit %s not found", key, oid)
	}
	out := Commit{ViaWeb: o.CommittedViaWeb}
	if o.Author.User != nil {
		out.Author = o.Author.User.Login
	}
	for _, p := range o.Parents.Nodes {
		out.Parents = append(out.Parents, p.Oid)
	}
	return out, nil
}
