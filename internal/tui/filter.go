package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	"github.com/babarot/gh-desk/internal/model"
)

// matches reports whether p passes the filter typed, ignoring case. The
// filter is words split by spaces, every one of which must hold, in any
// order. A word is one of:
//
//   - author:x, the author's login starts with x, without app/ for a bot
//   - repo:x, the owner/repo contains x
//   - title:x, the title contains x
//   - any other word, the key, the title or the author contains it
//
// A qualifier with nothing after it yet asks nothing, so the rows do not
// all go while it is being typed. Only these qualifiers are read: a word
// such as fix: is looked for as it is, as titles often start with one.
func matches(p model.PR, filter string) bool {
	for w := range strings.FieldsSeq(strings.ToLower(filter)) {
		if !matchesWord(p, w) {
			return false
		}
	}
	return true
}

// qualifiers are the names a word of the filter may start with, before a
// colon, to ask one field alone.
var qualifiers = []string{"author", "repo", "title"}

// qualifier splits w into a qualifier it starts with and the value after
// the colon, ignoring the case of the qualifier.
func qualifier(w string) (name, value string, ok bool) {
	for _, q := range qualifiers {
		if len(w) > len(q) && strings.EqualFold(w[:len(q)], q) && w[len(q)] == ':' {
			return w[:len(q)], w[len(q)+1:], true
		}
	}
	return "", "", false
}

// matchesWord reports whether p holds one word of the filter, already in
// lower case.
func matchesWord(p model.PR, w string) bool {
	login := func(s string) string { return strings.TrimPrefix(strings.ToLower(s), "app/") }
	if q, v, ok := qualifier(w); ok {
		switch q {
		case "author":
			return v == "" || strings.HasPrefix(login(p.Author), login(v))
		case "repo":
			return strings.Contains(strings.ToLower(p.Key.Repository()), v)
		case "title":
			return titleContains(p, v)
		}
	}
	return strings.Contains(strings.ToLower(p.Key.String()), w) ||
		titleContains(p, w) ||
		strings.Contains(strings.ToLower(p.Author), w)
}

// titleContains reports whether p's title contains w, already in lower
// case, as written or as it is drawn, so both :rocket: and the emoji find a
// title written with the shortcode (ADR-077).
func titleContains(p model.PR, w string) bool {
	return strings.Contains(strings.ToLower(p.Title), w) || strings.Contains(strings.ToLower(shownTitle(p.Title)), w)
}

// renderFilterText draws the filter typed as it is, spaces and all, in
// base, with the name of each qualifier it reads in qual, so the user sees
// which words ask one field alone.
func renderFilterText(filter string, base, qual lipgloss.Style) string {
	var b strings.Builder
	rest := filter
	for rest != "" {
		// A run of spaces, then a word up to the next space, split as
		// matches splits them.
		i := strings.IndexFunc(rest, func(r rune) bool { return !unicode.IsSpace(r) })
		if i < 0 {
			b.WriteString(base.Render(rest))
			break
		}
		if i > 0 {
			b.WriteString(base.Render(rest[:i]))
		}
		rest = rest[i:]
		j := strings.IndexFunc(rest, unicode.IsSpace)
		if j < 0 {
			j = len(rest)
		}
		w := rest[:j]
		if q, _, ok := qualifier(w); ok {
			b.WriteString(qual.Render(q) + base.Render(w[len(q):]))
		} else {
			b.WriteString(base.Render(w))
		}
		rest = rest[j:]
	}
	return b.String()
}

// filterPlaceholder is what an empty filter shows while it is typed: the
// qualifiers, so the user learns they are there.
func filterPlaceholder() string {
	qs := make([]string, len(qualifiers))
	for i, q := range qualifiers {
		qs[i] = q + ":"
	}
	return strings.Join(qs, " ")
}

// completion is what the complete key adds to filter: the rest of the one
// qualifier its last word starts, with the colon, once that word has two
// letters; "" when there is none. The words of a title often start like a
// qualifier, so it is only offered, never filled in by itself.
func completion(filter string) string {
	if filter == "" {
		return ""
	}
	if r, _ := utf8.DecodeLastRuneInString(filter); unicode.IsSpace(r) {
		return ""
	}
	w := lastWord(filter)
	if utf8.RuneCountInString(w) < 2 || strings.Contains(w, ":") {
		return ""
	}
	w = strings.ToLower(w)
	rest := ""
	for _, q := range qualifiers {
		if strings.HasPrefix(q, w) {
			if rest != "" {
				return ""
			}
			rest = q[len(w):] + ":"
		}
	}
	return rest
}

// filterCycle is the value completion going round: the filter as it was
// typed before complete filled in its first candidate, and the candidate
// filled in now. It ends at any other key.
type filterCycle struct {
	on   bool
	base string
	i    int
}

// lastWord is the word filter ends in, "" when it ends in a space.
func lastWord(filter string) string {
	if i := strings.LastIndexFunc(filter, unicode.IsSpace); i >= 0 {
		_, n := utf8.DecodeRuneInString(filter[i:])
		return filter[i+n:]
	}
	return filter
}

// valueCandidates are the values the last word of filter may be completed
// to, when it is author: or repo: with the start of a value: the logins
// (without app/) or repositories of the pull requests the tab shows under
// the rest of the filter, so a completion never leaves no rows and never
// names one hidden. head is the filter up to the value; the most common
// value in the tab comes first, then by name. author: is matched from the
// start of the login, as the filter does; repo: from the start of
// owner/repo or of the repository's name, as only what follows the cursor
// can be offered.
func (m *Model) valueCandidates(filter string) (head string, cands []string) {
	w := lastWord(filter)
	q, v, ok := qualifier(w)
	if !ok || !strings.EqualFold(q, "author") && !strings.EqualFold(q, "repo") {
		return "", nil
	}
	before := filter[:len(filter)-len(w)]
	lv := strings.ToLower(v)
	n := map[string]int{}
	t := m.tab()
	for _, r := range m.rowsOf(t, before, t.ruleFilter, nil) {
		if r.kind != rowPR {
			continue
		}
		var c string
		switch {
		case strings.EqualFold(q, "author"):
			c = strings.TrimPrefix(r.pr.Author, "app/")
		case strings.HasPrefix(strings.ToLower(r.pr.Key.Repository()), lv):
			c = r.pr.Key.Repository()
		default:
			c = r.pr.Key.Repo
		}
		if lc := strings.ToLower(c); c != "" && lc != lv && strings.HasPrefix(lc, lv) {
			n[c]++
		}
	}
	for c := range n {
		cands = append(cands, c)
	}
	slices.SortFunc(cands, func(a, b string) int {
		return cmp.Or(n[b]-n[a], cmp.Compare(strings.ToLower(a), strings.ToLower(b)))
	})
	return before + w[:len(q)+1], cands
}

// complete fills in what the filter's ghost offers: the rest of a
// qualifier, or a value, and again the next value while the values go
// round.
func (m *Model) complete() {
	if c := completion(m.filter); c != "" {
		m.filter += c
		return
	}
	if head, cands, ok := m.cycling(); ok {
		m.cycle.i = (m.cycle.i + 1) % len(cands)
		m.filter = head + cands[m.cycle.i]
		return
	}
	m.cycle = filterCycle{}
	if head, cands := m.valueCandidates(m.filter); len(cands) > 0 {
		m.cycle = filterCycle{on: true, base: m.filter}
		m.filter = head + cands[0]
	}
}

// cycling are the values going round, when they still are: the filter is
// the one complete filled in last, and the rows still give that value. A
// filter changed by other means, or rows read again, end the round.
func (m *Model) cycling() (head string, cands []string, ok bool) {
	if !m.cycle.on {
		return "", nil, false
	}
	head, cands = m.valueCandidates(m.cycle.base)
	if m.cycle.i >= len(cands) || m.filter != head+cands[m.cycle.i] {
		return "", nil, false
	}
	return head, cands, true
}

// canComplete reports whether complete would change the filter.
func (m *Model) canComplete() bool {
	if completion(m.filter) != "" {
		return true
	}
	if _, cands, ok := m.cycling(); ok {
		return len(cands) > 1
	}
	_, cands := m.valueCandidates(m.filter)
	return len(cands) > 0
}

// filterGhost is what follows the cursor, faint, while the filter is
// typed: the qualifiers while it is empty, the rest of a qualifier or of
// the first value it may be completed to, and while there are several
// values which one of them it is.
func (m *Model) filterGhost() string {
	if m.filter == "" {
		return filterPlaceholder()
	}
	if c := completion(m.filter); c != "" {
		return c
	}
	count := func(i, n int) string {
		if n < 2 {
			return ""
		}
		return fmt.Sprintf(" (%d/%d)", i+1, n)
	}
	if _, cands, ok := m.cycling(); ok {
		return count(m.cycle.i, len(cands))
	}
	_, v, _ := qualifier(lastWord(m.filter))
	if _, cands := m.valueCandidates(m.filter); len(cands) > 0 && len(v) <= len(cands[0]) {
		return cands[0][len(v):] + count(0, len(cands))
	}
	return ""
}
