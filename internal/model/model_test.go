package model

import "testing"

func TestParsePRKey(t *testing.T) {
	tests := []struct {
		in   string
		want PRKey
		ok   bool
	}{
		{"babarot/gomi#140", PRKey{"babarot", "gomi", 140}, true},
		{"babarot/gomi", PRKey{}, false},
		{"gomi#140", PRKey{}, false},
		{"babarot/gomi#0", PRKey{}, false},
		{"babarot/gomi#x", PRKey{}, false},
		{"a/b/c#1", PRKey{}, false},
		{"/gomi#1", PRKey{}, false},
	}
	for _, tt := range tests {
		got, err := ParsePRKey(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("ParsePRKey(%q) = %v, %v; want %v, ok=%v", tt.in, got, err, tt.want, tt.ok)
		}
		if tt.ok && got.String() != tt.in {
			t.Errorf("ParsePRKey(%q).String() = %q", tt.in, got.String())
		}
	}
}
