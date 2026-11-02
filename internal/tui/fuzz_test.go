package tui

import (
	"strings"
	"testing"
)

// Text from GitHub is cleaned before it is drawn. What is left has no
// escape sequence and no control character but the newline, whatever was
// written, so nothing in it can move the cursor or retitle the terminal.
func FuzzSafe(f *testing.F) {
	f.Add("plain title")
	f.Add("\x1b[2J\x1b[Hcleared")
	f.Add("\x1b]0;new title\x07rest")
	f.Add("\x9b31mC1 CSI")
	f.Add("tab\there\r\nline\x00\x7f")
	f.Add("日本語のタイトル")
	f.Fuzz(func(t *testing.T, s string) {
		out := safe(s)
		for _, r := range out {
			if r == '\n' {
				continue
			}
			if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
				t.Fatalf("safe(%q) = %q keeps %U", s, out, r)
			}
		}
		if strings.Contains(safeLine(s), "\n") {
			t.Fatalf("safeLine(%q) keeps a newline", s)
		}
	})
}

// HTML in a description is written by whoever opened the pull request.
// Turning it into Markdown never panics, whatever it holds.
func FuzzHTMLToMarkdown(f *testing.F) {
	f.Add("<p>Hello <b>world</b></p>")
	f.Add("<details><summary>More</summary>\n\n- a\n- b\n</details>")
	f.Add("<table><tr><td>a</td><td>b</td></tr></table>")
	f.Add("<ol start=\"3\"><li>x</li></ol>")
	f.Add("<a href=\"https://example.com\">link</a> &amp; &lt;tag&gt;")
	f.Add("<ul><li><ol><li>")
	f.Fuzz(func(t *testing.T, body string) {
		htmlToMarkdown(body)
	})
}
