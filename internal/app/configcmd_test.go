package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/config/ui"
	"github.com/babarot/gh-desk/internal/release"
	"github.com/babarot/gh-desk/internal/tui"
)

// runConfig runs gh desk --config path config args.
func runConfig(t *testing.T, path string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Run(append([]string{"--config", path, "config"}, args...), &out, &errb)
	return code, out.String(), errb.String()
}

func writeFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigCheck(t *testing.T) {
	t.Setenv(EnvLogLevel, "")
	tests := []struct {
		name, body string
		code       int
		want       []string
	}{
		{"fine", "[theme]\nlight = \"latte\"\n", 0, []string{"config.toml: no mistakes"}},
		{"warning", "[[tabs]]\nname = \"a\"\nquery = \"author:@me\"\n[[tabs]]\nkind = \"inbox\"\n", 0,
			[]string{"config.toml:3:9: warning: tabs[0].query has no is:pr", "no mistakes, 1 warning"}},
		{"mistake", "[theme]\nlight = \"dark\"\n", 1, []string{"theme.light must be one of"}},
		{"key mistake", "[keys]\naprove = \"a\"\n", 1, []string{`"aprove" is not an action; did you mean "approve"?`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, _ := runConfig(t, writeFile(t, tt.body), "check")
			if code != tt.code {
				t.Errorf("code = %d, want %d\n%s", code, tt.code, out)
			}
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("out =\n%s\nwant one containing %q", out, w)
				}
			}
		})
	}
	// A path after check wins over --config, and one not there is the
	// defaults.
	none := filepath.Join(t.TempDir(), "none.toml")
	if code, out, _ := runConfig(t, writeFile(t, "[theme]\nlight = \"dark\"\n"), "check", none); code != 0 || !strings.Contains(out, "the defaults are used") {
		t.Errorf("check %s: code %d, out %q", none, code, out)
	}
}

func TestConfigShow(t *testing.T) {
	path := writeFile(t, `
[[tabs]]
kind = "inbox"
[[tabs]]
name = "bots"
query = "is:open is:pr author:app/renovate"
sections = [
  { name = "can merge", merge = ["ready"] },
  { draft = true, show = "excluded" },
]

[[select]]
name = "lock files"
authors = ["renovate"]
title = "(?i)lock file"
checks = ["passing"]

[list]
refresh = "off"
[review]
editor = "code --wait"
[keys]
quit = ["q", "Q"]
`)
	code, diff, _ := runConfig(t, path, "show", "--diff")
	if code != 0 {
		t.Fatalf("show --diff: code %d", code)
	}
	for _, want := range []string{"[[tabs]]\nkind = \"inbox\"", "sections = [\n  { name = \"can merge\", merge = [\"ready\"] },\n  { draft = true, show = \"excluded\" },\n]", "[[select]]\nname = \"lock files\"",
		"title = \"(?i)lock file\"", "[list]\nrefresh = \"off\"", "editor = \"code --wait\"", "[keys]\nquit = [\"q\", \"Q\"]"} {
		if !strings.Contains(diff, want) {
			t.Errorf("show --diff =\n%s\nwant one containing %q", diff, want)
		}
	}
	for _, not := range []string{"[theme]", "stale_days", "approve ="} {
		if strings.Contains(diff, not) {
			t.Errorf("show --diff has %q, which is at its default:\n%s", not, diff)
		}
	}
	// What show prints reads back as the same settings.
	_, all, _ := runConfig(t, path, "show")
	_, again, _ := runConfig(t, writeFile(t, all), "show")
	if all != again || !strings.Contains(all, "[theme]\ndark = \"default\"") {
		t.Errorf("show does not read back as itself:\n%s\n---\n%s", all, again)
	}
}

func TestConfigReference(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	// The config file is not read, so a broken one does not matter.
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("not toml ["), 0o644); err != nil {
		t.Fatal(err)
	}
	code, all, _ := runConfig(t, path, "reference")
	if code != 0 || !strings.Contains(all, "# refresh = ") || !strings.Contains(all, "# approve = ") || strings.Contains(all, "New since") {
		t.Fatalf("reference: code %d\n%s", code, all)
	}
	for _, tt := range []struct{ name, want string }{
		{"merge", "[merge]\n"},
		{"claude", "[claude.mcp]\n"},
		{"list.refresh", "[list]\n# Refresh every tab"},
		{"select", "# [[select]]\n# name = \"small\""},
		{"select.max_lines", "[[select]]\n# The most lines"},
		{"exclude", "# [[exclude]]\n"},
		{"tabs", "# [[tabs]]\n"},
		{"keys", "[keys]\n"},
		{"keys.approve", "[keys]\n# approve = \"A\"\n"},
	} {
		if code, out, _ := runConfig(t, path, "reference", tt.name); code != 0 || !strings.Contains(out, tt.want) {
			t.Errorf("reference %s: code %d\n%s", tt.name, code, out)
		}
	}
	if code, _, errb := runConfig(t, path, "reference", "lst"); code != 1 || !strings.Contains(errb, `did you mean "list"?`) {
		t.Errorf("unknown name: code %d, err %q", code, errb)
	}
	if code, _, _ := runConfig(t, path, "reference", "a", "b"); code != 2 {
		t.Errorf("two names: code %d", code)
	}

	// The settings an update found are marked, and those gone are left out.
	if err := release.Open(filepath.Join(dir, "gh-desk")).Save(release.Record{New: []string{"list.bell", "gone.key"}, NewSince: "0.9.0"}); err != nil {
		t.Fatal(err)
	}
	_, all, _ = runConfig(t, path, "reference")
	if !strings.HasPrefix(all, "# New since v0.9.0: list.bell\n\n") || !strings.Contains(all, "# New since v0.9.0.\n# bell = ") {
		t.Errorf("marks:\n%s", all[:200])
	}
	if strings.Count(all, "New since") != 2 {
		t.Errorf("marks %d times", strings.Count(all, "New since"))
	}
	if _, out, _ := runConfig(t, path, "reference", "list.bell"); !strings.Contains(out, "# New since v0.9.0.\n") {
		t.Errorf("list.bell:\n%s", out)
	}
	// A key of an array of tables is named at the array's words.
	if err := release.Open(filepath.Join(dir, "gh-desk")).Save(release.Record{New: []string{"select.max_lines"}, NewSince: "0.9.0"}); err != nil {
		t.Fatal(err)
	}
	if _, out, _ := runConfig(t, path, "reference", "select"); !strings.Contains(out, "# New since v0.9.0: select.max_lines\n") {
		t.Errorf("select:\n%s", out)
	}
	if _, out, _ := runConfig(t, path, "reference", "list.refresh"); strings.Contains(out, "New since") {
		t.Errorf("list.refresh:\n%s", out)
	}

	// A record that cannot be read marks nothing.
	if err := os.WriteFile(filepath.Join(dir, "gh-desk", "release.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := runConfig(t, path, "reference"); code != 0 || strings.Contains(out, "New since") {
		t.Errorf("unreadable record: code %d", code)
	}
}

func TestConfigShowDiffIsAFileOfItsOwn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`
[[select]]
name = "mine"
mine = true

[list]
refresh = "1m"
home_owners = ["babarot"]

[merge]
auto_merge = false

[keys]
approve = "ctrl+a"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, diff, errb := runConfig(t, path, "show", "--diff")
	if code != 0 || !strings.Contains(diff, `refresh = "1m"`) || !strings.Contains(diff, `approve = "ctrl+a"`) {
		t.Fatalf("show --diff: code %d, err %q\n%s", code, errb, diff)
	}
	short := filepath.Join(t.TempDir(), "new.toml")
	if err := os.WriteFile(short, []byte(diff), 0o644); err != nil {
		t.Fatal(err)
	}
	_, before, _ := runConfig(t, path, "show")
	_, after, _ := runConfig(t, short, "show")
	if before != after {
		t.Errorf("settings changed:\n%s\n---\n%s", before, after)
	}
}

func applied(t *testing.T, path, src string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := applyConfig(path, false, strings.NewReader(src), &out, &errb)
	return code, out.String(), errb.String()
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestConfigApply(t *testing.T) {
	const before, after = "[list]\nbell = true\n", "[list]\nbell = false\n"
	path := writeFile(t, before)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errb := applied(t, path, after)
	if code != 0 || !strings.Contains(out, "wrote") || !strings.Contains(out, ".bak") {
		t.Fatalf("code %d, out %q, err %q", code, out, errb)
	}
	if got := read(t, path); got != after {
		t.Errorf("file %q", got)
	}
	if got := read(t, path+".bak"); got != before {
		t.Errorf("bak %q", got)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode())
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".config-*"))
	if len(left) != 0 {
		t.Errorf("left behind: %v", left)
	}
}

// What gh-desk refuses at start is refused here, whether config.Parse finds
// it or not, and the file stays as it was.
func TestConfigApplyRefuses(t *testing.T) {
	for _, tt := range []struct{ name, src, want string }{
		{"not toml", "[list\n", "config.toml"},
		{"a mistake", "[list]\nrefresh = \"soon\"\n", "list.refresh"},
		{"a key", "[keys]\napprove = 123\n", "keys.approve"},
		{"a tool", "[claude]\nallowed_tools = [\"Bash\"]\n", "claude.allowed_tools"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const before = "[list]\nbell = true\n"
			path := writeFile(t, before)
			code, _, errb := applied(t, path, tt.src)
			if code != 1 || !strings.Contains(errb, tt.want) {
				t.Errorf("code %d, err %q", code, errb)
			}
			if strings.Contains(errb, ".config-") {
				t.Errorf("the temporary file is named: %q", errb)
			}
			if got := read(t, path); got != before {
				t.Errorf("file %q", got)
			}
			if _, err := os.Stat(path + ".bak"); err == nil {
				t.Error("a .bak was written")
			}
		})
	}
}

// Nothing to apply is no mistake: a tool that ended without saving gives
// nothing, and the pipe it is in should not fail for it.
func TestConfigApplyWithNothing(t *testing.T) {
	const before = "# kept\n[list]\nbell = true\n"
	path := writeFile(t, before)
	code, _, errb := applied(t, path, " \n")
	if code != 0 || !strings.Contains(errb, "nothing to apply") {
		t.Errorf("code %d, err %q", code, errb)
	}
	if got := read(t, path); got != before {
		t.Errorf("file %q", got)
	}
	if _, err := os.Stat(path + ".bak"); err == nil {
		t.Error("a .bak was written")
	}
}

func TestConfigApplyThroughALink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.toml")
	link := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(real, []byte("[list]\nbell = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if code, _, errb := applied(t, link, "[list]\nbell = false\n"); code != 0 {
		t.Fatalf("code %d, err %q", code, errb)
	}
	if st, _ := os.Lstat(link); st.Mode()&os.ModeSymlink == 0 {
		t.Error("the link was replaced")
	}
	if got := read(t, real); !strings.Contains(got, "false") {
		t.Errorf("file %q", got)
	}
}

func TestConfigApplyToAFileThatCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	const before = "[list]\nbell = true\n"
	if err := os.WriteFile(path, []byte(before), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if code, _, errb := applied(t, path, "[list]\nbell = false\n"); code != 1 || !strings.Contains(errb, "cannot write") {
		t.Errorf("code %d, err %q", code, errb)
	}
	if got := read(t, path); got != before {
		t.Errorf("file %q", got)
	}
}

func TestConfigApplyWithNoFileYet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gh-desk", "config.toml")
	if code, out, errb := applied(t, path, "[list]\nbell = true\n"); code != 0 || strings.Contains(out, ".bak") {
		t.Fatalf("code %d, out %q, err %q", code, out, errb)
	}
}

func TestConfigInitEditSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gh-desk", "config.toml")
	if code, out, _ := runConfig(t, path, "init"); code != 0 || !strings.Contains(out, "wrote") {
		t.Fatalf("init: code %d, out %q", code, out)
	}
	if b, err := os.ReadFile(path); err != nil || strings.Count(string(b), "\n") > 5 {
		t.Errorf("the file written is %d bytes, err %v", len(b), err)
	}
	if code, out, _ := runConfig(t, path, "init"); code != 0 || !strings.Contains(out, "nothing was written") {
		t.Errorf("init again: code %d, out %q", code, out)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "true")
	if code, out, errb := runConfig(t, path, "edit"); code != 0 || !strings.Contains(out, "no mistakes") {
		t.Errorf("edit: code %d, out %q, err %q", code, out, errb)
	}
	t.Setenv("EDITOR", "")
	if code, _, errb := runConfig(t, path, "edit"); code != 1 || !strings.Contains(errb, "set review.editor") {
		t.Errorf("edit with no editor: code %d, err %q", code, errb)
	}
	if code, out, _ := runConfig(t, path, "schema"); code != 0 || out != string(config.Schema()) {
		t.Errorf("schema: code %d", code)
	}
	if code, _, _ := runConfig(t, path, "nope"); code != 2 {
		t.Errorf("unknown: code %d", code)
	}
	if code, _, _ := runConfig(t, path); code != 2 {
		t.Errorf("no command: code %d", code)
	}
}

// What scripts of the page read: the settings and the keys as a page has
// them, written so that they run without Go (mise run config-ui:test).
func TestConfigUIData(t *testing.T) {
	const path = "../config/ui/testdata/data.json"
	// cache.dir defaults to a path under the home of whoever runs this.
	t.Setenv("XDG_CACHE_HOME", "/home/you/.cache")
	shown, err := samplePreview(ui.SampleTime)
	if err != nil {
		t.Fatal(err)
	}
	judged, err := shown.judge(nil, ui.SampleTime)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.MarshalIndent(map[string]any{"DATA": config.UI(), "KEYS": uiKeys(), "LISTS": shown.lists, "JUDGED": json.RawMessage(judged)}, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, want) {
		t.Errorf("%s is not up to date; run go test ./internal/app -run TestConfigUIData -update", path)
	}
}

// The page on the site says so, and the command's does not: it is what keeps
// the page from asking a server that is not there.
func TestSettingsDemo(t *testing.T) {
	page, err := SettingsPlayground()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(page, []byte("const PLAYGROUND=true;")) {
		t.Error("the page for the site does not say it is the playground")
	}
	shown, err := samplePreview(ui.SampleTime)
	if err != nil {
		t.Fatal(err)
	}
	own, err := ui.Page(ui.Input{Keys: uiKeys(), Themes: tui.Palettes(), Spinners: tui.Spinners(), Lists: shown.lists})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(own, []byte("const PLAYGROUND=false;")) {
		t.Error("the page of the command says it is the playground")
	}
}

// What the page's code writes reads back as gh-desk reads it at start.
func TestConfigUIOutputLoads(t *testing.T) {
	const path = "../config/ui/testdata/output.toml"
	if _, _, _, err := load(path, false, new(slog.LevelVar)); err != nil {
		t.Fatal(err)
	}
	// And it says what it was given: nothing is lost or moved by being
	// written again.
	got, err := config.Raw([]byte(read(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal([]byte(read(t, "../config/ui/testdata/input.json")), &want); err != nil {
		t.Fatal(err)
	}
	g, _ := json.Marshal(got)
	w, _ := json.Marshal(want)
	if string(g) != string(w) {
		t.Errorf("output differs from input:\n%s\n%s", g, w)
	}
}

// savePage plays the browser: it reads the page, and saves src on it.
func savePage(t *testing.T, url, src string) (*http.Response, string) {
	t.Helper()
	page, err := http.Get(url)
	if err != nil {
		t.Error(err)
		return nil, ""
	}
	b, _ := io.ReadAll(page.Body)
	page.Body.Close()
	req, err := http.NewRequest(http.MethodPost, url+"save", strings.NewReader(src))
	if err != nil {
		t.Error(err)
		return nil, ""
	}
	req.Header.Set("Origin", "http://"+req.URL.Host)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Error(err)
		return nil, ""
	}
	return res, string(b)
}

// noQuestion fails a test when the terminal is asked.
func noQuestion(t *testing.T) confirmer {
	return func(_ context.Context, q string) (bool, error) {
		t.Errorf("asked %q", q)
		return false, nil
	}
}

func saved(t *testing.T, res *http.Response) (int, string) {
	t.Helper()
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res.StatusCode, string(b)
}

func TestConfigUI(t *testing.T) {
	path := writeFile(t, "[list]\nbell = true\n")
	const made = "[list]\nbell = false\n"
	var out, errb bytes.Buffer
	pages := make(chan string, 1)
	open := func(u string) error {
		// A file with a mistake stays on the page, and the second try goes.
		go func() {
			res, _ := savePage(t, u, "[keys]\napprove = 123\n")
			code, b := saved(t, res)
			if code != http.StatusUnprocessableEntity {
				t.Errorf("a mistake: %d", code)
			}
			if !strings.Contains(b, "keys.approve") || strings.Contains(b, "gh-desk-config-") {
				t.Errorf("answer %s", b)
			}
			res, page := savePage(t, u, made)
			res.Body.Close()
			pages <- page
		}()
		return nil
	}
	if code := uiConfig(t.Context(), path, false, noQuestion(t), open, &out, &errb); code != 0 {
		t.Fatalf("code %d, err %q", code, errb.String())
	}
	if got := read(t, path); got != made {
		t.Errorf("the file is %q", got)
	}
	if got := read(t, path+".bak"); !strings.Contains(got, "bell = true") {
		t.Errorf("the old file is %q", got)
	}
	if !strings.Contains(out.String(), "wrote") {
		t.Errorf("stdout %q", out.String())
	}
	if page := <-pages; !strings.Contains(page, `"bell":true`) || !strings.Contains(page, "merge.auto_merge") {
		t.Error("the page has no settings")
	}
}

// Anything but how things are drawn is saved only on a yes in the terminal,
// and what was asked about is what is written.
func TestConfigUIConfirmsInTheTerminal(t *testing.T) {
	const was = "[claude]\ncommand = \"claude\"\n"
	const made = "[claude]\ncommand = \"./evil\"\n"
	for _, tt := range []struct {
		name    string
		answer  bool
		askErr  error
		written bool
		page    string
	}{
		{name: "no", page: "not confirmed"},
		{name: "yes", answer: true, written: true},
		{name: "no terminal", askErr: errNoTerminal, page: "in a terminal"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFile(t, was)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var asked string
			ask := func(_ context.Context, q string) (bool, error) {
				asked = q
				return tt.answer, tt.askErr
			}
			var out, errb bytes.Buffer
			open := func(u string) error {
				go func() {
					res, _ := savePage(t, u, made)
					code, b := saved(t, res)
					if tt.written {
						if code != http.StatusOK {
							t.Errorf("%d %s", code, b)
						}
						return
					}
					if code != http.StatusUnprocessableEntity || !strings.Contains(b, tt.page) {
						t.Errorf("%d %s", code, b)
					}
					// The page stays open: read again, it still answers.
					if res, err := http.Get(u); err != nil {
						t.Error(err)
					} else if res.Body.Close(); res.StatusCode != http.StatusOK {
						t.Errorf("the page is gone: %d", res.StatusCode)
					}
					cancel()
				}()
				return nil
			}
			uiConfig(ctx, path, false, ask, open, &out, &errb)
			if !strings.Contains(asked, `claude.command: "claude" -> "./evil"`) {
				t.Errorf("asked %q", asked)
			}
			want := was
			if tt.written {
				want = made
			}
			if got := read(t, path); got != want {
				t.Errorf("the file is %q", got)
			}
		})
	}
}

// A file changed during the question is not overwritten by the answer.
func TestConfigUIWhenTheFileChangedDuringTheQuestion(t *testing.T) {
	path := writeFile(t, "[claude]\ncommand = \"claude\"\n")
	const other = "[claude]\ncommand = \"other\"\n"
	ask := func(context.Context, string) (bool, error) {
		return true, os.WriteFile(path, []byte(other), 0o644)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var out, errb bytes.Buffer
	open := func(u string) error {
		go func() {
			res, _ := savePage(t, u, "[claude]\ncommand = \"mine\"\n")
			if code, b := saved(t, res); code != http.StatusUnprocessableEntity || !strings.Contains(b, "changed while") {
				t.Errorf("%d %s", code, b)
			}
			cancel()
		}()
		return nil
	}
	uiConfig(ctx, path, false, ask, open, &out, &errb)
	if got := read(t, path); got != other {
		t.Errorf("the file is %q", got)
	}
}

func TestConfigUIWhenTheFileCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[list]\nbell = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var out, errb bytes.Buffer
	open := func(u string) error {
		go func() {
			res, _ := savePage(t, u, "[list]\nbell = false\n")
			if code, b := saved(t, res); code != http.StatusUnprocessableEntity || !strings.Contains(b, "New file") {
				t.Errorf("%d %s", code, b)
			}
			cancel()
		}()
		return nil
	}
	uiConfig(ctx, path, false, noQuestion(t), open, &out, &errb)
}

func TestAskLine(t *testing.T) {
	for in, want := range map[string]bool{
		"y\n":      true,
		"YES\n":    true,
		" y \n":    true,
		"\n":       false,
		"n\n":      false,
		"yep\n":    false,
		"y":        false,
		"":         false,
		"y y\n":    false,
		"no y\n":   false,
		"yes!\n":   false,
		"1\n":      false,
		"ok\n":     false,
		"true\n":   false,
		"\x1b[A\n": false,
	} {
		got, err := askLine(t.Context(), strings.NewReader(in), "save?", io.Discard)
		if err != nil || got != want {
			t.Errorf("%q: %v, %v", in, got, err)
		}
	}
}

func TestAskLineStopsOnCtrlC(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, err := askLine(ctx, r, "save?", io.Discard); got || err == nil {
		t.Errorf("%v, %v", got, err)
	}
}

// Standard input that is no terminal cannot confirm anything.
func TestTerminalConfirmerNeedsATerminal(t *testing.T) {
	f, err := os.Open(writeFile(t, "y\n"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := terminalConfirmer(f, io.Discard)(t.Context(), "save?")
	if got || !errors.Is(err, errNoTerminal) {
		t.Errorf("%v, %v", got, err)
	}
}

func TestConfirmation(t *testing.T) {
	if q := confirmation("/c", []config.Change{{Key: "theme.dark", Display: true}}); q != "" {
		t.Errorf("asked %q", q)
	}
	q := confirmation("/c", []config.Change{{Key: "theme.dark", Display: true}, {Key: "merge.auto_merge", Was: "false", Now: "true"}})
	if !strings.Contains(q, "merge.auto_merge: false -> true") || !strings.Contains(q, "and 1 that change") {
		t.Errorf("asked %q", q)
	}
}

func TestConfigUIWhenTheFileChanged(t *testing.T) {
	path := writeFile(t, "[list]\nbell = true\n")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var out, errb bytes.Buffer
	open := func(u string) error {
		go func() {
			if err := os.WriteFile(path, []byte("[list]\nbell = false\n"), 0o644); err != nil {
				t.Error(err)
			}
			res, _ := savePage(t, u, "[list]\nstale_days = 9\n")
			b, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(b), "changed while") {
				t.Errorf("%d %s", res.StatusCode, b)
			}
			cancel()
		}()
		return nil
	}
	if code := uiConfig(ctx, path, false, noQuestion(t), open, &out, &errb); code != 1 || out.Len() != 0 {
		t.Errorf("code %d, out %q", code, out.String())
	}
}

func TestConfigUIOpensNothingForAMistake(t *testing.T) {
	var out, errb bytes.Buffer
	opened := false
	open := func(string) error { opened = true; return nil }
	if code := uiConfig(t.Context(), writeFile(t, "[keys]\napprove = 123\n"), false, noQuestion(t), open, &out, &errb); code != 1 || opened {
		t.Errorf("code %d, opened %v", code, opened)
	}
}

func TestConfigUIClosed(t *testing.T) {
	var out, errb bytes.Buffer
	open := func(u string) error {
		go func() {
			req, _ := http.NewRequest(http.MethodPost, u+"close", nil)
			req.Header.Set("Origin", "http://"+req.URL.Host)
			if res, err := http.DefaultClient.Do(req); err != nil {
				t.Error(err)
			} else {
				res.Body.Close()
			}
		}()
		return nil
	}
	if code := uiConfig(t.Context(), writeFile(t, "[list]\nbell = true\n"), false, noQuestion(t), open, &out, &errb); code != 0 || out.Len() != 0 {
		t.Errorf("code %d, out %q, err %q", code, out.String(), errb.String())
	}
}

// ctrl+c leaves without saving, and with no browser it still waits.
func TestConfigUILeftWithoutSaving(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var out, errb bytes.Buffer
	open := func(string) error { cancel(); return errors.New("no browser") }
	code := uiConfig(ctx, filepath.Join(t.TempDir(), "config.toml"), false, noQuestion(t), open, &out, &errb)
	if code != 1 || out.Len() != 0 || !strings.Contains(errb.String(), "nothing was saved") {
		t.Errorf("code %d, out %q, err %q", code, out.String(), errb.String())
	}
}

// Every scope the config file takes is one the TUI narrows by.
func TestScopesAreTheTUIs(t *testing.T) {
	if !slices.Equal(config.Scopes, []string{tui.ScopeOwner, tui.ScopeRepo, tui.ScopeAuthor}) {
		t.Errorf("config.Scopes = %q", config.Scopes)
	}
}
