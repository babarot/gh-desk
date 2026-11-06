package config

import (
	"slices"
	"strings"
	"testing"
)

// What saves on the settings page without a question is this list. Adding
// to it takes a reason: see the comment of setting.Display and ADR-101.
func TestDisplaySettings(t *testing.T) {
	var got []string
	for _, s := range settings {
		if s.Display {
			got = append(got, s.Key)
		}
	}
	want := []string{
		"list.stale_days", "list.columns", "list.bell",
		"preview.height", "preview.tab",
		"description.style",
		"diff.collapse",
		"theme.dark", "theme.light", "theme.spinner", "theme.shimmer",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %q", got)
	}
}

func changed(t *testing.T, old, new string) []Change {
	t.Helper()
	got, err := Changes([]byte(old), []byte(new))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestChanges(t *testing.T) {
	for _, tt := range []struct {
		name, old, new string
		want           []string
		display        bool
	}{
		{name: "the same, written otherwise", old: "# a comment\n[list]\nbell = true\n", new: "[list]\nbell=true\n"},
		{name: "a value", old: "[claude]\ncommand = \"a\"\n", new: "[claude]\ncommand = \"b\"\n",
			want: []string{`claude.command: "a" -> "b"`}},
		{name: "back to the default", old: "[checks]\napproval = [\"x\"]\n", new: "",
			want: []string{`checks.approval: ["x"] -> (removed)`}},
		{name: "a new key", old: "", new: "[review]\neditor = \"vim\"\n",
			want: []string{`review.editor: (not set) -> "vim"`}},
		{name: "drawn only", old: "", new: "[theme]\ndark = \"x\"\n", display: true,
			want: []string{`theme.dark: (not set) -> "x"`}},
		{name: "a key nobody marked", old: "", new: "[keys]\napprove = \"z\"\n",
			want: []string{`keys.approve: (not set) -> "z"`}},
		{name: "a name that is not the setting it spells", old: "", new: "\"theme.dark\" = \"x\"\n",
			want: []string{`"theme.dark": (not set) -> "x"`}},
		{name: "a rule added", old: "[[auto_approve]]\nbot = true\n", new: "[[auto_approve]]\nbot = true\n[[auto_approve]]\nmine = true\n",
			want: []string{`auto_approve: (not set) -> {mine = true}`}},
		{name: "a rule edited", old: "[[exclude]]\nbot = true\n", new: "[[exclude]]\nbot = false\n",
			want: []string{`exclude: {bot = true} -> (removed)`, `exclude: (not set) -> {bot = false}`}},
		{name: "rules reordered", old: "[[select]]\nname = \"a\"\n[[select]]\nname = \"b\"\n", new: "[[select]]\nname = \"b\"\n[[select]]\nname = \"a\"\n",
			want: []string{`select: (in another order) -> (in another order)`}},
		{name: "all rules gone", old: "[[select]]\nname = \"a\"\n", new: "select = []\n",
			want: []string{`select: {name = "a"} -> (removed)`}},
		{name: "a server's command and args", old: "", new: "[claude.mcp.servers.x]\ncommand = \"sh\"\nargs = [\"-c\", \"id\"]\n",
			want: []string{`claude.mcp.servers.x.args: (not set) -> ["-c", "id"]`, `claude.mcp.servers.x.command: (not set) -> "sh"`}},
		{name: "a secret stays out", old: "", new: "[claude.mcp.servers.x.env]\nTOKEN = \"s3cret\"\n",
			want: []string{`claude.mcp.servers.x.env.TOKEN: (not set) -> (hidden)`}},
		{name: "a header too", old: "", new: "[claude.mcp.servers.x.headers]\nAuthorization = \"s3cret\"\n",
			want: []string{`claude.mcp.servers.x.headers.Authorization: (not set) -> (hidden)`}},
		{name: "a URL without its credentials", old: "", new: "[claude.mcp.servers.x]\nurl = \"https://u:s3cret@example.com/a?k=s3cret\"\n",
			want: []string{`claude.mcp.servers.x.url: (not set) -> "https://example.com/a" (its credentials hidden)`}},
		{name: "control characters are escaped", old: "", new: "[claude]\ninstructions = \"a\\u001b[2Jb\\nc\\u202ed\"\n",
			want: []string{`claude.instructions: (not set) -> "a\x1b[2Jb\nc\u202ed"`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, c := range changed(t, tt.old, tt.new) {
				got = append(got, c.String())
				if c.Display != tt.display {
					t.Errorf("%s: display %v", c.Key, c.Display)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %q\nwant %q", got, tt.want)
			}
			if strings.Contains(strings.Join(got, "\n"), "s3cret") {
				t.Errorf("a secret is shown: %q", got)
			}
		})
	}
}

func TestChangesOfAFileThatDoesNotRead(t *testing.T) {
	if _, err := Changes([]byte("not toml ["), nil); err == nil {
		t.Error("no error")
	}
}
