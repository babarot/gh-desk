package plan

import (
	"slices"

	"github.com/babarot/gh-desk/internal/model"
)

// Show is how a tab shows the pull requests of one of its sections.
type Show string

const (
	// ShowOpen shows the section's head and its rows.
	ShowOpen Show = "open"
	// ShowFolded shows the head alone, with the count, until it is
	// unfolded.
	ShowFolded Show = "folded"
	// ShowExcluded leaves the rows out as an exclusion rule does: counted
	// as excluded, and shown dimmed only while the tab shows the excluded.
	ShowExcluded Show = "excluded"
)

// Section is a part of a tab: the pull requests its rule says Yes of that
// no section above it took. A rule with no condition takes all that is
// left.
type Section struct {
	Name string
	Show Show
	Rule Rule
}

// Judge is what the section says of p for viewer: Yes when it takes p.
func (s Section) Judge(p model.PR, viewer string, f Facts) Verdict {
	return s.Rule.Judge(p, viewer, f)
}

// Place is the section p goes in: the first that says Yes of it, passing
// over one that cannot tell yet. pending reports whether one was passed
// over, so that the row is not left out on what is not read yet, and is
// drawn as not judged. It is -1 when no section takes p.
func Place(sections []Section, p model.PR, viewer string, f Facts) (i int, pending bool) {
	for i, s := range sections {
		switch s.Judge(p, viewer, f) {
		case Yes:
			return i, pending
		case Pending:
			pending = true
		}
	}
	return -1, pending
}

// OtherName is the section that takes what no other section of a tab
// does.
const OtherName = "other"

// Layout are the sections of a tab as its rows are laid out, and which of
// them takes what no other does: the one called other, or one made for it
// after the rest, so that no row is lost, neither one no rule takes nor
// one a rule cannot tell yet. other is -1 when there are no sections.
func Layout(ss []Section) (out []Section, other int) {
	if len(ss) == 0 {
		return nil, -1
	}
	for i, s := range ss {
		if s.Name == OtherName {
			return ss, i
		}
	}
	out = append(slices.Clone(ss), Section{Name: OtherName, Show: ShowOpen})
	return out, len(out) - 1
}

// PlaceIn is Place among sections laid out with Layout: what no section
// takes goes in other.
func PlaceIn(sections []Section, other int, p model.PR, viewer string, f Facts) (int, bool) {
	at, pending := Place(sections, p, viewer, f)
	if at < 0 {
		at = other
	}
	return at, pending
}
