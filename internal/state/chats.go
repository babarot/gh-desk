package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

// Chats keeps the conversations with Claude about pull requests, one file
// a pull request under chats/<host> in the state directory, so a
// conversation goes on after gh-desk quits and in another gh-desk.
// Each has a lock of its own, held while Claude Code works on its session,
// so two gh-desk never resume one session at once. See ADR-031.
type Chats struct {
	dir string
}

// ChatVersion is the format of a conversation's file; a file of another
// format is not read.
const ChatVersion = 1

// ChatRecord is a conversation as it is kept.
type ChatRecord struct {
	Version int         `json:"version"`
	Key     model.PRKey `json:"key"`
	// Session is Claude Code's session, Started once Claude Code has it.
	Session string `json:"session,omitempty"`
	Started bool   `json:"started,omitempty"`
	// Dir is the worktree the session started in; Claude Code resumes it
	// only from there.
	Dir string `json:"dir,omitempty"`
	// Head is the head of the pull request at the last question.
	Head  string  `json:"head,omitempty"`
	Model string  `json:"model,omitempty"`
	Cost  float64 `json:"cost,omitempty"`
	// Rev rises by one at each write and never goes down while the file
	// is there, so a gh-desk can tell that what it holds is older.
	Rev   int        `json:"rev"`
	Turns []ChatTurn `json:"turns"`
}

// ChatTurn is a question or an answer.
type ChatTurn struct {
	You   bool       `json:"you,omitempty"`
	Text  string     `json:"text"`
	Steps []ChatStep `json:"steps,omitempty"`
	Err   bool       `json:"err,omitempty"`
	// Head is the head of the pull request a question was asked at.
	Head string    `json:"head,omitempty"`
	At   time.Time `json:"at"`
	// Cost is what an answer cost, in US dollars.
	Cost float64 `json:"cost,omitempty"`
}

// ChatStep is a tool Claude used, and on what.
type ChatStep struct {
	Tool string `json:"tool"`
	Arg  string `json:"arg,omitempty"`
}

// OpenChats returns the conversations kept in dir for host.
func OpenChats(dir, host string) *Chats {
	if host == "" {
		host = "github.com"
	}
	return &Chats{dir: filepath.Join(dir, "chats", host)}
}

func (c *Chats) path(k model.PRKey) string {
	return filepath.Join(c.dir, k.Owner, k.Repo, strconv.Itoa(k.Number)+".json")
}

// Load reads the conversation about k; ok is false when there is none.
func (c *Chats) Load(k model.PRKey) (r ChatRecord, ok bool, err error) {
	p := c.path(k)
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return ChatRecord{}, false, nil
	}
	if err != nil {
		return ChatRecord{}, false, err
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return ChatRecord{}, false, fmt.Errorf("%s: %w", p, err)
	}
	if r.Version != ChatVersion {
		return ChatRecord{}, false, fmt.Errorf("%s: format %d is not read", p, r.Version)
	}
	return r, true, nil
}

// Save replaces the conversation about r.Key whole, through a temporary
// file of its own, so a crash leaves the old one or the new one.
func (c *Chats) Save(r ChatRecord) error {
	r.Version = ChatVersion
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	p := c.path(r.Key)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".chat-*.json")
	if err != nil {
		return err
	}
	_, werr := f.Write(b)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(f.Name())
		return err
	}
	if err := os.Rename(f.Name(), p); err != nil {
		os.Remove(f.Name())
		return err
	}
	return nil
}

// Remove forgets the conversation about k. Its lock file stays: a lock is
// on the file, and one made again would let a gh-desk that still has
// the old one open hold the lock beside one that opened the new one.
func (c *Chats) Remove(k model.PRKey) error {
	err := os.Remove(c.path(k))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Keys are the pull requests that have a conversation kept.
func (c *Chats) Keys() ([]model.PRKey, error) {
	var out []model.PRKey
	owners, err := os.ReadDir(c.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, o := range owners {
		if !o.IsDir() {
			continue
		}
		repos, _ := os.ReadDir(filepath.Join(c.dir, o.Name()))
		for _, r := range repos {
			if !r.IsDir() {
				continue
			}
			files, _ := os.ReadDir(filepath.Join(c.dir, o.Name(), r.Name()))
			for _, f := range files {
				name, ok := strings.CutSuffix(f.Name(), ".json")
				if !ok {
					continue
				}
				n, err := strconv.Atoi(name)
				if err != nil {
					continue
				}
				out = append(out, model.PRKey{Owner: o.Name(), Repo: r.Name(), Number: n})
			}
		}
	}
	return out, nil
}

// Lock takes the lock of the conversation about k without waiting. When
// another gh-desk holds it, held is false and who names that one as far
// as its lock file says. unlock releases the lock; the operating system
// releases it too when this process exits, however it exits.
func (c *Chats) Lock(k model.PRKey) (unlock func(), held bool, who string, err error) {
	p := c.path(k) + ".lock"
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return nil, false, "", err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, "", err
	}
	ok, err := tryLock(f)
	if err != nil || !ok {
		b, _ := os.ReadFile(p)
		f.Close()
		var h holder
		if json.Unmarshal(b, &h) == nil && h.PID > 0 {
			who = fmt.Sprintf("pid %d in %s", h.PID, h.Dir)
		}
		return nil, false, who, err
	}
	writeHolder(f)
	return func() { f.Close() }, true, "", nil
}
