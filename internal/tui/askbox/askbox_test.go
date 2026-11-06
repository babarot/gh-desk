package askbox

import "testing"

func entries(qs ...string) []Entry {
	var out []Entry
	for _, q := range qs {
		out = append(out, Entry{Question: q})
	}
	return out
}

func TestRecallWalksAndComesBack(t *testing.T) {
	list := entries("c", "b", "a")
	var r Recall
	for _, want := range []string{"c", "b", "a"} {
		if got, ok := r.Older(list, "typed"); !ok || got != want {
			t.Fatalf("Older = %q, %v, want %q", got, ok, want)
		}
	}
	if _, ok := r.Older(list, "ignored"); ok {
		t.Error("went past the oldest")
	}
	if r.At() != 2 {
		t.Errorf("At = %d", r.At())
	}
	for _, want := range []string{"b", "c", "typed"} {
		if got := r.Newer(list); got != want {
			t.Fatalf("Newer = %q, want %q", got, want)
		}
	}
	if r.Active() {
		t.Error("still in the questions")
	}
}

func TestRecallOfAListGoneShorter(t *testing.T) {
	var r Recall
	list := entries("c", "b", "a")
	r.Older(list, "typed")
	r.Older(list, "")
	r.Older(list, "")
	if got := r.Newer(entries("c")); got != "c" {
		t.Errorf("Newer = %q", got)
	}
	r.Older(list, "")
	r.Older(list, "")
	if got := r.Newer(nil); got != "typed" || r.Active() {
		t.Errorf("Newer = %q, active %v", got, r.Active())
	}
}

func TestRecallOfNothing(t *testing.T) {
	var r Recall
	if _, ok := r.Older(nil, "typed"); ok || r.Active() {
		t.Error("recalled from nothing")
	}
}

func TestGhost(t *testing.T) {
	try := []string{"bots with passing checks"}
	list := entries("Bots again", "先週から動いていないものは？", "the same")
	for _, tc := range []struct{ typed, want string }{
		{"", ""},
		{"BO", "ts with passing checks"},
		{"bots a", "gain"},
		{"先週", "から動いていないものは？"},
		{"the same", ""},
		{"nothing", ""},
		{"\u212a", ""},
	} {
		if got := Ghost(tc.typed, try, list); got != tc.want {
			t.Errorf("Ghost(%q) = %q, want %q", tc.typed, got, tc.want)
		}
	}
}

func TestLastWord(t *testing.T) {
	for _, tc := range []struct{ typed, head, word string }{
		{"", "", ""},
		{"#1", "", "#1"},
		{"compare #1", "compare ", "#1"},
		{"compare ", "compare ", ""},
		{"比べて　#1", "比べて　", "#1"},
	} {
		if h, w := LastWord(tc.typed); h != tc.head || w != tc.word {
			t.Errorf("LastWord(%q) = %q, %q", tc.typed, h, w)
		}
	}
}

func TestMenu(t *testing.T) {
	var m Menu
	m.For("a").Next(3)
	m.For("a").Next(3)
	if m.For("a").At(3) != 2 {
		t.Errorf("At = %d", m.At(3))
	}
	m.Next(3)
	m.Prev(3)
	if m.At(3) != 2 {
		t.Errorf("round the end: %d", m.At(3))
	}
	m.Close()
	if !m.For("a").Closed() {
		t.Error("not closed")
	}
	if m.For("ab").Closed() || m.At(3) != 0 {
		t.Error("a text changed did not open it from the first")
	}
}

func TestHits(t *testing.T) {
	got := Hits(entries("Bots with passing checks", "先週から動いていないものは？", "bots again"), "BOTS")
	if len(got) != 2 {
		t.Errorf("Hits = %+v", got)
	}
	if got := Hits(entries("先週から動いていないものは？"), "動いて"); len(got) != 1 {
		t.Errorf("Hits in the middle of Japanese = %+v", got)
	}
	if got := Hits(entries("a"), ""); len(got) != 1 {
		t.Errorf("Hits of nothing = %+v", got)
	}
}

func TestSearchGoesRound(t *testing.T) {
	var s Search
	s.Prev(3)
	if s.At(3) != 2 {
		t.Errorf("Prev from the first = %d", s.At(3))
	}
	s.Next(3)
	if s.At(3) != 0 {
		t.Errorf("Next from the last = %d", s.At(3))
	}
	s.Next(3)
	s.Next(3)
	if s.At(1) != 0 {
		t.Errorf("At of fewer hits = %d", s.At(1))
	}
	s.Next(0)
	s.Prev(0)
	if s.At(0) != 0 {
		t.Errorf("At of no hits = %d", s.At(0))
	}
}

func TestShownFollowsTheHit(t *testing.T) {
	for _, tc := range []struct{ at, n, room, from, to int }{
		{0, 3, 5, 0, 3}, {4, 9, 5, 0, 5}, {5, 9, 5, 1, 6}, {8, 9, 5, 4, 9},
	} {
		if from, to := Shown(tc.at, tc.n, tc.room); from != tc.from || to != tc.to {
			t.Errorf("Shown(%d, %d, %d) = %d, %d", tc.at, tc.n, tc.room, from, to)
		}
	}
}

func TestPrependAndUnique(t *testing.T) {
	got := Prepend(entries("a", "b"), Entry{Question: "b"})
	if len(got) != 2 || got[0].Question != "b" || got[1].Question != "a" {
		t.Errorf("Prepend = %+v", got)
	}
	got = Unique(entries("b", "a", "b"))
	if len(got) != 2 || got[0].Question != "b" || got[1].Question != "a" {
		t.Errorf("Unique = %+v", got)
	}
}
