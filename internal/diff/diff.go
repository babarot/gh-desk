// Package diff reads the patches git diff gives, file by file, and finds
// the words that differ between a line and the one that replaced it.
//
// git (or GitHub, for a pull request's diff) computes the diffs; this only
// reads them. It was taken from gh-mini, which reads the same patches. Should it need to read what they do not cover, such as
// copies, github.com/bluekeyes/go-gitdiff reads git's patches whole, and
// Parse could fill File from it instead.
package diff

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)

// File is one file's patch.
type File struct {
	// OldPath and NewPath are the file before and after; one of them is
	// empty for a file added or deleted.
	OldPath string
	NewPath string
	// OldMode and NewMode are set when the mode changed.
	OldMode string
	NewMode string
	Binary  bool
	Hunks   []Hunk
}

// Path is the file's path now, or before it was deleted.
func (f *File) Path() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// Stat counts the lines added and deleted.
func (f *File) Stat() (added, deleted int) {
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			switch l.Kind {
			case Added:
				added++
			case Deleted:
				deleted++
			}
		}
	}
	return added, deleted
}

// Hunk is a run of lines changed, with those around them.
type Hunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	// Section is what git writes after the line numbers, often the
	// function the lines are in.
	Section string
	Lines   []Line
}

// Kind tells a line added, deleted or kept.
type Kind byte

const (
	Context Kind = ' '
	Added   Kind = '+'
	Deleted Kind = '-'
)

// Line is one line of a hunk, without its newline.
type Line struct {
	Kind Kind
	Text string
	// Old and New are its numbers in the file before and after, zero on
	// the side it is not in.
	Old, New int
	// NoNewline is set on a last line with no newline after it.
	NoNewline bool
}

// Parse reads the output of git diff -p, as git diff-index and diff-files
// give it with their default prefixes.
func Parse(b []byte) []File {
	var files []File
	var f *File
	var h *Hunk
	old, cur := 0, 0
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64<<10), 64<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "diff --git ") {
			files = append(files, File{})
			f = &files[len(files)-1]
			h = nil
			f.OldPath, f.NewPath = gitPaths(line[len("diff --git "):])
			continue
		}
		if f == nil {
			continue
		}
		if h != nil {
			switch {
			case strings.HasPrefix(line, "+"):
				h.Lines = append(h.Lines, Line{Kind: Added, Text: line[1:], New: cur})
				cur++
				continue
			case strings.HasPrefix(line, "-"):
				h.Lines = append(h.Lines, Line{Kind: Deleted, Text: line[1:], Old: old})
				old++
				continue
			case strings.HasPrefix(line, " "), line == "":
				h.Lines = append(h.Lines, Line{Kind: Context, Text: strings.TrimPrefix(line, " "), Old: old, New: cur})
				old++
				cur++
				continue
			case strings.HasPrefix(line, `\`):
				if n := len(h.Lines); n > 0 {
					h.Lines[n-1].NoNewline = true
				}
				continue
			}
		}
		switch {
		case strings.HasPrefix(line, "@@ "):
			hk, ok := parseHunkHeader(line)
			if !ok {
				continue
			}
			f.Hunks = append(f.Hunks, hk)
			h = &f.Hunks[len(f.Hunks)-1]
			old, cur = hk.OldStart, hk.NewStart
		case strings.HasPrefix(line, "--- "):
			f.OldPath = side(line[4:], "a/")
		case strings.HasPrefix(line, "+++ "):
			f.NewPath = side(line[4:], "b/")
		case strings.HasPrefix(line, "rename from "):
			f.OldPath = unquote(line[len("rename from "):])
		case strings.HasPrefix(line, "rename to "):
			f.NewPath = unquote(line[len("rename to "):])
		case strings.HasPrefix(line, "new file mode "):
			f.OldPath = ""
		case strings.HasPrefix(line, "deleted file mode "):
			f.NewPath = ""
		case strings.HasPrefix(line, "old mode "):
			f.OldMode = line[len("old mode "):]
		case strings.HasPrefix(line, "new mode "):
			f.NewMode = line[len("new mode "):]
		case strings.HasPrefix(line, "Binary files "):
			f.Binary = true
		}
	}
	return files
}

// side reads the path of a ---/+++ line: "" for /dev/null. git ends a
// path that has a space in it with a tab.
func side(s, prefix string) string {
	s = strings.TrimSuffix(s, "\t")
	if s == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(unquote(s), prefix)
}

// gitPaths reads the two paths of a "diff --git a/x b/y" line. A path may
// hold spaces, so this is only a guess where no ---/+++ or rename line
// follows to tell them, as for a binary file: the two halves of a file
// that kept its name are the same.
func gitPaths(s string) (oldPath, newPath string) {
	if strings.HasPrefix(s, `"`) {
		if a, rest, ok := cutQuoted(s); ok {
			return strings.TrimPrefix(a, "a/"), strings.TrimPrefix(unquote(strings.TrimSpace(rest)), "b/")
		}
	}
	if n := len(s); n%2 == 1 && s[:n/2] == "a/"+s[n/2+3:] && s[n/2:n/2+3] == " b/" {
		p := s[2 : n/2]
		return p, p
	}
	a, b, _ := strings.Cut(s, " b/")
	return strings.TrimPrefix(a, "a/"), unquote(b)
}

// cutQuoted reads a quoted path off the front of s.
func cutQuoted(s string) (path, rest string, ok bool) {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			p, err := strconv.Unquote(s[:i+1])
			return p, s[i+1:], err == nil
		}
	}
	return "", s, false
}

// unquote reads a path git quoted, as it does one with a double quote, a
// backslash or a control character in it, and any not ASCII unless
// core.quotePath is off. git's escapes are C's, which Go reads too.
func unquote(s string) string {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	if p, err := strconv.Unquote(s); err == nil {
		return p
	}
	return s
}

func parseHunkHeader(line string) (Hunk, bool) {
	// @@ -1,3 +1,4 @@ section
	rest := line[3:]
	ranges, section, ok := strings.Cut(rest, " @@")
	if !ok {
		return Hunk{}, false
	}
	o, n, ok := strings.Cut(ranges, " ")
	if !ok || !strings.HasPrefix(o, "-") || !strings.HasPrefix(n, "+") {
		return Hunk{}, false
	}
	h := Hunk{Section: strings.TrimPrefix(section, " ")}
	h.OldStart, h.OldLines = lineRange(o[1:])
	h.NewStart, h.NewLines = lineRange(n[1:])
	return h, true
}

// lineRange reads "start,count", where a count of one is left out.
func lineRange(s string) (start, count int) {
	a, b, ok := strings.Cut(s, ",")
	start, _ = strconv.Atoi(a)
	count = 1
	if ok {
		count, _ = strconv.Atoi(b)
	}
	return start, count
}
