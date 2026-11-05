package mcpserver

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/babarot/gh-desk/internal/model"
)

// What the tools return is text for Claude, inside tags that mark it as
// written by others, as the pull request's description and diff are in the
// prompt.

// How much of a log Claude is given: the lines around each error, and the
// end. Constants, as they size what one answer costs.
const (
	errorContext = 5
	maxErrorRuns = 20
	tailLines    = 150
	maxLogText   = 40_000
	// maxText is the most of one body or comment given.
	maxText = 8_000
)

func formatChecks(sets []model.CommitChecks) string {
	var b strings.Builder
	for _, cs := range sets {
		fmt.Fprintf(&b, "<checks commit=%q label=%q>\n", cs.OID, cs.Label)
		if len(cs.Checks) == 0 {
			b.WriteString("(no checks)\n")
		}
		for _, c := range cs.Checks {
			if c.ID == 0 {
				fmt.Fprintf(&b, "status %q: %s", c.Name, c.State)
				if c.Description != "" {
					fmt.Fprintf(&b, " (%s)", oneLine(c.Description))
				}
			} else {
				fmt.Fprintf(&b, "check run id=%d %q: %s", c.ID, c.Name, c.State)
				if c.Workflow != "" {
					fmt.Fprintf(&b, ", workflow %q", c.Workflow)
				}
				if c.App != "" {
					fmt.Fprintf(&b, ", app %s", c.App)
				}
			}
			if c.Required {
				b.WriteString(", required")
			}
			if c.URL != "" {
				b.WriteString(", " + c.URL)
			}
			b.WriteString("\n")
		}
		if cs.More {
			b.WriteString("(there are more checks than these)\n")
		}
		b.WriteString("</checks>\n")
	}
	return b.String()
}

func formatCheckRun(d model.CheckRunDetail, log []byte, hasLog bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<check_run id=%d name=%q status=%q conclusion=%q app=%q url=%q>\n", d.ID, d.Name, d.Status, d.Conclusion, d.App, d.URL)
	for _, p := range []struct{ name, text string }{{"title", d.Title}, {"summary", d.Summary}, {"text", d.Text}} {
		if strings.TrimSpace(p.text) != "" {
			fmt.Fprintf(&b, "<%s>\n%s\n</%s>\n", p.name, cut(p.text, maxText), p.name)
		}
	}
	if d.AnnotationCount > 0 {
		fmt.Fprintf(&b, "<annotations count=%d shown=%d>\n", d.AnnotationCount, len(d.Annotations))
		for _, a := range d.Annotations {
			fmt.Fprintf(&b, "%s:%d %s %s: %s\n", a.Path, a.Line, a.Level, oneLine(a.Title), cut(a.Message, 1000))
		}
		b.WriteString("</annotations>\n")
	}
	switch {
	case d.App != "github-actions":
		b.WriteString("(not a GitHub Actions job: it has no log here; its details are at its url)\n")
	case !hasLog:
		b.WriteString("(GitHub has no log for this job: it was canceled, or its log expired)\n")
	default:
		b.WriteString("<log>\n" + trimLog(string(log)) + "</log>\n")
	}
	b.WriteString("</check_run>\n")
	return b.String()
}

var (
	ansi      = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	timestamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z ?`)
)

// trimLog is the part of an Actions log Claude is given: the lines around
// each error (##[error], or a line saying error or failed), and the end,
// without timestamps and terminal escapes, at most maxLogText.
func trimLog(log string) string {
	lines := strings.Split(strings.TrimRight(log, "\n"), "\n")
	for i, l := range lines {
		l = ansi.ReplaceAllString(l, "")
		l = timestamp.ReplaceAllString(l, "")
		lines[i] = strings.TrimRight(l, "\r")
	}
	keep := make([]bool, len(lines))
	runs := 0
	for i, l := range lines {
		if !isError(l) {
			continue
		}
		if i == 0 || !keep[i] {
			runs++
		}
		if runs > maxErrorRuns {
			break
		}
		for j := max(0, i-errorContext); j <= min(len(lines)-1, i+errorContext); j++ {
			keep[j] = true
		}
	}
	for j := max(0, len(lines)-tailLines); j < len(lines); j++ {
		keep[j] = true
	}
	var b strings.Builder
	skipped := false
	for i, l := range lines {
		if !keep[i] {
			skipped = true
			continue
		}
		if skipped {
			b.WriteString("[…]\n")
			skipped = false
		}
		b.WriteString(l + "\n")
	}
	out := b.String()
	if len(out) > maxLogText {
		out = "[… the start is cut]\n" + out[len(out)-maxLogText:]
	}
	return out
}

func isError(l string) bool {
	if strings.Contains(l, "##[error]") {
		return true
	}
	low := strings.ToLower(l)
	return strings.Contains(low, "error:") || strings.Contains(low, "error ") && strings.HasPrefix(strings.TrimSpace(low), "error") ||
		strings.Contains(low, " failed") || strings.HasPrefix(strings.TrimSpace(low), "fail")
}

func formatHits(hits []model.IssueHit) string {
	var b strings.Builder
	b.WriteString("<search_results>\n")
	if len(hits) == 0 {
		b.WriteString("(none)\n")
	}
	for _, h := range hits {
		kind := "issue"
		if h.PR {
			kind = "pr"
		}
		fmt.Fprintf(&b, "#%d %s %s by %s, updated %s: %s", h.Number, kind, h.State, h.Author, h.Updated.Format("2006-01-02"), oneLine(h.Title))
		if len(h.Labels) > 0 {
			fmt.Fprintf(&b, " [%s]", strings.Join(h.Labels, ", "))
		}
		b.WriteString("\n")
	}
	b.WriteString("</search_results>\n")
	return b.String()
}

func formatIssue(d model.IssueDetail) string {
	kind := "issue"
	if d.PR {
		kind = "pull_request"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<%s number=%d state=%q author=%q>\n", kind, d.Number, d.State, d.Author)
	fmt.Fprintf(&b, "<title>%s</title>\n", oneLine(d.Title))
	if len(d.Labels) > 0 {
		fmt.Fprintf(&b, "<labels>%s</labels>\n", strings.Join(d.Labels, ", "))
	}
	fmt.Fprintf(&b, "<body>\n%s\n</body>\n", cut(d.Body, maxText))
	fmt.Fprintf(&b, "<comments count=%d shown=%d>\n", d.CommentCount, len(d.Comments))
	for _, c := range d.Comments {
		fmt.Fprintf(&b, "%s wrote on %s:\n%s\n\n", c.Author, c.Created.Format("2006-01-02"), cut(c.Body, maxText))
	}
	b.WriteString("</comments>\n")
	if d.PR {
		fmt.Fprintf(&b, "<reviews count=%d shown=%d>\n", d.ReviewCount, len(d.Reviews))
		for _, r := range d.Reviews {
			fmt.Fprintf(&b, "%s: %s\n", r.Author, r.State)
			if strings.TrimSpace(r.Body) != "" {
				b.WriteString(cut(r.Body, maxText) + "\n")
			}
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "</reviews>\n<review_threads count=%d shown=%d>\n", d.ThreadCount, len(d.Threads))
		for _, t := range d.Threads {
			state := "open"
			if t.Resolved {
				state = "resolved"
			}
			fmt.Fprintf(&b, "%s:%d (%s, comments: %d)\n", t.Path, t.Line, state, t.CommentCount)
			for _, c := range t.Comments {
				fmt.Fprintf(&b, "  %s: %s\n", c.Author, strings.ReplaceAll(cut(c.Body, maxText), "\n", "\n  "))
			}
		}
		b.WriteString("</review_threads>\n")
		fmt.Fprintf(&b, "<files count=%d shown=%d>\n", d.FileCount, len(d.Files))
		for _, f := range d.Files {
			fmt.Fprintf(&b, "%s +%d -%d\n", f.Path, f.Additions, f.Deletions)
		}
		b.WriteString("</files>\n")
		if len(d.Closes) > 0 {
			var ns []string
			for _, r := range d.Closes {
				ns = append(ns, fmt.Sprintf("#%d", r.Number))
			}
			fmt.Fprintf(&b, "<closes>%s</closes>\n", strings.Join(ns, ", "))
		}
	}
	fmt.Fprintf(&b, "</%s>\n", kind)
	return b.String()
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// cut is s, at most n bytes of it, saying so when cut.
func cut(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + "\n[… cut]"
}
