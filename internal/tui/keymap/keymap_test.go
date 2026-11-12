package keymap

import (
	"slices"
	"strings"
	"testing"
)

func TestDefaultsHaveNoConflicts(t *testing.T) {
	m := Default()
	if cs := m.conflicts(); len(cs) > 0 {
		t.Fatalf("default keys conflict: %+v", cs)
	}
	seen := map[Action]bool{}
	for _, d := range Defaults {
		if seen[d.Action] {
			t.Errorf("action %q defined twice", d.Action)
		}
		seen[d.Action] = true
		for _, k := range d.Keys {
			if err := CheckKey(k); err != nil {
				t.Errorf("default key of %s: %v", d.Action, err)
			}
		}
	}
}

func TestApply(t *testing.T) {
	tests := []struct {
		name    string
		set     map[string][]string
		check   map[Action][]string
		problem string
	}{
		{
			name:  "replace keys",
			set:   map[string][]string{"approve": {"a"}},
			check: map[Action][]string{"approve": {"a"}, "show_excluded": nil},
		},
		{
			name:  "turn off",
			set:   map[string][]string{"triage": {}},
			check: map[Action][]string{"triage": {}},
		},
		{
			name:  "set key takes it from a default nav key",
			set:   map[string][]string{"approve": {"j"}},
			check: map[Action][]string{"approve": {"j"}, "down": {"down", "ctrl+n"}},
		},
		{
			name:  "scopes that do not meet keep their keys",
			set:   map[string][]string{"approve": {"y"}},
			check: map[Action][]string{"approve": {"y"}, "confirm": {"y", "enter"}},
		},
		{
			// No popup reads both, so enter answers a confirm popup and
			// picks in a menu.
			name:  "confirm and choose share enter",
			set:   map[string][]string{"choose": {"enter"}},
			check: map[Action][]string{"choose": {"enter"}, "confirm": {"y", "enter"}},
		},
		{
			name:    "two set actions on one key",
			set:     map[string][]string{"approve": {"x"}, "merge": {"x"}},
			problem: `keys.merge: "x" is also approve's key`,
		},
		{
			name:    "unknown action",
			set:     map[string][]string{"aprove": {"x"}},
			problem: `"aprove" is not an action`,
		},
		{
			name:    "shifted letter",
			set:     map[string][]string{"approve": {"shift+a"}},
			problem: `write "A"`,
		},
		{
			name:    "fixed key",
			set:     map[string][]string{"quit": {"ctrl+c"}},
			problem: "fixed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, ps := Apply(Default(), tt.set)
			if tt.problem != "" {
				if len(ps) == 0 || !strings.Contains(ps[0].Message, tt.problem) {
					t.Fatalf("problems = %+v; want one containing %q", ps, tt.problem)
				}
				return
			}
			if len(ps) > 0 {
				t.Fatalf("problems = %+v", ps)
			}
			for a, want := range tt.check {
				if got := m.Keys(a); !slices.Equal(got, want) && (len(got) != 0 || len(want) != 0) {
					t.Errorf("Keys(%s) = %q; want %q", a, got, want)
				}
			}
		})
	}
}

func TestMatches(t *testing.T) {
	m := Default()
	if !m.Matches("approve", "A") || m.Matches("approve", "a") {
		t.Error("approve should match A only")
	}
	if got := m.Actions(ScopeConfirm); !slices.Equal(got, []Action{"confirm", "include_failing", "wait_for_checks", "delete_branch"}) {
		t.Errorf("Actions(confirm) = %v", got)
	}
}

func TestCheckKey(t *testing.T) {
	tests := []struct {
		key  string
		want string // empty when the key is fine
	}{
		{"a", ""},
		{"A", ""},
		{"ctrl+s", ""},
		{"shift+tab", ""},
		{"f5", ""},
		{"Enter", `write "enter"`},
		{"ctrl-s", `write "ctrl+s"`},
		{"shift+y", `write "Y"`},
		{"ctrl+Y", `write "ctrl+shift+y"`},
		{"shift+ctrl+a", `write "ctrl+shift+a"`},
		{"hyper+a", "not one of"},
		{" ", "empty"},
		{"ab", "not a key"},
	}
	for _, tt := range tests {
		err := CheckKey(tt.key)
		switch {
		case tt.want == "" && err != nil:
			t.Errorf("CheckKey(%q) = %v; want nil", tt.key, err)
		case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
			t.Errorf("CheckKey(%q) = %v; want one containing %q", tt.key, err, tt.want)
		}
	}
}

// An old name is told with the new one, and every new one is an action.
func TestRenamed(t *testing.T) {
	_, ps := Apply(Default(), map[string][]string{"open": {"x"}})
	if len(ps) != 1 || !ps[0].AtKey || ps[0].Message != `keys.open: "open" was renamed to "menu"` {
		t.Errorf("problems = %+v", ps)
	}
	m := Default()
	for old, to := range renamed {
		if _, ok := m.defs[Action(old)]; ok {
			t.Errorf("%q is both an action and an old name", old)
		}
		if _, ok := m.defs[to]; !ok {
			t.Errorf("%q was renamed to %q, which is not an action", old, to)
		}
	}
}

// An action that is gone is told with why, and is neither an action nor an
// old name.
func TestRemoved(t *testing.T) {
	_, ps := Apply(Default(), map[string][]string{"fold_bots": {"b"}})
	if len(ps) != 1 || !ps[0].AtKey || !strings.HasPrefix(ps[0].Message, `keys.fold_bots: "fold_bots" is no longer an action: `) {
		t.Errorf("problems = %+v", ps)
	}
	m := Default()
	for name := range removed {
		if _, ok := m.defs[Action(name)]; ok {
			t.Errorf("%q is both an action and removed", name)
		}
		if _, ok := renamed[name]; ok {
			t.Errorf("%q is both renamed and removed", name)
		}
	}
}

// The ? list also reads the list's fold, so a key both are set to is a
// mistake, and fold set to the key of the release notes takes it from
// them.
func TestHelpBorrowsFold(t *testing.T) {
	if _, ps := Apply(Default(), map[string][]string{"fold": {"w"}, "release_notes": {"w"}}); len(ps) != 1 {
		t.Errorf("problems %+v", ps)
	}
	m, ps := Apply(Default(), map[string][]string{"fold": {"w"}})
	if len(ps) != 0 || len(m.Keys("release_notes")) != 0 || !m.Matches("fold", "w") {
		t.Errorf("problems %+v, release_notes %v", ps, m.Keys("release_notes"))
	}
}

// An action shares the keys of the nav action it claims, and with no other:
// a nav key set for any other action is still a mistake (ADR-125).
func TestClaims(t *testing.T) {
	m := Default()
	if !m.Matches("down", "ctrl+n") || !m.Matches("history_next", "ctrl+n") {
		t.Errorf("ctrl+n is not both down and history_next: %v, %v", m.Keys("down"), m.Keys("history_next"))
	}
	if !m.Matches("up", "ctrl+p") || !m.Matches("history_prev", "ctrl+p") {
		t.Errorf("ctrl+p is not both up and history_prev: %v, %v", m.Keys("up"), m.Keys("history_prev"))
	}
	// A key set for the claiming action, or the claimed one, stays on both.
	m, ps := Apply(Default(), map[string][]string{"history_next": {"ctrl+j"}, "down": {"down", "ctrl+j"}})
	if len(ps) > 0 || !m.Matches("history_next", "ctrl+j") || !m.Matches("down", "ctrl+j") {
		t.Errorf("problems %+v, history_next %v, down %v", ps, m.Keys("history_next"), m.Keys("down"))
	}
	// A claim is between two actions: history_next does not claim up.
	if _, ps := Apply(Default(), map[string][]string{"history_next": {"k"}, "up": {"k"}}); len(ps) != 1 {
		t.Errorf("history_next on up's key: problems %+v", ps)
	}
	// Another action of the same scope cannot take a nav key.
	if _, ps := Apply(Default(), map[string][]string{"chat_restart": {"ctrl+n"}, "down": {"ctrl+n"}}); len(ps) != 1 {
		t.Errorf("chat_restart on down's key: problems %+v", ps)
	}
}

// ctrl+n and ctrl+p are neither the first nor the last key of down and up,
// whose first and last keys the hints show.
func TestMovingKeysKeepTheirHints(t *testing.T) {
	m := Default()
	for a, want := range map[Action][2]string{"down": {"down", "j"}, "up": {"up", "k"}} {
		ks := m.Keys(a)
		if ks[0] != want[0] || ks[len(ks)-1] != want[1] {
			t.Errorf("%s keys %v: want %s first and %s last", a, ks, want[0], want[1])
		}
	}
}
