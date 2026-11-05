package tui

import (
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
	w := filter
	if i := strings.LastIndexFunc(filter, unicode.IsSpace); i >= 0 {
		_, n := utf8.DecodeRuneInString(filter[i:])
		w = filter[i+n:]
	}
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
