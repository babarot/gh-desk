package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-triage/internal/diff"
	"github.com/babarot/gh-triage/internal/model"
)

// Text from GitHub (titles, descriptions, comments, diffs, branch names)
// is written by anyone who can open a pull request. It is cleaned when it
// arrives, before anything draws it: escape sequences would otherwise
// reach the terminal, moving the cursor, changing the title or worse.

// safe removes escape sequences and other control characters from s, and
// writes a tab as four spaces.
func safe(s string) string {
	s = strings.ReplaceAll(ansi.Strip(s), "\t", "    ")
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, s)
}

func safeLine(s string) string { return strings.ReplaceAll(safe(s), "\n", " ") }

func safePR(p model.PR) model.PR {
	p.Title, p.Author = safeLine(p.Title), safeLine(p.Author)
	p.HeadRef, p.BaseRef = safeLine(p.HeadRef), safeLine(p.BaseRef)
	p.Requested = append([]model.Reviewer(nil), p.Requested...)
	for i := range p.Requested {
		p.Requested[i].Login = safeLine(p.Requested[i].Login)
	}
	p.Reviews = append([]model.Review(nil), p.Reviews...)
	for i := range p.Reviews {
		p.Reviews[i].Author = safeLine(p.Reviews[i].Author)
	}
	p.Labels = append([]string(nil), p.Labels...)
	for i := range p.Labels {
		p.Labels[i] = safeLine(p.Labels[i])
	}
	return p
}

func safePRs(ps []model.PR) []model.PR {
	out := make([]model.PR, len(ps))
	for i, p := range ps {
		out[i] = safePR(p)
	}
	return out
}

func safeNotification(n model.Notification) model.Notification {
	n.Title, n.Reason = safeLine(n.Title), safeLine(n.Reason)
	return n
}

func safeComment(c model.Comment) model.Comment {
	c.Author, c.Body = safeLine(c.Author), safe(c.Body)
	return c
}

func safeDetail(d model.Detail) model.Detail {
	d.Body = safe(d.Body)
	d.Checks = append([]model.Check(nil), d.Checks...)
	for i := range d.Checks {
		d.Checks[i].Name = safeLine(d.Checks[i].Name)
	}
	cs := make([]model.Comment, len(d.Comments))
	for i, c := range d.Comments {
		cs[i] = safeComment(c)
	}
	d.Comments = cs
	return d
}

func safeFiles(fs []diff.File) []diff.File {
	for i := range fs {
		f := &fs[i]
		f.OldPath, f.NewPath = safeLine(f.OldPath), safeLine(f.NewPath)
		for j := range f.Hunks {
			h := &f.Hunks[j]
			h.Section = safeLine(h.Section)
			for k := range h.Lines {
				h.Lines[k].Text = safeLine(h.Lines[k].Text)
			}
		}
	}
	return fs
}
