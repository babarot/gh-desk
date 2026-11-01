package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-triage/internal/keymap"
)

func TestHTMLToMarkdown(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"coverage report",
			`<h2>Coverage Report</h2> <table> <thead> <tr> <th align="center">Status</th> <th align="left">Category</th> <th align="right">Percentage</th> </tr> </thead> <tbody> <tr> <td align="center">🔵</td> <td align="left">Lines</td> <td align="right">70.5% <em>±0%</em></td> </tr> </tbody> </table><details><summary>File Coverage</summary>No changed files found.</details>` + "\n" +
				`<em>Generated in workflow <a href=https://github.com/acme/api/actions/runs/1>#4</a> for commit <a href="https://github.com/acme/api/commit/aaaaaaa">aaaaaaa</a></em>`,
			"## Coverage Report\n\n| Status | Category | Percentage |\n|---|---|---|\n| 🔵 | Lines | 70.5% ±0% |\n\n▸ File Coverage\nNo changed files found.\nGenerated in workflow #4 for commit aaaaaaa"},
		{"lists and breaks",
			"<ul>\n  <li>one <b>bold</b></li>\n  <li>two<ol start=\"3\"><li>three</li><li>four</li></ol></li>\n</ul>\nline<br>next &amp; last",
			"- one **bold**\n- two\n  3. three\n  4. four\nline\nnext & last"},
		{"markdown and code stay",
			"Use `<div>` and *this*, see <https://example.com>.\n\n```html\n<p>kept</p>\n```\n<pre>a &lt; b\n</pre>",
			"Use `<div>` and *this*, see https://example.com.\n\n```html\n<p>kept</p>\n```\n```\na < b\n```"},
		{"no html", "plain *text*", "plain *text*"},
		{"entities are not code spans", "&#xE000;0&#xE001; and `&amp;`", "\uE0000\uE001 and `&amp;`"},
	}
	for _, c := range cases {
		if got := htmlToMarkdown(c.in); got != c.want {
			t.Errorf("%s:\n%q\nwant:\n%q", c.name, got, c.want)
		}
	}
}

func TestHTMLIsDrawn(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	var got []string
	for _, l := range m.bodyLines("<h2>Report</h2><table><tr><th>a</th><th>b</th></tr><tr><td>1</td><td>22</td></tr></table>", 30) {
		got = append(got, ansi.Strip(l.text))
	}
	want := []string{"Report", "a │ b ", "──┼───", "1 │ 22"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("lines:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
