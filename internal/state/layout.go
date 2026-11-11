package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// Layout is how the screen was left, kept in layout.json under the state
// directory for the next launch. It takes no lock: two gh-desk writing it
// at once leave the one written last.
type Layout struct {
	path string
	// mu keeps one change of this gh-desk from undoing another, made at
	// the same time: the pane's height and the scope are written apart.
	mu sync.Mutex
}

type layoutFile struct {
	PreviewHeight int `json:"preview_height,omitempty"`
	// Scope is the value of list.scope chosen last, with the scope it is
	// of, so that a value of another scope is not taken for one.
	Scope *Scope `json:"scope,omitempty"`
}

// Scope is a value chosen of a list.scope: an owner, a repository or an
// author, by Kind.
type Scope struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// OpenLayout returns the layout kept in dir.
func OpenLayout(dir string) *Layout {
	return &Layout{path: filepath.Join(dir, "layout.json")}
}

// errBroken is a file that is there but does not parse.
var errBroken = errors.New("not a layout file")

// read is the file as kept, empty when there is none.
func (l *Layout) read() (layoutFile, error) {
	var f layoutFile
	b, err := os.ReadFile(l.path)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return layoutFile{}, fmt.Errorf("%s: %w: %w", l.path, errBroken, err)
	}
	return f, nil
}

// PreviewHeight is the preview pane's height last set, or 0 when none was.
func (l *Layout) PreviewHeight() (int, error) {
	f, err := l.read()
	return max(f.PreviewHeight, 0), err
}

// Scope is the value last chosen of the scope kind, or "" when none was,
// or one of another scope.
func (l *Layout) Scope(kind string) (string, error) {
	f, err := l.read()
	if err != nil || f.Scope == nil || f.Scope.Kind != kind {
		return "", err
	}
	return f.Scope.Value, nil
}

// SavePreviewHeight keeps the preview pane's height.
func (l *Layout) SavePreviewHeight(h int) error {
	return l.update(func(f *layoutFile) { f.PreviewHeight = h })
}

// SaveScope keeps value as the one chosen of the scope kind; "" for none.
func (l *Layout) SaveScope(kind, value string) error {
	return l.update(func(f *layoutFile) { f.Scope = &Scope{Kind: kind, Value: value} })
}

// update changes what change changes and keeps the rest as kept. A file
// that does not parse is written anew; one that cannot be read is left as
// it is, so that what it keeps is not lost. The file is replaced whole,
// through a temporary file of its own.
func (l *Layout) update(change func(*layoutFile)) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := l.read()
	if err != nil && !errors.Is(err, errBroken) {
		return err
	}
	change(&f)
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(l.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "layout-*.json")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), l.path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
