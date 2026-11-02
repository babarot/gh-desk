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
			check: map[Action][]string{"approve": {"a"}, "show_settled": nil},
		},
		{
			name:  "turn off",
			set:   map[string][]string{"claude": {}},
			check: map[Action][]string{"claude": {}},
		},
		{
			name:  "set key takes it from a default nav key",
			set:   map[string][]string{"approve": {"j"}},
			check: map[Action][]string{"approve": {"j"}, "down": {"down"}},
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
	if got := m.Actions(ScopeConfirm); !slices.Equal(got, []Action{"confirm", "include_failing"}) {
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
