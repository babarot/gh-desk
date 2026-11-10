package plan

import (
	"reflect"
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
		{"a group takes what GroupOf puts in it", GroupSections([]Group{GroupTeam, GroupYou}, nil), read, 1, false},
		{"a group not given takes none", GroupSections([]Group{GroupBots}, nil), read, -1, false},
	}
	for _, tt := range tests {
		at, pending := Place(tt.sections, tt.pr, "me", Facts{ApprovalKnown: true})
		if at != tt.at || pending != tt.pending {
			t.Errorf("%s: Place = %d, %v, want %d, %v", tt.name, at, pending, tt.at, tt.pending)
		}
	}
}

func TestGroupSections(t *testing.T) {
	got := GroupSections([]Group{GroupYou, GroupBots}, []Group{GroupBots})
	want := []Section{{Name: "you", Show: ShowOpen, Group: GroupYou}, {Name: "bots", Show: ShowFolded, Group: GroupBots}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GroupSections = %+v, want %+v", got, want)
	}
}
