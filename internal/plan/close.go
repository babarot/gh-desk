package plan

import (
	"fmt"
	"strings"

	"github.com/babarot/gh-desk/internal/model"
)

// Close is what closing one pull request does, decided from fresh
// information: whether it is closed, and what becomes of its branch.
type Close struct {
	// Skip is set when it is not closed, and Reason says why.
	Skip   bool
	Reason string
	// Delete is whether its head branch is deleted after it is closed.
	Delete bool
	// Keep says why its head branch is kept, when deleting was asked.
	Keep string
	// Notes are what the user should know before saying yes.
	Notes []string
}

// ForClose decides closing a pull request, with a comment when comment is
// set, deleting its head branch when deleteBranch is. basedOn are the open
// pull requests whose base its head branch is. A branch that cannot be
// deleted is kept, and the pull request is still closed (ADR-066).
func ForClose(mi model.MergeInfo, basedOn []int, comment, deleteBranch bool) Close {
	switch {
	case mi.State != model.StateOpen:
		return Close{Skip: true, Reason: stateWord(mi.State)}
	case !mi.CanClose:
		return Close{Skip: true, Reason: "you cannot close it"}
	}
	c := Close{Reason: "close"}
	if comment && mi.Locked {
		c.Notes = append(c.Notes, "its conversation is locked; GitHub may refuse the comment")
	}
	if deleteBranch {
		c.Keep = KeepBranch(mi, basedOn)
		c.Delete = c.Keep == ""
	}
	return c
}

// ForComment decides commenting on a pull request. A comment is taken
// whatever its state, so it is never skipped: the popup tells what the
// user may not know since the list was read.
func ForComment(mi model.MergeInfo) Decision {
	d := Decision{Reason: "comment"}
	if mi.State != model.StateOpen {
		d.Notes = append(d.Notes, "it is "+stateWord(mi.State))
	}
	if mi.Locked {
		d.Notes = append(d.Notes, "its conversation is locked; GitHub may refuse the comment")
	}
	return d
}

// KeepBranch says why the head branch of a pull request must be kept, or
// "" when it can be deleted: it is in a fork, gone, the viewer cannot push
// to the repository, or an open pull request is based on it, which
// deleting it would close. Whether GitHub offers to delete it is known
// only once the pull request is closed; the runner asks that after the
// close.
func KeepBranch(mi model.MergeInfo, basedOn []int) string {
	switch {
	case mi.HeadInFork:
		return "in a fork"
	case mi.HeadRefID == "":
		return "already deleted"
	case !mi.CanWrite:
		return "you cannot delete it"
	case len(basedOn) > 0:
		ns := make([]string, len(basedOn))
		for i, n := range basedOn {
			ns[i] = fmt.Sprintf("#%d", n)
		}
		return "base of " + strings.Join(ns, ", ")
	}
	return ""
}
