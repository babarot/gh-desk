package config

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The types here check a value as it is read. The decoder stops at the
// first error a value returns, so each keeps what was written and what is
// wrong with it instead, and Load reports them all at once.

// Duration is a Go duration setting, such as "3m", or "off" where the
// setting allows it. TOML has no type for a duration; a number is handed
// over as its digits, and is wrong for want of a unit.
type Duration struct {
	D time.Duration
	// Off is set when "off" was written.
	Off bool
	raw string
	ok  bool
}

func (d *Duration) UnmarshalText(b []byte) error {
	*d = parseDuration(string(b))
	return nil
}

func parseDuration(s string) Duration {
	d := Duration{raw: s}
	if s == "off" {
		d.Off, d.ok = true, true
		return d
	}
	v, err := time.ParseDuration(s)
	d.D, d.ok = v, err == nil && v > 0
	return d
}

// dur is a Duration for a default.
func dur(s string) Duration { return parseDuration(s) }

// String is the duration as written.
func (d Duration) String() string { return d.raw }

// problem says what is wrong with d at setting, if anything.
func (d Duration) problem(setting string, allowOff bool) (Problem, bool) {
	off := ""
	if allowOff {
		off = `, or "off"`
	}
	v, err := time.ParseDuration(d.raw)
	switch {
	case d.ok && (!d.Off || allowOff):
		return Problem{}, false
	case d.raw == "0" || err == nil && v <= 0:
		return problem(setting, "%s must be more than 0%s, got %s", setting, off, d.raw), true
	case isDigits(d.raw):
		return problem(setting, `%s must be a duration with its unit, such as "3m"%s, got %s`, setting, off, d.raw), true
	}
	return problem(setting, `%s must be a duration such as "3m"%s, got %q`, setting, off, d.raw), true
}

// Days is a number of days, or "off".
type Days struct {
	N int
	// Off is set when "off" was written.
	Off bool
	raw string
	ok  bool
}

func (d *Days) UnmarshalText(b []byte) error {
	s := string(b)
	*d = Days{raw: s}
	if s == "off" {
		d.Off, d.ok = true, true
		return nil
	}
	n, err := strconv.Atoi(s)
	d.N, d.ok = n, err == nil && n > 0
	return nil
}

func days(n int) Days { return Days{N: n, raw: strconv.Itoa(n), ok: true} }

func (d Days) problem(setting string) (Problem, bool) {
	if d.ok {
		return Problem{}, false
	}
	return problem(setting, `%s must be a number of days, 1 or more, or "off", got %s`, setting, d.raw), true
}

// Glob is a glob pattern, as path.Match reads it.
type Glob struct {
	Pattern string
	err     error
}

func (g *Glob) UnmarshalText(b []byte) error {
	*g = glob(string(b))
	return nil
}

func glob(p string) Glob {
	_, err := path.Match(p, "")
	return Glob{Pattern: p, err: err}
}

// globs are Globs for a default.
func globs(ps ...string) []Glob {
	out := make([]Glob, len(ps))
	for i, p := range ps {
		out[i] = glob(p)
	}
	return out
}

// Patterns are the patterns of gs.
func Patterns(gs []Glob) []string {
	out := make([]string, len(gs))
	for i, g := range gs {
		out[i] = g.Pattern
	}
	return out
}

// globProblems says what is wrong with each pattern of setting.
func globProblems(setting string, gs []Glob) Problems {
	var ps Problems
	for i, g := range gs {
		if g.err != nil {
			ps = append(ps, Problem{Key: strings.Split(setting, "."), Index: i,
				Message: fmt.Sprintf("%s[%d] is not a glob pattern: %q", setting, i, g.Pattern)})
		}
	}
	return ps
}

// Regexp is a regular expression, compiled as it is read.
type Regexp struct {
	Re  *regexp.Regexp
	raw string
	err error
}

func (r *Regexp) UnmarshalText(b []byte) error {
	*r = Regexp{raw: string(b)}
	r.Re, r.err = regexp.Compile(r.raw)
	return nil
}

// String is the expression as written.
func (r Regexp) String() string { return r.raw }

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range strings.TrimPrefix(s, "-") {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
