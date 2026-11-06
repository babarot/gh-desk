package release

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompare(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want int
	}{
		{"0.8.0", "0.7.0", 1},
		{"v0.7.0", "0.7.0", 0},
		{"0.7.0", "0.10.0", -1},
		{"1.0.0", "0.99.99", 1},
	} {
		if got, err := Compare(tt.a, tt.b); err != nil || got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, %v; want %d", tt.a, tt.b, got, err, tt.want)
		}
	}
	for _, bad := range []string{"", "0.8", "0.8.0-rc.1", "x.y.z", "0.-1.0"} {
		if _, err := Compare(bad, "0.7.0"); err == nil {
			t.Errorf("Compare(%q) parsed", bad)
		}
		if Newer(bad, "0.7.0") {
			t.Errorf("Newer(%q) = true", bad)
		}
	}
}

func TestMethodOf(t *testing.T) {
	ext := "/home/me/.local/share/gh/extensions"
	for _, tt := range []struct {
		exe  string
		want Method
	}{
		{"/nix/store/abc-gh-desk-0.7.0/bin/gh-desk", Nix},
		{"/home/me/.local/share/gh/extensions/gh-desk/gh-desk", Extension},
		{"/home/me/.local/share/gh/extensions-old/gh-desk", Download},
		{"/usr/local/bin/gh-desk", Download},
	} {
		if got := MethodOf(tt.exe, ext); got != tt.want {
			t.Errorf("MethodOf(%q) = %s, want %s", tt.exe, got, tt.want)
		}
	}
	if MethodOf("/usr/local/bin/gh-desk", "") != Download {
		t.Error("with no extensions directory, a binary is a download")
	}
	for m, want := range map[Method]string{Nix: "update it with Nix", Extension: "gh extension upgrade desk", Download: Page} {
		if m.How() != want {
			t.Errorf("%s.How() = %q", m, m.How())
		}
	}
}

func TestDue(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		checked time.Time
		want    bool
	}{
		{time.Time{}, true},
		{now.Add(-time.Hour), false},
		{now.Add(-CheckEvery), true},
		{now.Add(time.Hour), true}, // a clock set back
	} {
		if got := (Record{Checked: tt.checked}).Due(now); got != tt.want {
			t.Errorf("Due with checked %v = %v", tt.checked, got)
		}
	}
}

func TestStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	s := Open(dir)
	r, err := s.Load()
	if err != nil || r != (Record{}) {
		t.Fatalf("Load with no file = %+v, %v", r, err)
	}
	want := Record{Checked: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC), Latest: "0.8.0", Told: "0.8.0", LastRun: "0.7.0"}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Load(); err != nil || got != want {
		t.Errorf("Load = %+v, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "release.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil {
		t.Error("a broken file loaded")
	}
}

// The body of v0.7.0 as GitHub generated it.
const v070 = "<!-- Release notes generated using configuration in .github/release.yml at 4e1d4716d7fb6dd5421933eeb99c749b9e0227af -->\r\n\r\n" +
	"## What's Changed\r\n### New Features\r\n" +
	"* Let the filter ask one field with author:, repo: and title: by @babarot in https://github.com/babarot/gh-desk/pull/66\r\n" +
	"### Improvements\r\n" +
	"* Let ctrl+w in the filter stop at a slash or a colon by @babarot in https://github.com/babarot/gh-desk/pull/65\r\n" +
	"* Bump the dependencies by @renovate[bot] in https://github.com/babarot/gh-desk/pull/70\r\n" +
	"\r\n\r\n**Full Changelog**: https://github.com/babarot/gh-desk/compare/v0.6.0...v0.7.0"

func TestNotes(t *testing.T) {
	want := "### New Features\n" +
		"* Let the filter ask one field with author:, repo: and title: (#66)\n" +
		"### Improvements\n" +
		"* Let ctrl+w in the filter stop at a slash or a colon (#65)\n" +
		"* Bump the dependencies (#70)"
	if got := Notes(v070); got != want {
		t.Errorf("Notes =\n%s\nwant\n%s", got, want)
	}
	if got := Notes("Written by hand, by @alice."); got != "Written by hand, by @alice." {
		t.Errorf("Notes changed a hand-written body: %q", got)
	}
}
