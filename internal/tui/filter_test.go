package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

func TestFilterWords(t *testing.T) {
	human := model.PR{Key: model.PRKey{Owner: "babarot", Repo: "gh-desk", Number: 123}, Title: "fix: keep the cursor when alice reloads", Author: "bob"}
	bot := model.PR{Key: model.PRKey{Owner: "acme", Repo: "api", Number: 7}, Title: "Bump golang.org/x/net", Author: "app/dependabot"}
	moji := model.PR{Key: model.PRKey{Owner: "acme", Repo: "api", Number: 8}, Title: ":bug: Stop the retries", Author: "carol"}
	tests := []struct {
		filter string
		pr     model.PR
		want   bool
	}{
		{"", human, true},
		{"   ", human, true},
		// A word with no qualifier looks in the key, the title and the
		// author, ignoring case.
		{"CURSOR", human, true},
		{"#12", human, true},
		{"bob", human, true},
		// Every word must hold, in any order, each in one field.
		{"cursor fix", human, true},
		{"cursor nothing", human, false},
		{"reloads bob", human, true},
		{"reloads bo", human, true},
		// author: asks the author alone, from the start of the login.
		{"author:alice", human, false},
		{"author:bob", human, true},
		{"author:BO", human, true},
		{"author:ob", human, false},
		{"author:dependabot", bot, true},
		{"author:app/dependabot", bot, true},
		{"author:bot", bot, false},
		// A qualifier still being typed asks nothing.
		{"author:", human, true},
		{"cursor author:", human, true},
		// repo: and title: ask their field alone.
		{"repo:gh-desk", human, true},
		{"repo:babarot/", human, true},
		{"repo:bob", human, false},
		{"title:alice", human, true},
		{"title:bob", human, false},
		{"author:bob title:cursor repo:desk", human, true},
		// A title is matched as written and as drawn (ADR-077).
		{":bug:", moji, true},
		{"\U0001F41B", moji, true},
		{"title:\U0001F41B retries", moji, true},
		// Only the known qualifiers are read; fix: is a word as it is.
		{"fix:", human, true},
		{"feat:", human, false},
	}
	for _, tt := range tests {
		if got := matches(tt.pr, tt.filter); got != tt.want {
			t.Errorf("matches(%s, %q) = %v, want %v", tt.pr.Key, tt.filter, got, tt.want)
		}
	}
}

func TestFilterTextColorsQualifiers(t *testing.T) {
	base, qual := lipgloss.NewStyle(), lipgloss.NewStyle().Bold(true)
	tests := []struct{ filter, want string }{
		{"repo:hoge", qual.Render("repo") + ":hoge"},
		{"  REPO:x  fix:", "  " + qual.Render("REPO") + ":x  fix:"},
		{"cursor　author:", "cursor　" + qual.Render("author") + ":"},
		{"author", "author"},
		{"xauthor:bob", "xauthor:bob"},
	}
	for _, tt := range tests {
		got := renderFilterText(tt.filter, base, qual)
		if got != tt.want {
			t.Errorf("renderFilterText(%q) = %q, want %q", tt.filter, got, tt.want)
		}
		if lipgloss.Width(got) != lipgloss.Width(tt.filter) {
			t.Errorf("renderFilterText(%q) is %d wide, want %d", tt.filter, lipgloss.Width(got), lipgloss.Width(tt.filter))
		}
	}
}

func TestFilterCompletion(t *testing.T) {
	tests := []struct{ filter, want string }{
		{"", ""},
		// One letter is not enough: many words start like a qualifier.
		{"a", ""},
		{"au", "thor:"},
		{"AU", "thor:"},
		{"author", ":"},
		{"re", "po:"},
		{"tit", "le:"},
		{"cursor au", "thor:"},
		{"cursor　au", "thor:"},
		{"au ", ""},
		{"author:", ""},
		{"author:bo", ""},
		{"ax", ""},
		{"autho r", ""},
	}
	for _, tt := range tests {
		if got := completion(tt.filter); got != tt.want {
			t.Errorf("completion(%q) = %q, want %q", tt.filter, got, tt.want)
		}
	}
}

func TestFilterOffersQualifiers(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "/")
	if s := screen(m); !strings.Contains(s, "/ author: repo: title:") {
		t.Errorf("an empty filter does not show the qualifiers:\n%s", s)
	}
	if strings.Contains(ansi.Strip(m.hints()), "complete") {
		t.Errorf("tab is told with nothing to complete: %q", ansi.Strip(m.hints()))
	}

	// The rest of a qualifier follows the word, and tab fills it in.
	press(m, "a", "u")
	if s := screen(m); !strings.Contains(s, "/ author:") {
		t.Errorf("au does not offer author:\n%s", s)
	}
	if !strings.Contains(ansi.Strip(m.hints()), "complete") {
		t.Errorf("tab is not told: %q", ansi.Strip(m.hints()))
	}
	press(m, "tab", "o", "c")
	if m.filter != "author:oc" || !m.filtering {
		t.Fatalf("filter = %q, filtering = %v; want %q and filtering", m.filter, m.filtering, "author:oc")
	}

	// An accepted filter offers nothing.
	press(m, "backspace", "backspace", "backspace", "backspace", "backspace", "enter")
	if m.filter != "auth" {
		t.Fatalf("filter = %q, want %q", m.filter, "auth")
	}
	if s := screen(m); strings.Contains(s, "author:") {
		t.Errorf("an accepted filter still offers a completion:\n%s", s)
	}
}
