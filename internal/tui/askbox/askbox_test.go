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
