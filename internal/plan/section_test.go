package plan

import (
	"testing"

	"github.com/babarot/gh-desk/internal/model"
)

func TestPlace(t *testing.T) {
	read := model.PR{Author: "alice", Requested: []model.Reviewer{{Login: "me"}}, Checks: model.CheckSuccess, HeadOID: "h", HeavyHead: "h"}
	unread := read
	unread.HeavyHead = ""
	bot := read
	bot.AuthorIsBot = true
	passing := Section{Name: "passing", Rule: Rule{Checks: []Checks{ChecksPassing}}}
	bots := Section{Name: "bots", Rule: Rule{Bot: new(true)}}
	rest := Section{Name: "rest"}
	tests := []struct {
		name     string
		sections []Section
		pr       model.PR
		at       int
		pending  bool
	}{
		{"the first that takes it", []Section{bots, passing, rest}, read, 1, false},
		{"above takes it first", []Section{bots, passing, rest}, bot, 0, false},
		{"no condition takes the rest", []Section{bots, rest}, read, 1, false},
		{"none takes it", []Section{bots}, read, -1, false},
		{"one that cannot tell is passed over", []Section{passing, rest}, unread, 1, true},
		{"nothing after it", []Section{passing}, unread, -1, true},
	}
	for _, tt := range tests {
		at, pending := Place(tt.sections, tt.pr, "me", Facts{ApprovalKnown: true})
		if at != tt.at || pending != tt.pending {
			t.Errorf("%s: Place = %d, %v, want %d, %v", tt.name, at, pending, tt.at, tt.pending)
		}
	}
}

// What no section takes goes in the one called other, or in one made for
// it after the rest; a tab with no sections has none.
func TestLayout(t *testing.T) {
	bots := Section{Name: "bots", Rule: Rule{Bot: new(true)}}
	got, other := Layout([]Section{bots})
	if len(got) != 2 || got[1].Name != OtherName || other != 1 {
		t.Errorf("Layout = %+v, %d; want other made after the rest", got, other)
	}
	mine := Section{Name: OtherName, Rule: Rule{Draft: new(true)}}
	if got, other := Layout([]Section{mine, bots}); len(got) != 2 || other != 0 {
		t.Errorf("Layout = %+v, %d; want the user's other", got, other)
	}
	if got, other := Layout(nil); got != nil || other != -1 {
		t.Errorf("Layout(nil) = %+v, %d", got, other)
	}
	if at, _ := PlaceIn(got, other, model.PR{}, "me", Facts{}); at != 1 {
		t.Errorf("PlaceIn = %d; want what bots does not take in other", at)
	}
}
