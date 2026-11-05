package mcpserver

import (
	"fmt"
	"regexp"
	"strings"
)

// GitHub's search ORs two repo: qualifiers, so a search that took Claude's
// text as written could reach another repository. searchQuery takes no
// syntax from Claude: each argument is checked against the values it may
// have, or quoted as a phrase, so nothing in it reads as a qualifier.

var (
	kinds  = map[string]string{"issue": "is:issue", "pr": "is:pr"}
	states = map[string]string{"open": "is:open", "closed": "is:closed", "merged": "is:merged"}
	// login is a GitHub login, a bot's with [bot], or app/<name>.
	login = regexp.MustCompile(`^(app/)?[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})(\[bot\])?$`)
)

// phrase is s as a quoted phrase: quotes, backslashes and line breaks,
// which could end the phrase early, are dropped.
func phrase(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case '"', '\\', '\n', '\r':
			return ' '
		}
		return r
	}, s)
	return `"` + strings.Join(strings.Fields(s), " ") + `"`
}

// searchQuery is the search of in, in owner/repo.
func searchQuery(owner, repo string, in searchIn) (string, error) {
	q := []string{"repo:" + owner + "/" + repo}
	if in.Kind != "" {
		k, ok := kinds[in.Kind]
		if !ok {
			return "", fmt.Errorf(`kind must be "issue" or "pr", got %q`, in.Kind)
		}
		q = append(q, k)
	}
	if in.State != "" {
		st, ok := states[in.State]
		if !ok {
			return "", fmt.Errorf(`state must be "open", "closed" or "merged", got %q`, in.State)
		}
		q = append(q, st)
	}
	if in.Author != "" {
		if !login.MatchString(in.Author) {
			return "", fmt.Errorf("author must be a login, or app/<name> for a bot, got %q", in.Author)
		}
		q = append(q, "author:"+in.Author)
	}
	for _, l := range in.Labels {
		if p := phrase(l); p != `""` {
			q = append(q, "label:"+p)
		}
	}
	// The phrase goes last, so that nothing after it could be taken into it.
	if p := phrase(in.Text); p != `""` {
		q = append(q, p)
	}
	return strings.Join(q, " "), nil
}
