package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Layout is how the screen was left, kept in layout.json under the state
// directory for the next launch. It takes no lock: two gh-desk writing it
// at once leave the one written last.
type Layout struct {
	path string
}

type layoutFile struct {
	PreviewHeight int `json:"preview_height,omitempty"`
}

// OpenLayout returns the layout kept in dir.
func OpenLayout(dir string) *Layout {
	return &Layout{path: filepath.Join(dir, "layout.json")}
}

// PreviewHeight is the preview pane's height last set, or 0 when none was.
func (l *Layout) PreviewHeight() (int, error) {
	b, err := os.ReadFile(l.path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var f layoutFile
	if err := json.Unmarshal(b, &f); err != nil {
		return 0, fmt.Errorf("%s: %w", l.path, err)
	}
	return max(f.PreviewHeight, 0), nil
}

// SavePreviewHeight keeps the preview pane's height. The file is replaced
// whole, through a temporary file of its own.
func (l *Layout) SavePreviewHeight(h int) error {
	b, err := json.MarshalIndent(layoutFile{PreviewHeight: h}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(l.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "layout-*.json")
	if err != nil {
		return err
	}
	_, werr := f.Write(b)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(f.Name())
		return err
	}
	if err := os.Rename(f.Name(), l.path); err != nil {
		os.Remove(f.Name())
		return err
	}
	return nil
}
