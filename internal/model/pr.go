package model

import "time"

// ReviewDecision is GitHub's summary of a pull request's reviews.
type ReviewDecision string

const (
	DecisionApproved         ReviewDecision = "APPROVED"
	DecisionChangesRequested ReviewDecision = "CHANGES_REQUESTED"
	DecisionReviewRequired   ReviewDecision = "REVIEW_REQUIRED"
)

// ReviewState is the state of one review.
type ReviewState string

const (
	ReviewApproved         ReviewState = "APPROVED"
	ReviewChangesRequested ReviewState = "CHANGES_REQUESTED"
	ReviewCommented        ReviewState = "COMMENTED"
	ReviewDismissed        ReviewState = "DISMISSED"
	ReviewPending          ReviewState = "PENDING"
)

// ReviewEvent is what a review sent from gh-desk does.
type ReviewEvent string

const (
	EventApprove        ReviewEvent = "APPROVE"
	EventComment        ReviewEvent = "COMMENT"
	EventRequestChanges ReviewEvent = "REQUEST_CHANGES"
)

// CheckState is the combined state of a pull request's checks, or the state
// of one check, as GitHub's statusCheckRollup and check runs give it.
type CheckState string

const (
	CheckSuccess  CheckState = "SUCCESS"
	CheckFailure  CheckState = "FAILURE"
	CheckError    CheckState = "ERROR"
	CheckPending  CheckState = "PENDING"
	CheckExpected CheckState = "EXPECTED"
	// CheckSkipped and CheckNeutral are conclusions of single check runs.
	CheckSkipped CheckState = "SKIPPED"
	CheckNeutral CheckState = "NEUTRAL"
)

// PRState is whether a pull request is open.
type PRState string

const (
	StateOpen   PRState = "OPEN"
	StateClosed PRState = "CLOSED"
	StateMerged PRState = "MERGED"
)

// Reviewer is someone a review was requested from: a user, or a team by its
// slug.
type Reviewer struct {
	Login string
	Team  bool
}

// Review is a reviewer's latest review.
type Review struct {
	Author string
	State  ReviewState
}

// ViewerReview is the viewer's latest review and the commit it was given
// on.
type ViewerReview struct {
	State  ReviewState
	Commit string
}

// PR is a pull request as the lists show it.
type PR struct {
	// ID is GitHub's node ID, for mutations.
	ID          string
	Key         PRKey
	Title       string
	URL         string
	State       PRState
	Draft       bool
	Author      string
	AuthorIsBot bool
	Created     time.Time
	Updated     time.Time
	HeadRef     string
	// HeadRepo is the repository of the head branch, "owner/repo", which
	// is another than Key's for a fork; empty when it is gone.
	HeadRepo     string
	HeadOID      string
	BaseRef      string
	Additions    int
	Deletions    int
	ChangedFiles int
	Comments     int
	Decision     ReviewDecision
	Checks       CheckState
	Requested    []Reviewer
	Reviews      []Review
	Labels       []string
	// HeavyHead is the head the heavy fields (Additions, Deletions,
	// ChangedFiles, Comments, Decision, Checks, Labels) were read for;
	// empty until they are. A list's search reads only the light fields.
	HeavyHead string
}

// HeavyRead reports whether the heavy fields are of the head shown.
func (p PR) HeavyRead() bool { return p.HeavyHead != "" && p.HeavyHead == p.HeadOID }

// Heavy are the fields of a pull request GitHub takes longest to work out,
// read apart from the lists, for the head HeadOID.
type Heavy struct {
	HeadOID      string
	Additions    int
	Deletions    int
	ChangedFiles int
	Comments     int
	Decision     ReviewDecision
	Checks       CheckState
	Labels       []string
}

// SetHeavy puts h in p.
func (p *PR) SetHeavy(h Heavy) {
	p.Additions, p.Deletions, p.ChangedFiles, p.Comments = h.Additions, h.Deletions, h.ChangedFiles, h.Comments
	p.Decision, p.Checks, p.Labels, p.HeavyHead = h.Decision, h.Checks, h.Labels, h.HeadOID
}

// Heavy is p's heavy fields.
func (p PR) Heavy() Heavy {
	return Heavy{HeadOID: p.HeavyHead, Additions: p.Additions, Deletions: p.Deletions, ChangedFiles: p.ChangedFiles,
		Comments: p.Comments, Decision: p.Decision, Checks: p.Checks, Labels: p.Labels}
}

// ReviewBy is login's latest review, if there is one.
func (p PR) ReviewBy(login string) (Review, bool) {
	for _, r := range p.Reviews {
		if r.Author == login {
			return r, true
		}
	}
	return Review{}, false
}

// RequestedFrom reports whether a review is requested from login in person.
func (p PR) RequestedFrom(login string) bool {
	for _, r := range p.Requested {
		if !r.Team && r.Login == login {
			return true
		}
	}
	return false
}

// RequestedFromTeam reports whether a review is requested from any team.
func (p PR) RequestedFromTeam() bool {
	for _, r := range p.Requested {
		if r.Team {
			return true
		}
	}
	return false
}

// Notification is a GitHub notification about a pull request.
type Notification struct {
	ThreadID string
	Key      PRKey
	Title    string
	Reason   string
	Unread   bool
	Updated  time.Time
	// LatestCommentURL is the API URL of the comment that caused the
	// notification, or empty when it was not a comment.
	LatestCommentURL string
}

// Check is one check run or status of a pull request's head.
type Check struct {
	Name  string
	State CheckState
	// Required is whether the base branch's rules require it to pass.
	Required bool
	// Kind is whether it is a check run or a commit status.
	Kind CheckKind
	// Run is a check run's status (QUEUED, IN_PROGRESS, WAITING, COMPLETED,
	// ...); empty for a commit status. State says PENDING for every run
	// not completed; Run tells a run waiting for an approval of its
	// environment from one at work.
	Run RunStatus
}

// CheckKind is what reported a check: GitHub Actions and other apps report
// check runs, a service outside reports a commit status.
type CheckKind string

const (
	KindCheckRun CheckKind = "CheckRun"
	KindStatus   CheckKind = "StatusContext"
)

// RunStatus is the status of a check run or of a workflow run, upper case
// as GraphQL gives it.
type RunStatus string

const (
	RunCompleted RunStatus = "COMPLETED"
	// RunWaiting is a run waiting for someone to approve its environment.
	RunWaiting RunStatus = "WAITING"
)

// WorkflowRun is one run of a GitHub Actions workflow on a commit. A run is
// not completed until every one of its jobs is; a job that needs another is
// made only when that one ends, so before then it has no check run, and
// only its workflow run says it is still to come.
type WorkflowRun struct {
	Name   string
	Status RunStatus
	// Conclusion is set once it is completed (SUCCESS, FAILURE, ...).
	Conclusion CheckState
}

// MergeState is GitHub's mergeStateStatus: whether a pull request can be
// merged now, and if not, why.
type MergeState string

const (
	MergeClean    MergeState = "CLEAN"
	MergeUnstable MergeState = "UNSTABLE"
	MergeHasHooks MergeState = "HAS_HOOKS"
	MergeBehind   MergeState = "BEHIND"
	MergeBlocked  MergeState = "BLOCKED"
	MergeDirty    MergeState = "DIRTY"
	MergeDraft    MergeState = "DRAFT"
	MergeUnknown  MergeState = "UNKNOWN"
)

// MergeMethod is how a pull request is merged.
type MergeMethod string

const (
	MethodMerge  MergeMethod = "MERGE"
	MethodSquash MergeMethod = "SQUASH"
	MethodRebase MergeMethod = "REBASE"
)

// RepoMerge is what a repository allows when merging.
type RepoMerge struct {
	Methods   []MergeMethod
	AutoMerge bool
}

// MergeInfo is what deciding and following a merge needs about one pull
// request, read fresh before each step.
type MergeInfo struct {
	Key     PRKey
	ID      string
	State   PRState
	Draft   bool
	HeadOID string
	// BaseOID is the head of the base branch.
	BaseOID    string
	MergeState MergeState
	Decision   ReviewDecision
	// AutoMerge is whether auto-merge is on.
	AutoMerge bool
	// Queue is whether its base branch merges through a merge queue:
	// GitHub refuses to merge it any other way.
	Queue bool
	// InQueue is whether it is in the merge queue.
	InQueue bool
	// AdminMerge is whether the viewer can merge it as an admin, past the
	// rules of its base branch.
	AdminMerge bool
	Rollup     CheckState
	// Checks and Files are read only when asked for; nil otherwise.
	Checks []Check
	// ChecksPartial is whether more checks were reported than were read.
	ChecksPartial bool
	// Runs are the workflow runs of the head commit, read with the checks.
	// RunsRead is false when they were not read, or reading them failed
	// (Actions turned off, or the token may not read them); RunsPartial is
	// whether more runs were there than were read.
	Runs        []WorkflowRun
	RunsRead    bool
	RunsPartial bool
	Files       []string
	Repo        RepoMerge
	// Reviews are the latest review of each reviewer.
	Reviews []Review
	// ViewerReview is the viewer's latest review, of any state, a
	// dismissed one too; nil for none.
	ViewerReview *ViewerReview
	// CanClose is whether the viewer may close it.
	CanClose bool
	// Locked is whether its conversation is locked.
	Locked bool
	// Head is its head branch; its ID is empty when the branch is gone.
	Head Branch
	// BaseRef is the name of its base branch.
	BaseRef string
	// HeadRefID is the node ID of the head branch, empty when it is gone.
	HeadRefID string
	// HeadInFork is whether the head branch is in another repository.
	HeadInFork bool
	// CanWrite is whether the viewer can push to the repository, which
	// deleting a branch of it takes.
	CanWrite bool
	// CanDeleteHead is whether GitHub offers the viewer to delete the head
	// branch. It does only once the pull request is closed or merged, so
	// it says nothing of an open one.
	CanDeleteHead bool
}

// Branch is a branch of a repository.
type Branch struct {
	Owner, Repo, Name string
}

// Comment is a comment on a pull request.
type Comment struct {
	Author  string
	Body    string
	Created time.Time
}

// Detail is what is read only when one pull request is opened or previewed.
type Detail struct {
	Body     string
	Checks   []Check
	Comments []Comment
}

// PendingKind is what a recorded job waits to do.
type PendingKind string

// PendingApprove is an approval that waits for the jobs at work.
const PendingApprove PendingKind = "approve"

// Pending is a merge gh-desk waits for, as recorded to go on at the next
// launch. The times waited are kept, not the times started, so the time
// gh-desk was not running is not counted against the limits.
type Pending struct {
	// Kind is what is waited for: a merge when empty, or an approval
	// asked to wait for the jobs.
	Kind PendingKind `json:"kind,omitempty"`
	// IncludeFailing is set on an approval the user let in although a
	// check failed: a job that fails while it waits does not stop it.
	IncludeFailing bool `json:"include_failing,omitempty"`
	// Auto is set on an approval a rule asked for, judged again as one
	// when it goes on (ADR-093).
	Auto bool `json:"auto,omitempty"`
	// Body is the text of an approval that waits, kept only until it is
	// sent; it is never logged.
	Body        string `json:"body,omitempty"`
	Key         PRKey  `json:"key"`
	ID          string `json:"id"`
	Head        string `json:"head"`
	Author      string `json:"author"`
	AuthorIsBot bool   `json:"author_is_bot"`
	// AutoMerge is whether the user agreed to auto-merge for it.
	AutoMerge bool `json:"auto_merge"`
	// Watching is set once auto-merge is on or it is in the merge queue,
	// and it is only followed.
	Watching bool `json:"watching"`
	// Queued is set once it is seen in the merge queue, so leaving the
	// queue unmerged tells from auto-merge being turned off.
	Queued bool `json:"queued,omitempty"`
	// Approved is set when the user's approval was on it, sent by
	// gh-desk or there before the merge started; an approval dismissed
	// meanwhile stops it at the next launch.
	Approved bool `json:"approved"`
	// Approving is set while an approval is being sent: if gh-desk
	// stops then, whether it reached GitHub is not known.
	Approving bool `json:"approving"`
	// Readied is set once marking the draft ready is sent or on its way:
	// a draft found again after it was turned back by someone, and stops
	// the merge instead of being marked ready again.
	Readied       bool          `json:"readied"`
	ChecksWaited  time.Duration `json:"checks_waited"`
	BlockedWaited time.Duration `json:"blocked_waited"`
	Watched       time.Duration `json:"watched"`
}
