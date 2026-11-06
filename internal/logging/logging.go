// Package logging makes the logger gh-desk writes its log file with: JSON
// lines through log/slog, to a file under the state directory, never to the
// terminal the TUI draws on.
//
// The logger is made once, in the app package, and passed down; packages add
// what they know with With (an operation, a batch, a pull request) and never
// keep a logger of their own in a package variable.
package logging

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Attribute keys every package uses, so a log line can be searched by them.
const (
	KeyRun          = "run"
	KeyVersion      = "version"
	KeyOp           = "op"
	KeyBatch        = "batch"
	KeyPR           = "pr"
	KeyStep         = "step"
	KeyAttempt      = "attempt"
	KeyGHRequestID  = "gh_request_id"
	KeyGQLCost      = "gql_cost"
	KeyGQLRemaining = "gql_remaining"
	KeyDurationMS   = "duration_ms"
	KeyClaude       = "claude_session"
	// KeyJobs names what a wait waits on, KeyEnded what ended since the
	// last line and how, KeyStarted what began.
	KeyJobs    = "jobs"
	KeyEnded   = "ended"
	KeyStarted = "started"
	// KeyMergeState is GitHub's mergeStateStatus of a pull request, and
	// KeyWas what it was before.
	KeyMergeState = "merge_state"
	KeyWas        = "was"
	// KeyRule names the [[auto_approve]] rule that asked for an approval,
	// and KeyHead the head it judged.
	KeyRule = "rule"
	KeyHead = "head"
)

// Rotation defaults: the file is moved aside when it is over MaxSize at
// startup, and Keep old files are kept.
const (
	DefaultMaxSize = 10 << 20
	DefaultKeep    = 3
)

// DefaultPath is where the log file goes: $XDG_STATE_HOME/gh-desk/logs,
// or ~/.local/state/gh-desk/logs.
func DefaultPath() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "gh-desk", "logs", "gh-desk.log")
}

// ParseLevel reads a level as written in the config file or the
// environment: debug, info, warn or error.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return slog.LevelInfo, fmt.Errorf("%q is not a log level: debug, info, warn or error", s)
}

// Options say where and what to log.
type Options struct {
	// Path is the log file; DefaultPath when empty.
	Path string
	// Level can be a *slog.LevelVar, to change it after the file is open.
	Level slog.Leveler
	// MaxSize and Keep control rotation at startup; the defaults when zero.
	MaxSize int64
	Keep    int
	// NoRotate leaves the file as it is: for a process that writes to the
	// log beside the one that rotates it, such as gh desk mcp beside the
	// TUI, so that two never move the file aside at once.
	NoRotate bool
	// Records, when set, keeps what is written to the file in memory too,
	// at the same level.
	Records *Records
}

// Open rotates the log file if it has grown past MaxSize, opens it for
// appending, and returns a logger writing JSON lines to it. Closing the
// returned closer closes the file.
func Open(o Options) (*slog.Logger, io.Closer, error) {
	if o.Path == "" {
		o.Path = DefaultPath()
	}
	if o.MaxSize <= 0 {
		o.MaxSize = DefaultMaxSize
	}
	if o.Keep <= 0 {
		o.Keep = DefaultKeep
	}
	if err := os.MkdirAll(filepath.Dir(o.Path), 0o755); err != nil {
		return nil, nil, err
	}
	if !o.NoRotate {
		if err := rotate(o.Path, o.MaxSize, o.Keep); err != nil {
			return nil, nil, err
		}
	}
	f, err := os.OpenFile(o.Path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, err
	}
	if o.Records != nil {
		return slog.New(slog.NewMultiHandler(jsonHandler(f, o.Level), o.Records.Handler(o.Level))), f, nil
	}
	return New(f, o.Level), f, nil
}

// New returns a logger writing JSON lines to w at level and above. Tests
// pass a buffer.
func New(w io.Writer, level slog.Leveler) *slog.Logger {
	return slog.New(jsonHandler(w, level))
}

// Memory returns a logger that keeps its records in r and writes no file,
// for gh-desk run where there is none, such as the playground.
func Memory(r *Records, level slog.Leveler) *slog.Logger {
	return slog.New(r.Handler(level))
}

func jsonHandler(w io.Writer, level slog.Leveler) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
}

// Discard is a logger that writes nothing, for code run without one.
func Discard() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// rotate moves path to path.1, path.1 to path.2 and so on, dropping the
// oldest beyond keep, when path is larger than max.
func rotate(path string, max int64, keep int) error {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() <= max {
		return nil
	}
	old := func(n int) string { return fmt.Sprintf("%s.%d", path, n) }
	if err := os.Remove(old(keep)); err != nil && !os.IsNotExist(err) {
		return err
	}
	for n := keep - 1; n >= 1; n-- {
		if err := os.Rename(old(n), old(n+1)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.Rename(path, old(1))
}

// NewID returns a short random ID, for a run, an operation or a batch.
func NewID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b[:])
}
