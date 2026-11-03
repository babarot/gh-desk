// Package listcache keeps the lists gh-desk last read in a file, so the
// next launch shows them at once while it reads them again.
//
// The file belongs to one account on one host: one written for another is
// not used, and is replaced whole on the next write. Each list is kept by
// its search, so a tab whose search changed starts empty, and a write
// replaces only the lists it names, leaving those of a gh-desk started
// with another config.
package listcache

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

// version is the file's format; a file of another one is not used.
const version = 1

// List is a tab's pull requests as last read.
type List struct {
	Query string     `json:"query"`
	Total int        `json:"total"`
	Read  time.Time  `json:"read"`
	PRs   []model.PR `json:"prs"`
}

// Inbox is the notifications as last read, with their pull requests.
type Inbox struct {
	Read   time.Time            `json:"read"`
	Notifs []model.Notification `json:"notifs"`
	PRs    []model.PR           `json:"prs"`
}

type file struct {
	Version int    `json:"version"`
	Host    string `json:"host"`
	Viewer  string `json:"viewer"`
	Lists   []List `json:"lists"`
	Inbox   *Inbox `json:"inbox,omitempty"`
}

// Cache is the file for one account. Its methods may be called from
// several goroutines.
type Cache struct {
	path, host, viewer string
	mu                 sync.Mutex
	got                file
}

// Open reads the file at path for viewer on host. A missing file, one of
// another format, host or viewer, gives an empty cache; one that cannot be
// read or parsed gives an empty cache and the error, for the log.
func Open(path, host, viewer string) (*Cache, error) {
	c := &Cache{path: path, host: host, viewer: viewer}
	f, err := read(path)
	if err != nil {
		return c, err
	}
	if c.own(f) {
		c.got = f
	}
	return c, nil
}

func (c *Cache) own(f file) bool {
	return f.Version == version && f.Host == c.host && f.Viewer == c.viewer
}

func read(path string) (file, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return file{}, nil
	}
	if err != nil {
		return file{}, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return file{}, err
	}
	return f, nil
}

// List is the list last read for query.
func (c *Cache) List(query string) (List, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, l := range c.got.Lists {
		if l.Query == query {
			return l, true
		}
	}
	return List{}, false
}

// Inbox is the inbox last read.
func (c *Cache) Inbox() (Inbox, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.got.Inbox == nil {
		return Inbox{}, false
	}
	return *c.got.Inbox, true
}

// PutList records l, replacing the list of its query.
func (c *Cache) PutList(l List) error {
	return c.update(func(f *file) {
		for i := range f.Lists {
			if f.Lists[i].Query == l.Query {
				f.Lists[i] = l
				return
			}
		}
		f.Lists = append(f.Lists, l)
	})
}

// PutInbox records the inbox.
func (c *Cache) PutInbox(in Inbox) error {
	return c.update(func(f *file) { f.Inbox = &in })
}

// update reads the file again, changes it and writes it through a
// temporary file of its own, so two gh-desk writing at once never rename
// each other's half-written file. One of another account is replaced, not
// changed, so no list of that account is left on disk.
func (c *Cache) update(change func(*file)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	f, err := read(c.path)
	if err != nil || !c.own(f) {
		f = file{Version: version, Host: c.host, Viewer: c.viewer}
	}
	change(&f)
	c.got = f
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".lists-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	// The lists name pull requests of private repositories; only the
	// user reads them.
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), c.path)
}
