package plan

import "github.com/babarot/gh-desk/internal/model"

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
	// Group, when set, has the section take the pull requests GroupOf puts
	// in it, and its Rule is not read. A tab split into groups is read as
	// sections this way, so that it keeps what the groups showed, which a
	// rule cannot say of the other group (a review asked of neither the
	// viewer nor a team).
	Group Group
}

// Judge is what the section says of p for viewer: Yes when it takes p.
func (s Section) Judge(p model.PR, viewer string, f Facts) Verdict {
	if s.Group != "" {
		if GroupOf(p, viewer) == s.Group {
			return Yes
		}
		return No
	}
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

// GroupSections are the sections of a tab split into groups, in the order
// given, each named after its group; those in folded are folded. A pull
// request of a group not given goes in none, so it is not shown, as the
// group was not.
func GroupSections(groups, folded []Group) []Section {
	out := make([]Section, 0, len(groups))
	for _, g := range groups {
		s := Section{Name: string(g), Show: ShowOpen, Group: g}
		for _, f := range folded {
			if f == g {
				s.Show = ShowFolded
			}
		}
		out = append(out, s)
	}
	return out
}
