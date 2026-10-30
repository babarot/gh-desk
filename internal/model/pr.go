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
