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

// ReviewEvent is what a review sent from gh-triage does.
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

// PR is a pull request as the lists show it.
type PR struct {
	// ID is GitHub's node ID, for mutations.
	ID           string
	Key          PRKey
	Title        string
	URL          string
	State        PRState
	Draft        bool
	Author       string
	AuthorIsBot  bool
	Created      time.Time
	Updated      time.Time
	HeadRef      string
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
	Rollup    CheckState
	// Checks and Files are read only when asked for; nil otherwise.
	Checks []Check
	Files  []string
	Repo   RepoMerge
	// Reviews are the latest review of each reviewer.
	Reviews []Review
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

// Pending is a merge gh-triage waits for, as recorded to go on at the next
// launch. The times waited are kept, not the times started, so the time
// gh-triage was not running is not counted against the limits.
type Pending struct {
	Key         PRKey  `json:"key"`
	ID          string `json:"id"`
	Head        string `json:"head"`
	Author      string `json:"author"`
	AuthorIsBot bool   `json:"author_is_bot"`
	// AutoMerge is whether the user agreed to auto-merge for it.
	AutoMerge bool `json:"auto_merge"`
	// Watching is set once auto-merge is on and it is only followed.
	Watching bool `json:"watching"`
	// Approved is set when the user's approval was on it, sent by
	// gh-triage or there before the merge started; an approval dismissed
	// meanwhile stops it at the next launch.
	Approved bool `json:"approved"`
	// Approving is set while an approval is being sent: if gh-triage
	// stops then, whether it reached GitHub is not known.
	Approving     bool          `json:"approving"`
	ChecksWaited  time.Duration `json:"checks_waited"`
	BlockedWaited time.Duration `json:"blocked_waited"`
	Watched       time.Duration `json:"watched"`
}
