package statusline

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 2, 15, 4, 0, 0, time.UTC)

func shown(l *Line, at time.Duration, acts []string, row string) string {
	_, text := l.Shown(t0.Add(at), acts, row)
	return text
}

// Each kind of event lasts its own time.
func TestEventsLast(t *testing.T) {
	for _, c := range []struct {
		say  func(*Line)
		life time.Duration
	}{
		{func(l *Line) { l.Say(t0, "x") }, InfoFor},
		{func(l *Line) { l.Refuse(t0, "x") }, RefusalFor},
		{func(l *Line) { l.Fail(t0, "x") }, FailureFor},
	} {
		var l Line
		c.say(&l)
		if got := shown(&l, c.life-time.Millisecond, nil, ""); got != "x" {
			t.Errorf("gone before its time: %q", got)
		}
		if kind, _ := l.Shown(t0.Add(c.life), nil, ""); kind != None {
			t.Errorf("kept after its time: %v", kind)
		}
	}
}

// A refusal goes at the next key; other events do not.
func TestRefusalGoesAtNextKey(t *testing.T) {
	var l Line
	l.Refuse(t0, "no")
	l.KeyPressed()
	if got := shown(&l, 0, nil, ""); got != "" {
		t.Errorf("refusal kept after a key: %q", got)
	}
	l.Say(t0, "ok")
	l.KeyPressed()
	if got := shown(&l, 0, nil, ""); got != "ok" {
		t.Errorf("information went at a key: %q", got)
	}
}

// Information does not hide a failure still in its time; a refusal does
// until the next key.
func TestFailureFirst(t *testing.T) {
	var l Line
	l.Fail(t0, "broke")
	l.Say(t0.Add(time.Second), "sent")
	if got := shown(&l, time.Second, nil, ""); got != "broke" {
		t.Fatalf("information hid the failure: %q", got)
	}
	l.Refuse(t0.Add(2*time.Second), "no")
	if kind, got := l.Shown(t0.Add(2*time.Second), nil, ""); kind != Refusal || got != "no" {
		t.Fatalf("the failure hid a refusal: %v %q", kind, got)
	}
	l.KeyPressed()
	if got := shown(&l, 2*time.Second, nil, ""); got != "broke" {
		t.Errorf("the failure did not come back: %q", got)
	}
	// Past the failure's time, the information is past its own as well.
	if kind, _ := l.Shown(t0.Add(FailureFor), nil, ""); kind != None {
		t.Errorf("kind = %v", kind)
	}
}

// Work in progress shows all of it, under events, and comes back when they
// end.
func TestActivities(t *testing.T) {
	var l Line
	acts := []string{"sending the comment… 1/2", "refreshing…"}
	if kind, got := l.Shown(t0, acts, ""); kind != Activity || got != "sending the comment… 1/2 · refreshing…" {
		t.Fatalf("%v %q", kind, got)
	}
	l.Say(t0, "canceled")
	if got := shown(&l, 0, acts, ""); got != "canceled" {
		t.Errorf("an event did not go first: %q", got)
	}
	if kind, _ := l.Shown(t0.Add(InfoFor), acts, ""); kind != Activity {
		t.Errorf("the work in progress did not come back: %v", kind)
	}
}

// The row's result goes before the work in progress for a moment after
// the cursor moved onto it, and under it after.
func TestRowResult(t *testing.T) {
	var l Line
	acts := []string{"refreshing…"}
	l.MovedOnto(t0, true)
	if kind, _ := l.Shown(t0, acts, "acme/api#12: – conflicts"); kind != Row {
		t.Fatalf("kind = %v", kind)
	}
	if kind, _ := l.Shown(t0.Add(RowFor), acts, "acme/api#12: – conflicts"); kind != Activity {
		t.Errorf("the row hid the work in progress after its moment: %v", kind)
	}
	if kind, _ := l.Shown(t0.Add(RowFor), nil, "acme/api#12: – conflicts"); kind != Row {
		t.Errorf("the row's result is not under the work in progress: %v", kind)
	}
	l.MovedOnto(t0, false)
	if kind, _ := l.Shown(t0, acts, ""); kind != Activity {
		t.Errorf("a row with no result took the moment: %v", kind)
	}
}

// Due is the next end of what is shown, and zero when nothing ends.
func TestDue(t *testing.T) {
	var l Line
	if d := l.Due(t0); !d.IsZero() {
		t.Errorf("due on an empty line: %v", d)
	}
	l.Fail(t0, "broke")
	l.Say(t0, "sent")
	if d := l.Due(t0); !d.Equal(t0.Add(InfoFor)) {
		t.Errorf("due = %v", d)
	}
	if d := l.Due(t0.Add(InfoFor)); !d.Equal(t0.Add(FailureFor)) {
		t.Errorf("due after the information = %v", d)
	}
	if d := l.Due(t0.Add(FailureFor)); !d.IsZero() {
		t.Errorf("due after all = %v", d)
	}
}
