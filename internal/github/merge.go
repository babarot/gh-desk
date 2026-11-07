package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

const infoFields = `
  id number state isDraft headRefOid baseRefOid mergeStateStatus reviewDecision
  viewerCanMergeAsAdmin autoMergeRequest { enabledAt }
  viewerCanClose locked headRefName baseRefName headRef { id } isCrossRepository viewerCanDeleteHeadRef
  repository { nameWithOwner mergeCommitAllowed squashMergeAllowed rebaseMergeAllowed autoMergeAllowed viewerPermission }
  latestReviews(first: 20) { nodes { author { login } state } }
  viewerLatestReview { state commit { oid } }
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
		PageInfo struct{ HasNextPage bool }
		Nodes    []struct {
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
	BaseRefOid       string
	MergeStateStatus string
	ReviewDecision   string
	// ViewerCanMergeAsAdmin is whether the viewer can merge past the base
	// branch's rules.
	ViewerCanMergeAsAdmin bool
	AutoMergeRequest      *struct{ EnabledAt time.Time }
	ViewerCanClose        bool
	Locked                bool
	HeadRefName           string
	BaseRefName           string
	HeadRef               *struct{ ID string }
	IsCrossRepository     bool
	// ViewerCanDeleteHeadRef is whether the viewer can delete the head
	// branch.
	ViewerCanDeleteHeadRef bool
	Repository             struct {
		NameWithOwner      string
		MergeCommitAllowed bool
		SquashMergeAllowed bool
		RebaseMergeAllowed bool
		AutoMergeAllowed   bool
		ViewerPermission   string
	}
	LatestReviews struct {
		Nodes []struct {
			Author *struct{ Login string }
			State  string
		}
	}
	ViewerLatestReview *struct {
		State  string
		Commit *struct{ Oid string }
	}
	Commits commitsNode
	Files   *struct{ Nodes []struct{ Path string } }
}

func (n infoNode) toModel() model.MergeInfo {
	owner, repo, _ := strings.Cut(n.Repository.NameWithOwner, "/")
	mi := model.MergeInfo{
		Key: model.PRKey{Owner: owner, Repo: repo, Number: n.Number}, ID: n.ID, State: model.PRState(n.State),
		Draft: n.IsDraft, HeadOID: n.HeadRefOid, BaseOID: n.BaseRefOid, MergeState: model.MergeState(n.MergeStateStatus),
		Decision: model.ReviewDecision(n.ReviewDecision), AutoMerge: n.AutoMergeRequest != nil,
		AdminMerge: n.ViewerCanMergeAsAdmin,
		Repo:       model.RepoMerge{AutoMerge: n.Repository.AutoMergeAllowed},
		CanClose:   n.ViewerCanClose, Locked: n.Locked, HeadInFork: n.IsCrossRepository, CanDeleteHead: n.ViewerCanDeleteHeadRef,
		Head: model.Branch{Owner: owner, Repo: repo, Name: n.HeadRefName}, BaseRef: n.BaseRefName,
		CanWrite: slices.Contains([]string{"WRITE", "MAINTAIN", "ADMIN"}, n.Repository.ViewerPermission),
	}
	if n.HeadRef != nil {
		mi.HeadRefID = n.HeadRef.ID
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
	if v := n.ViewerLatestReview; v != nil {
		mi.ViewerReview = &model.ViewerReview{State: model.ReviewState(v.State)}
		if v.Commit != nil {
			mi.ViewerReview.Commit = v.Commit.Oid
		}
	}
	if len(n.Commits.Nodes) > 0 {
		if rollup := n.Commits.Nodes[0].Commit.StatusCheckRollup; rollup != nil {
			mi.Rollup = model.CheckState(rollup.State)
			if rollup.Contexts != nil {
				mi.Checks = []model.Check{}
				mi.ChecksPartial = rollup.Contexts.PageInfo.HasNextPage
				for _, c := range rollup.Contexts.Nodes {
					switch c.Typename {
					case "CheckRun":
						st := model.CheckState(c.Conclusion)
						if c.Status != string(model.RunCompleted) {
							st = model.CheckPending
						}
						mi.Checks = append(mi.Checks, model.Check{Name: c.Name, State: st, Required: c.IsRequired,
							Kind: model.KindCheckRun, Run: model.RunStatus(c.Status)})
					case "StatusContext":
						mi.Checks = append(mi.Checks, model.Check{Name: c.Context, State: model.CheckState(c.State), Required: c.IsRequired,
							Kind: model.KindStatus})
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
// (with whether they are required), its changed files and the workflow
// runs of its head, which makes a larger query and a request for each
// pull request. Deciding a merge needs them; watching auto-merge, reading a
// pull request again to tell what happened to it, and telling whom to wake
// among the waits of a repository (ADR-103) leave them out. A pull request
// that cannot be read is left out of the result.
func (c *Client) MergeInfo(ctx context.Context, ids []string, details bool) (map[string]model.MergeInfo, error) {
	out := map[string]model.MergeInfo{}
	if len(ids) == 0 {
		return out, nil
	}
	var b strings.Builder
	b.WriteString("query {\n  rateLimit { cost remaining resetAt }\n")
	for i, id := range ids {
		fmt.Fprintf(&b, "  n%d: node(id: %q) { ... on PullRequest {%s\n", i, id, infoFields)
		if details {
			// isRequired needs the pull request, so each one is asked for
			// with its own ID.
			fmt.Fprintf(&b, `  commits2: commits(last: 1) { nodes { commit { statusCheckRollup { state contexts(first: 100) { pageInfo { hasNextPage } nodes {
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
			c.noteRateLimit(name, rl, "prs", len(ids))
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
			n.Commits = *n.Commits2
		}
		out[id] = n.toModel()
	}
	var gqlErr *api.GraphQLError
	if err != nil && !errors.As(err, &gqlErr) {
		return out, err
	}
	if details {
		c.readRuns(ctx, out)
	}
	return out, nil
}

// runsAtOnce is how many pull requests' workflow runs are read at once.
const runsAtOnce = 8

// readRuns reads the workflow runs of each pull request's head into it.
// One that cannot be read is left with RunsRead false: a repository with
// Actions turned off, or a token that may not read them, still merges, and
// the caller decides what not knowing them means.
func (c *Client) readRuns(ctx context.Context, infos map[string]model.MergeInfo) {
	type read struct {
		id      string
		runs    []model.WorkflowRun
		partial bool
		ok      bool
	}
	var reads []*read
	for id, mi := range infos {
		if mi.State == model.StateOpen {
			reads = append(reads, &read{id: id})
		}
	}
	var wg sync.WaitGroup
	slots := make(chan struct{}, runsAtOnce)
	for _, r := range reads {
		mi := infos[r.id]
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			runs, partial, err := c.WorkflowRuns(ctx, mi.Key, mi.HeadOID)
			if err != nil {
				c.log.Debug("could not read the workflow runs", logging.KeyPR, mi.Key.String(), "err", err.Error())
				return
			}
			r.runs, r.partial, r.ok = runs, partial, true
		})
	}
	wg.Wait()
	for _, r := range reads {
		if r.ok {
			mi := infos[r.id]
			mi.Runs, mi.RunsRead, mi.RunsPartial = r.runs, true, r.partial
			infos[r.id] = mi
		}
	}
}

// runsPerPage is the most workflow runs GitHub gives in one page.
const runsPerPage = 100

// WorkflowRuns reads the workflow runs of a commit of the pull request's
// repository: the first page, and whether there were more.
func (c *Client) WorkflowRuns(ctx context.Context, key model.PRKey, sha string) ([]model.WorkflowRun, bool, error) {
	var resp struct {
		TotalCount   int `json:"total_count"`
		WorkflowRuns []struct {
			Name       string
			Status     string
			Conclusion *string
		} `json:"workflow_runs"`
	}
	path := fmt.Sprintf("repos/%s/%s/actions/runs?head_sha=%s&per_page=%d",
		url.PathEscape(key.Owner), url.PathEscape(key.Repo), url.QueryEscape(sha), runsPerPage)
	if err := c.rest.DoWithContext(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, false, err
	}
	runs := make([]model.WorkflowRun, 0, len(resp.WorkflowRuns))
	for _, r := range resp.WorkflowRuns {
		// REST spells them in lower case, GraphQL in upper case.
		run := model.WorkflowRun{Name: r.Name, Status: model.RunStatus(strings.ToUpper(r.Status))}
		if r.Conclusion != nil {
			run.Conclusion = model.CheckState(strings.ToUpper(*r.Conclusion))
		}
		runs = append(runs, run)
	}
	c.log.Debug("workflow runs", logging.KeyPR, key.String(), "runs", len(runs), "total", resp.TotalCount)
	return runs, resp.TotalCount > len(runs), nil
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
			return fmt.Errorf("%w: %w", ErrCleanStatus, err)
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

// MarkReady marks a draft pull request ready for review. GitHub takes no
// expected head for it; head is logged only, the runner having checked it
// just before.
func (c *Client) MarkReady(ctx context.Context, key model.PRKey, id, head string) error {
	return c.write(ctx, "ready-for-review", key, head,
		`mutation($id: ID!) {
  markPullRequestReadyForReview(input: {pullRequestId: $id}) { clientMutationId }
}`, map[string]any{"id": id})
}

// Close closes the pull request. GitHub takes no expected head for it;
// head is logged only, the runner having checked it just before.
func (c *Client) Close(ctx context.Context, key model.PRKey, id, head string) error {
	return c.write(ctx, "close", key, head,
		`mutation($id: ID!) {
  closePullRequest(input: {pullRequestId: $id}) { clientMutationId }
}`, map[string]any{"id": id})
}

// DeleteBranch deletes the pull request's head branch by its ref's node
// ID. GitHub takes no expected head for it either; the runner checked it
// just before.
func (c *Client) DeleteBranch(ctx context.Context, key model.PRKey, refID, head string) error {
	return c.write(ctx, "delete-branch", key, head,
		`mutation($id: ID!) {
  deleteRef(input: {refId: $id}) { clientMutationId }
}`, map[string]any{"id": refID})
}

// BasedOn reads, for each branch, the numbers of the open pull requests
// whose base it is, in one query. A branch none is based on is left out.
func (c *Client) BasedOn(ctx context.Context, branches []model.Branch) (map[model.Branch][]int, error) {
	out := map[model.Branch][]int{}
	if len(branches) == 0 {
		return out, nil
	}
	var q strings.Builder
	var decl []string
	vars := map[string]any{}
	q.WriteString("{\n")
	for i, b := range branches {
		decl = append(decl, fmt.Sprintf("$o%d: String!, $r%d: String!, $b%d: String!", i, i, i))
		vars[fmt.Sprintf("o%d", i)], vars[fmt.Sprintf("r%d", i)], vars[fmt.Sprintf("b%d", i)] = b.Owner, b.Repo, b.Name
		fmt.Fprintf(&q, "  b%d: repository(owner: $o%d, name: $r%d) { pullRequests(baseRefName: $b%d, states: OPEN, first: 5) { nodes { number } } }\n", i, i, i, i)
	}
	q.WriteString("}")
	query := "query(" + strings.Join(decl, ", ") + ") " + q.String()
	var resp map[string]*struct {
		PullRequests struct {
			Nodes []struct{ Number int }
		}
	}
	if err := c.gql.DoWithContext(ctx, query, vars, &resp); err != nil {
		return nil, err
	}
	for i, b := range branches {
		r := resp[fmt.Sprintf("b%d", i)]
		if r == nil {
			continue
		}
		for _, n := range r.PullRequests.Nodes {
			out[b] = append(out[b], n.Number)
		}
	}
	c.log.Debug("based on read", "branches", len(branches), "stacked", len(out))
	return out, nil
}

// UpdateBranch merges the base branch into the pull request's branch, only
// if its head is still head.
func (c *Client) UpdateBranch(ctx context.Context, key model.PRKey, id, head string) error {
	return c.write(ctx, "update-branch", key, head,
		`mutation($id: ID!, $head: GitObjectID!) {
  updatePullRequestBranch(input: {pullRequestId: $id, expectedHeadOid: $head, updateMethod: MERGE}) { clientMutationId }
}`, map[string]any{"id": id, "head": head})
}

// Commit is what tells a commit gh-desk made by updating a branch from
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

// Bodies reads the bodies of the pull requests with the given node IDs, in
// one query.
func (c *Client) Bodies(ctx context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	var resp struct {
		RateLimit rateLimit
		Nodes     []*struct {
			ID   string
			Body string
		}
	}
	err := c.query(ctx, "bodies", `query($ids: [ID!]!) {
  rateLimit { cost remaining resetAt }
  nodes(ids: $ids) { ... on PullRequest { id body } }
}`, map[string]any{"ids": ids}, &resp, &resp.RateLimit)
	for _, n := range resp.Nodes {
		if n != nil && n.ID != "" {
			out[n.ID] = n.Body
		}
	}
	var gqlErr *api.GraphQLError
	if err != nil && !errors.As(err, &gqlErr) {
		return out, err
	}
	return out, nil
}
