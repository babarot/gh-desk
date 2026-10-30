// Package app puts gh-triage together: it reads the flags and the config
// file, opens the log, builds the keymap, and starts the TUI. It is the only
// package that knows every other one; the rest receive what they need from
// here.
package app

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/babarot/gh-triage/internal/claude"
	"github.com/babarot/gh-triage/internal/config"
	"github.com/babarot/gh-triage/internal/github"
	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/listcache"
	"github.com/babarot/gh-triage/internal/logging"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/plan"
	"github.com/babarot/gh-triage/internal/runner"
	"github.com/babarot/gh-triage/internal/state"
	"github.com/babarot/gh-triage/internal/tui"
)

// Version is set at build time.
var Version = "dev"

// EnvLogLevel overrides [log] level in the config file.
const EnvLogLevel = "GH_TRIAGE_LOG_LEVEL"

const usage = `Usage: gh triage [flags] [command]

Commands:
  gc         remove the worktrees of pull requests no longer open
  log        print where the log file is
  version    print the version

Flags:
`

// Run runs gh-triage with args (without the program name) and returns the
// exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("gh-triage", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", config.FilePath(), "the config file")
	debug := flags.Bool("debug", false, "log at debug level")
	noCache := flags.Bool("no-cache", false, "neither show nor keep the lists last read")
	flags.Usage = func() {
		fmt.Fprint(stderr, usage)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	switch flags.Arg(0) {
	case "", "gc":
	case "log":
		fmt.Fprintln(stdout, logging.DefaultPath())
		return 0
	case "version":
		fmt.Fprintln(stdout, Version)
		return 0
	default:
		fmt.Fprintf(stderr, "gh-triage: unknown command %q\n", flags.Arg(0))
		flags.Usage()
		return 2
	}

	level := new(slog.LevelVar)
	log, closeLog, err := logging.Open(logging.Options{Level: level})
	if err != nil {
		fmt.Fprintf(stderr, "gh-triage: open the log file: %v\n", err)
		return 1
	}
	defer closeLog.Close()
	run := logging.NewID()
	log = log.With(logging.KeyRun, run)

	cfg, km, err := setup(*configPath, *debug, level, log)
	if err != nil {
		log.Error("setup failed", "err", err.Error())
		fmt.Fprintln(stderr, err)
		return 1
	}
	log.Info("start", logging.KeyVersion, Version, "config", *configPath, "log", logging.DefaultPath(),
		"log_level", level.Level().String(), "refresh", cfg.RefreshInterval().String())

	gh, err := github.New(log, github.Options{})
	if err != nil {
		log.Error("github client", "err", err.Error())
		fmt.Fprintf(stderr, "gh-triage: %v (is gh logged in? try gh auth login)\n", err)
		return 1
	}
	if flags.Arg(0) == "gc" {
		ws := &claude.Workspace{Dir: cfg.CacheDir(), Log: log}
		if err := gc(context.Background(), ws, gh, stdout); err != nil {
			log.Error("gc", "err", err.Error())
			fmt.Fprintf(stderr, "gh-triage: gc: %v\n", err)
			return 1
		}
		return 0
	}
	set := settings(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	viewer, err := gh.Viewer(ctx)
	if err != nil {
		log.Error("viewer", "err", err.Error())
		fmt.Fprintf(stderr, "gh-triage: could not read who you are on GitHub: %v\n", err)
		return 1
	}
	set.Viewer = viewer
	if !*noCache {
		set.Lists = openLists(cfg, viewer, log)
	}
	// One gh-triage at a time keeps the record of merges it waits for;
	// another one runs without waiting merges.
	store, err := state.Open(state.DefaultDir())
	if err != nil {
		log.Warn("could not open the record of pending merges", "err", err.Error())
		set.StoreProblem = "could not open " + state.DefaultDir() + ": " + err.Error()
	}
	var rstore runner.Store
	if store != nil {
		rstore = store
		set.CanWait = true
		set.Resume = store.Pending()
		log.Info("pending merges", "count", len(set.Resume))
	} else {
		log.Info("another gh-triage holds the record of pending merges")
	}
	rn := runner.New(runnerGitHub{gh}, log, runner.Options{Settings: set.Plan, Viewer: viewer, Store: rstore, CanWait: rstore != nil,
		ChecksTimeout: config.Duration(cfg.Merge.ChecksTimeout), BlockedTimeout: config.Duration(cfg.Merge.BlockedTimeout),
		WatchTimeout: config.Duration(cfg.Merge.WatchTimeout)})
	cl := asker{
		Claude: claude.New(claude.Options{Model: cfg.Claude.Model, Tools: cfg.Claude.AllowedTools, Instructions: cfg.Claude.Instructions, Log: log}),
		ws:     &claude.Workspace{Dir: cfg.CacheDir(), Borrow: claude.GhqRoots(ctx), Log: log},
	}
	start := time.Now()
	err = tui.Run(ctx, source{gh}, rn, cl, km, log, set)
	// Stop the batches and wait for them to stop writing the record before
	// the lock goes, so another gh-triage never shares it.
	cancel()
	rn.Wait()
	if store != nil {
		store.Close()
	}
	log.Info("exit", logging.KeyDurationMS, time.Since(start).Milliseconds(), "err", errString(err))
	if err != nil {
		fmt.Fprintf(stderr, "gh-triage: %v (run %s)\n", err, run)
		return 1
	}
	return 0
}

// openLists opens the lists last read by this account on this host. The
// viewer is the one just read, never the cached one: the lists of another
// account are not shown, and nothing is done in its name.
func openLists(cfg config.File, viewer string, log *slog.Logger) tui.Lists {
	path := filepath.Join(cfg.CacheDir(), "lists.json")
	c, err := listcache.Open(path, github.DefaultHost(), viewer)
	if err != nil {
		log.Warn("could not read the lists last read", "err", err.Error())
	}
	return c
}

// prReader reads pull requests by key.
type prReader interface {
	PullRequests(ctx context.Context, keys []model.PRKey) (map[model.PRKey]model.PR, error)
}

// gc removes the worktrees of pull requests that are no longer open. A
// pull request GitHub does not return (deleted, or out of reach) is kept.
func gc(ctx context.Context, ws *claude.Workspace, gh prReader, out io.Writer) error {
	wts, err := ws.Worktrees()
	if err != nil {
		return err
	}
	var keys []model.PRKey
	for _, w := range wts {
		keys = append(keys, w.Key)
	}
	removed := 0
	for chunk := range slices.Chunk(keys, 50) {
		prs, err := gh.PullRequests(ctx, chunk)
		if err != nil {
			return err
		}
		for _, k := range chunk {
			p, ok := prs[k]
			if !ok || p.State == model.StateOpen {
				continue
			}
			if err := ws.Remove(ctx, k); err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
			fmt.Fprintf(out, "removed %s (%s)\n", k, strings.ToLower(string(p.State)))
			removed++
		}
	}
	fmt.Fprintf(out, "%d removed, %d kept, in %s\n", removed, len(keys)-removed, ws.Dir)
	return nil
}

// source is the GitHub client as the TUI asks for it.
type source struct{ *github.Client }

func (s source) SearchPage(ctx context.Context, query string, first int, after string) ([]model.PR, int, string, []string, error) {
	pg, err := s.Client.SearchPage(ctx, query, first, after)
	return pg.PRs, pg.Total, pg.Next, pg.Warnings, err
}

// asker is Claude and its worktrees as the TUI asks for them.
type asker struct {
	*claude.Claude
	ws *claude.Workspace
}

func (a asker) Prepare(ctx context.Context, key model.PRKey, head string) (string, error) {
	return a.ws.Prepare(ctx, key, head)
}

// Once runs in the cache, away from any repository, so no project's
// CLAUDE.md is read.
func (a asker) Once(ctx context.Context, _ string, prompt, schema string) (claude.Answer, error) {
	if err := os.MkdirAll(a.ws.Dir, 0o755); err != nil {
		return claude.Answer{}, err
	}
	return a.Claude.Once(ctx, a.ws.Dir, prompt, schema)
}

// runnerGitHub is the GitHub client as the runner asks for it.
type runnerGitHub struct{ *github.Client }

func (g runnerGitHub) EnableAutoMerge(ctx context.Context, key model.PRKey, id, head string, method model.MergeMethod) error {
	err := g.Client.EnableAutoMerge(ctx, key, id, head, method)
	if errors.Is(err, github.ErrCleanStatus) {
		return fmt.Errorf("%w: %v", runner.ErrCleanStatus, err)
	}
	return err
}

func (g runnerGitHub) CommitInfo(ctx context.Context, key model.PRKey, oid string) (runner.Commit, error) {
	c, err := g.Client.CommitInfo(ctx, key, oid)
	return runner.Commit(c), err
}

// settings are the config values the TUI uses.
func settings(cfg config.File) tui.Settings {
	set := tui.Settings{
		InboxLimit: cfg.Inbox.Limit, HideClosed: cfg.Inbox.HideClosed, DoneAfterReview: cfg.Inbox.DoneAfterReview,
		HomeOwners: cfg.UI.HomeOwners, StaleDays: cfg.UI.StaleDays, PreviewHeight: cfg.Preview.Height,
		Collapse: cfg.Diff.Collapse, Theme: cfg.UI.Theme, Editor: cmp.Or(os.Getenv("VISUAL"), os.Getenv("EDITOR")),
		Refresh: cfg.RefreshInterval(), ReleaseNotes: cfg.Claude.ReleaseNotesCheck,
		Plan: plan.Settings{AutoMerge: cfg.Merge.AutoMerge, UpdateBranch: cfg.Merge.UpdateBranch,
			ApprovalChecks: cfg.UI.ApprovalChecks, AttentionPaths: cfg.Bulk.AttentionPaths},
	}
	for _, s := range cfg.Select {
		r := plan.Rule{Name: s.Name, Author: s.Author, Packages: s.Packages, MaxFiles: s.MaxFiles, Bot: s.Bot, ChecksPassing: s.Checks == "passing"}
		if s.Title != "" {
			r.Title = regexp.MustCompile(s.Title) // config.Load has checked it
		}
		set.Rules = append(set.Rules, r)
	}
	for _, m := range cfg.Merge.Methods {
		set.Plan.Methods = append(set.Plan.Methods, model.MergeMethod(strings.ToUpper(m)))
	}
	for _, s := range cfg.Sections {
		set.Sections = append(set.Sections, tui.Section{Name: s.Name, Query: s.Query, Grouped: s.Group == "requested",
			Folded: s.Folded, DraftsLast: s.Sort == "drafts_last", Limit: s.Limit})
	}
	return set
}

// setup writes the config file on first run, reads it, sets the log level,
// and builds the keymap. A mistake anywhere in the file is reported where
// it is, all at once.
func setup(path string, debug bool, level *slog.LevelVar, log *slog.Logger) (config.File, keymap.Map, error) {
	if err := config.WriteTemplate(path, KeysTemplate()); err == nil {
		log.Info("wrote the config file", "config", path)
	} else if !errors.Is(err, fs.ErrExist) {
		log.Warn("could not write the config file", "config", path, "err", err.Error())
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.File{}, keymap.Map{}, err
	}
	var ps config.Problems
	want := cfg.Log.Level
	if env := os.Getenv(EnvLogLevel); env != "" {
		want = env
	}
	if debug {
		want = "debug"
	}
	if l, err := logging.ParseLevel(want); err != nil {
		if want == cfg.Log.Level {
			ps = append(ps, config.SettingProblem("log.level", "log.level: %v", err))
		} else {
			return config.File{}, keymap.Map{}, fmt.Errorf("%s: %w", EnvLogLevel, err)
		}
	} else {
		level.Set(l)
	}
	for i, t := range cfg.Claude.AllowedTools {
		if !slices.Contains(claude.Tools, t) {
			ps = append(ps, config.Problem{Key: []string{"claude", "allowed_tools"}, Index: i,
				Message: fmt.Sprintf("claude.allowed_tools[%d] must be one of %q (Claude only reads), got %q", i, claude.Tools, t)})
		}
	}
	km, kps := applyKeys(cfg.Keys)
	ps = append(ps, kps...)
	if len(ps) > 0 {
		return config.File{}, keymap.Map{}, config.Report(path, ps)
	}
	return cfg, km, nil
}

// applyKeys gives the actions under [keys] their keys, turning what the
// keymap finds wrong into problems placed in the config file.
func applyKeys(set config.Keys) (keymap.Map, config.Problems) {
	var ps config.Problems
	keys := map[string][]string{}
	for name, kl := range set {
		if kl.Err != nil {
			ps = append(ps, config.Problem{Key: []string{"keys", name}, Index: -1, Message: fmt.Sprintf("keys.%s %v", name, kl.Err)})
			continue
		}
		keys[name] = kl.Keys
	}
	km, kps := keymap.Apply(keymap.Default(), keys)
	for _, p := range kps {
		ps = append(ps, config.Problem{Key: []string{"keys", p.Action}, Index: p.Index, AtKey: p.AtKey, Message: p.Message})
	}
	if len(ps) > 0 {
		return keymap.Map{}, ps
	}
	return km, nil
}

// KeysTemplate is the [keys] part of the config file written on first run:
// every action at its default keys, commented out, scope by scope.
func KeysTemplate() string {
	var b strings.Builder
	scope := keymap.Scope("")
	for _, d := range keymap.Defaults {
		if d.Scope != scope {
			if scope != "" {
				b.WriteString("#\n")
			}
			scope = d.Scope
			fmt.Fprintf(&b, "# %s:\n", scopeWords[scope])
		}
		fmt.Fprintf(&b, "# %s = %s\n", d.Action, keysValue(d.Keys))
	}
	return b.String()
}

var scopeWords = map[keymap.Scope]string{
	keymap.ScopeGlobal:  "Anywhere",
	keymap.ScopeNav:     "Moving, in every list and pane",
	keymap.ScopeList:    "The list and its preview pane",
	keymap.ScopePR:      "The pull request screen",
	keymap.ScopePopup:   "A confirm popup",
	keymap.ScopeChat:    "The Claude screen",
	keymap.ScopeCompose: "Writing a review",
	keymap.ScopeInput:   "Typing in the filter",
}

// keysValue writes keys as TOML: a string for one key, an array otherwise.
func keysValue(keys []string) string {
	if len(keys) == 1 {
		return fmt.Sprintf("%q", keys[0])
	}
	q := make([]string, len(keys))
	for i, k := range keys {
		q[i] = fmt.Sprintf("%q", k)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
