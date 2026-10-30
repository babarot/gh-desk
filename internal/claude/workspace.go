package claude

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/babarot/gh-triage/internal/logging"
	"github.com/babarot/gh-triage/internal/model"
)

// Workspace keeps a clone of each repository in the cache and a worktree
// of each pull request at its head, for Claude to read. Nothing is ever
// written to the user's own clones: a worktree added from one would leave
// its entry in that clone, and moving the clone would break it.
type Workspace struct {
	// Dir is the cache: repos/<owner>/<repo> holds the clones, and
	// worktrees/<owner>/<repo>/<number> the worktrees.
	Dir string
	// Borrow are the roots of the user's clones laid out as
	// <root>/github.com/<owner>/<repo> (ghq's roots). A clone found there
	// lends its objects to a new clone, which copies them so it does not
	// depend on that clone afterwards.
	Borrow []string
	Log    *slog.Logger
	// Remote is the URL a repository is cloned from; nil is GitHub.
	Remote func(owner, repo string) string

	mu    sync.Mutex
	repos map[string]*sync.Mutex
}

// repoLock serializes git commands on one clone.
func (w *Workspace) repoLock(repo string) *sync.Mutex {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.repos == nil {
		w.repos = map[string]*sync.Mutex{}
	}
	if w.repos[repo] == nil {
		w.repos[repo] = &sync.Mutex{}
	}
	return w.repos[repo]
}

func (w *Workspace) clonePath(k model.PRKey) string {
	return filepath.Join(w.Dir, "repos", k.Owner, k.Repo)
}

// WorktreePath is where the worktree of k is.
func (w *Workspace) WorktreePath(k model.PRKey) string {
	return filepath.Join(w.Dir, "worktrees", k.Owner, k.Repo, strconv.Itoa(k.Number))
}

// Prepare makes the worktree of k at head, cloning the repository and
// fetching the pull request first when needed, and returns its path.
func (w *Workspace) Prepare(ctx context.Context, k model.PRKey, head string) (string, error) {
	if head == "" {
		return "", errors.New("the head of the pull request is not known")
	}
	l := w.repoLock(k.Repository())
	l.Lock()
	defer l.Unlock()
	start := time.Now()
	log := w.Log.With(logging.KeyPR, k.String())
	repo := w.clonePath(k)
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		if err := w.clone(ctx, k, repo); err != nil {
			return "", err
		}
		log.Info("cloned", logging.KeyDurationMS, time.Since(start).Milliseconds())
	}
	if !w.has(ctx, repo, head) {
		ref := fmt.Sprintf("+refs/pull/%d/head:refs/gh-triage/pull/%d", k.Number, k.Number)
		if _, err := git(ctx, repo, "fetch", "--quiet", "origin", ref); err != nil {
			return "", err
		}
		if !w.has(ctx, repo, head) {
			// The head moved since it was read; fetch the commit seen.
			if _, err := git(ctx, repo, "fetch", "--quiet", "origin", head); err != nil {
				return "", err
			}
		}
	}
	wt := w.WorktreePath(k)
	if _, err := os.Stat(wt); err == nil {
		// Claude only reads, so the worktree has nothing to keep.
		if _, err := git(ctx, wt, "checkout", "--quiet", "--force", "--detach", head); err != nil {
			return "", err
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(wt), 0o755); err != nil {
			return "", err
		}
		// A worktree removed by hand leaves its entry behind.
		_, _ = git(ctx, repo, "worktree", "prune")
		if _, err := git(ctx, repo, "worktree", "add", "--quiet", "--detach", wt, head); err != nil {
			return "", err
		}
	}
	log.Debug("worktree ready", "head", head, logging.KeyDurationMS, time.Since(start).Milliseconds())
	return wt, nil
}

func (w *Workspace) has(ctx context.Context, repo, oid string) bool {
	_, err := git(ctx, repo, "cat-file", "-e", oid+"^{commit}")
	return err == nil
}

// clone makes a blobless clone of k's repository, borrowing the objects of
// a clone of the user's when there is one.
func (w *Workspace) clone(ctx context.Context, k model.PRKey, dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	tmp := dir + ".partial"
	_ = os.RemoveAll(tmp)
	args := []string{"clone", "--quiet", "--filter=blob:none", "--no-checkout"}
	for _, root := range w.Borrow {
		mine := filepath.Join(root, "github.com", k.Owner, k.Repo)
		if _, err := os.Stat(filepath.Join(mine, ".git")); err == nil {
			args = append(args, "--reference-if-able", mine, "--dissociate")
			break
		}
	}
	url := fmt.Sprintf("https://github.com/%s/%s.git", k.Owner, k.Repo)
	if w.Remote != nil {
		url = w.Remote(k.Owner, k.Repo)
	}
	args = append(args, url, tmp)
	if _, err := git(ctx, "", args...); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	return os.Rename(tmp, dir)
}

// Worktree is a pull request's worktree in the cache.
type Worktree struct {
	Key  model.PRKey
	Path string
}

// Worktrees lists the worktrees in the cache.
func (w *Workspace) Worktrees() ([]Worktree, error) {
	var out []Worktree
	root := filepath.Join(w.Dir, "worktrees")
	owners, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, o := range owners {
		repos, _ := os.ReadDir(filepath.Join(root, o.Name()))
		for _, r := range repos {
			nums, _ := os.ReadDir(filepath.Join(root, o.Name(), r.Name()))
			for _, n := range nums {
				num, err := strconv.Atoi(n.Name())
				if err != nil || !n.IsDir() {
					continue
				}
				k := model.PRKey{Owner: o.Name(), Repo: r.Name(), Number: num}
				out = append(out, Worktree{Key: k, Path: w.WorktreePath(k)})
			}
		}
	}
	return out, nil
}

// Remove removes k's worktree and the ref fetched for it.
func (w *Workspace) Remove(ctx context.Context, k model.PRKey) error {
	l := w.repoLock(k.Repository())
	l.Lock()
	defer l.Unlock()
	repo := w.clonePath(k)
	if _, err := git(ctx, repo, "worktree", "remove", "--force", w.WorktreePath(k)); err != nil {
		if err := os.RemoveAll(w.WorktreePath(k)); err != nil {
			return err
		}
		_, _ = git(ctx, repo, "worktree", "prune")
	}
	_, _ = git(ctx, repo, "update-ref", "-d", fmt.Sprintf("refs/gh-triage/pull/%d", k.Number))
	return nil
}

// git runs git in dir. It asks gh for credentials, so a private repository
// is read with the account gh is signed in with, whatever git is set up
// to use. It runs no hooks: with a relative core.hooksPath in the user's
// config, a checkout would run hooks the pull request adds.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"-c", "core.hooksPath=/dev/null", "-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = msg[i+1:]
		}
		return "", fmt.Errorf("git %s: %s", args[0], cmp.Or(msg, err.Error()))
	}
	return out.String(), nil
}

// GhqRoots are the roots ghq keeps clones under (git config ghq.root, or
// ~/ghq), for Workspace.Borrow.
func GhqRoots(ctx context.Context) []string {
	out, _ := git(ctx, "", "config", "--get-all", "ghq.root")
	var roots []string
	for _, r := range strings.Split(strings.TrimSpace(out), "\n") {
		if r == "" {
			continue
		}
		if r == "~" || strings.HasPrefix(r, "~/") {
			if h, err := os.UserHomeDir(); err == nil {
				r = filepath.Join(h, r[1:])
			}
		}
		roots = append(roots, r)
	}
	if len(roots) == 0 {
		if h, err := os.UserHomeDir(); err == nil {
			roots = []string{filepath.Join(h, "ghq")}
		}
	}
	return roots
}
