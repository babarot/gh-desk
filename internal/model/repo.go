package model

import "time"

// The types below are what Claude may read of a pull request's repository
// through gh-desk's own MCP server (ADR-080): checks and their logs, and
// the repository's issues and pull requests. Each carries the repository it
// came from, so the server can leave out anything GitHub returns from
// another one.

// RepoCheck is one check of a commit: a check run, or a status when ID is
// zero.
type RepoCheck struct {
	// ID is the check run's ID, which for GitHub Actions is the job's ID.
	ID       int64
	Name     string
	Workflow string
	// App is the slug of the app that runs the check, such as
	// "github-actions"; empty for a status.
	App   string
	State CheckState
	// Required is whether the check is required to merge the pull request
	// it was read for; false when read for a commit alone.
	Required    bool
	URL         string
	Description string
}

// CommitChecks are the checks of one commit.
type CommitChecks struct {
	// Label says which commit this is: "head", "merge commit", or the ref
	// asked for.
	Label  string
	OID    string
	Checks []RepoCheck
	// More is set when there were more checks than were read.
	More bool
}

// CheckRunDetail is what a check run says of itself, and its annotations.
type CheckRunDetail struct {
	ID         int64
	Repo       string
	Name       string
	App        string
	Status     string
	Conclusion string
	URL        string
	Title      string
	Summary    string
	Text       string
	// Annotations are the first of them; AnnotationCount is how many
	// there are in all.
	Annotations     []Annotation
	AnnotationCount int
}

// Annotation is a check run's note on a line of a file.
type Annotation struct {
	Path    string
	Line    int
	Level   string
	Title   string
	Message string
}

// IssueHit is an issue or a pull request a search found.
type IssueHit struct {
	Repo    string
	Number  int
	PR      bool
	Title   string
	State   string
	Author  string
	Updated time.Time
	Labels  []string
}

// IssueDetail is one issue or pull request, as Claude reads it. The fields
// after Comments are a pull request's.
type IssueDetail struct {
	Repo   string
	Number int
	PR     bool
	Title  string
	State  string
	Author string
	Body   string
	Labels []string
	// Comments are the latest ones; CommentCount is how many there are.
	Comments     []Comment
	CommentCount int
	Reviews      []SubmittedReview
	ReviewCount  int
	Threads      []Thread
	ThreadCount  int
	Files        []ChangedFile
	FileCount    int
	// Closes are the issues the pull request closes, as their repository
	// and number only.
	Closes []IssueRef
}

// SubmittedReview is a review submitted on a pull request.
type SubmittedReview struct {
	Author string
	State  string
	Body   string
}

// Thread is a pull request's thread of comments on a line.
type Thread struct {
	Path     string
	Line     int
	Resolved bool
	Comments []Comment
	// CommentCount is how many comments the thread has, those not sent yet
	// included.
	CommentCount int
}

// ChangedFile is a file a pull request changes.
type ChangedFile struct {
	Path      string
	Additions int
	Deletions int
}

// IssueRef names an issue by its repository and number.
type IssueRef struct {
	Repo   string
	Number int
}
