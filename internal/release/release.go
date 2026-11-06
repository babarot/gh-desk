// Package release tells whether a newer release of gh-desk is out, how
// the running install is updated, and keeps what was found between
// launches. It reads nothing from the network: internal/github does.
package release

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Page is where the releases are on the web.
const Page = "https://github.com/babarot/gh-desk/releases"

// CheckEvery is how long a reading of the releases holds before the next
// launch reads them again. A constant: once a day is often enough to hear
// of a release, and rarely enough that nobody would turn it down.
const CheckEvery = 24 * time.Hour

// Compare compares two versions written as three numbers, with or without
// a leading v: -1 when a is older than b, 0 when they are the same, 1 when
// a is newer.
func Compare(a, b string) (int, error) {
	x, err := parse(a)
	if err != nil {
		return 0, err
	}
	y, err := parse(b)
	if err != nil {
		return 0, err
	}
	for i := range x {
		if x[i] != y[i] {
			if x[i] < y[i] {
				return -1, nil
			}
			return 1, nil
		}
	}
	return 0, nil
}

// Newer reports whether a is a newer version than b. A version that does
// not parse is never newer, so a tag of another form offers nothing.
func Newer(a, b string) bool {
	c, err := Compare(a, b)
	return err == nil && c > 0
}

func parse(v string) ([3]int, error) {
	var n [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return n, fmt.Errorf("version %q is not three numbers", v)
	}
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil || x < 0 {
			return n, fmt.Errorf("version %q is not three numbers", v)
		}
		n[i] = x
	}
	return n, nil
}

// Method is how the running gh-desk was installed, which says how it is
// updated.
type Method string

const (
	// Nix is a release build from the Nix store.
	Nix Method = "nix"
	// Extension is a release build gh installed as an extension.
	Extension Method = "gh-extension"
	// Download is any other release build.
	Download Method = "download"
)

// MethodOf decides how a release build was installed from where its
// executable really is (symbolic links followed) and gh's extensions
// directory. A gh extension that links into the Nix store is Nix: Nix,
// not gh, updates it.
func MethodOf(exe, extensions string) Method {
	switch {
	case strings.HasPrefix(exe, "/nix/store/"):
		return Nix
	case extensions != "" && strings.HasPrefix(exe, filepath.Clean(extensions)+string(filepath.Separator)):
		return Extension
	}
	return Download
}

// How says how an install of m is updated, as told next to a newer
// release.
func (m Method) How() string {
	switch m {
	case Nix:
		return "update it with Nix"
	case Extension:
		return "gh extension upgrade desk"
	}
	return Page
}

// Record is what is kept between launches: when the releases were last
// read and the newest found, the newest one told of, and the version that
// ran last.
type Record struct {
	Checked time.Time `json:"checked"`
	Latest  string    `json:"latest,omitempty"`
	Told    string    `json:"told,omitempty"`
	LastRun string    `json:"last_run,omitempty"`
}

// Due reports whether the releases are read again at now.
func (r Record) Due(now time.Time) bool {
	return r.Checked.IsZero() || now.Sub(r.Checked) >= CheckEvery || now.Before(r.Checked)
}

// Store keeps the record in release.json under the state directory. It
// takes no lock: two gh-desk writing it at once leave the one written last,
// which at worst tells of a release once more.
type Store struct {
	path string
}

// Open returns the store in dir.
func Open(dir string) *Store {
	return &Store{path: filepath.Join(dir, "release.json")}
}

// Load reads the record; with none kept, it is empty.
func (s *Store) Load() (Record, error) {
	var r Record
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return Record{}, fmt.Errorf("%s: %w", s.path, err)
	}
	return r, nil
}

// Save replaces the record whole, through a temporary file of its own.
func (s *Store) Save(r Record) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "release-*.json")
	if err != nil {
		return err
	}
	_, werr := f.Write(b)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(f.Name())
		return err
	}
	if err := os.Rename(f.Name(), s.path); err != nil {
		os.Remove(f.Name())
		return err
	}
	return nil
}

var (
	notesComment = regexp.MustCompile(`(?m)^<!--.*-->[ \t]*\n?`)
	notesHeading = regexp.MustCompile(`(?m)^## What's Changed[ \t]*\n?`)
	notesFull    = regexp.MustCompile(`(?m)^\*\*Full Changelog\*\*.*$`)
	notesBy      = regexp.MustCompile(` by @[\w-]+(?:\[bot\])? in https://github\.com/[\w.-]+/[\w.-]+/pull/(\d+)`)
)

// Notes is a release's notes as GitHub generated them, shortened to what a
// reader of gh-desk wants: the comment, the "What's Changed" heading and
// the link to the full changelog go, and "by @user in <pull request URL>"
// becomes the pull request's number.
func Notes(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = notesComment.ReplaceAllString(body, "")
	body = notesHeading.ReplaceAllString(body, "")
	body = notesFull.ReplaceAllString(body, "")
	body = notesBy.ReplaceAllString(body, " (#$1)")
	return strings.TrimSpace(body)
}
