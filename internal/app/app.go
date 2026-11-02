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
  config     check, show or edit the config file (gh triage config help)
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
	case "config":
		return configCommand(flags.Args()[1:], *configPath, *debug, stdout, stderr)
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

	cfg, km, warns, err := setup(*configPath, *debug, level, log)
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
		chats := state.OpenChats(state.DefaultDir(), github.DefaultHost())
		if err := gc(context.Background(), ws, chats, gh, stdout); err != nil {
			log.Error("gc", "err", err.Error())
			fmt.Fprintf(stderr, "gh-triage: gc: %v\n", err)
			return 1
		}
		return 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	viewer, err := gh.Viewer(ctx)
	if err != nil {
		log.Error("viewer", "err", err.Error())
		fmt.Fprintf(stderr, "gh-triage: could not read who you are on GitHub: %v\n", err)
		return 1
	}
	p := assemble(cfg, warns, env{gh: gh, host: github.DefaultHost(), viewer: viewer, stateDir: state.DefaultDir(),
		borrow: claude.GhqRoots(ctx), noCache: *noCache}, log)
	start := time.Now()
	err = tui.Run(ctx, source{gh}, p.rn, p.cl, km, log, p.set)
	// Stop the batches and wait for them to stop writing the record before
	// the lock goes, so another gh-triage never shares it.
	cancel()
	p.rn.Wait()
	if p.store != nil {
		p.store.Close()
	}
	if p.lock != nil {
		p.lock.close()
	}
	log.Info("exit", logging.KeyDurationMS, time.Since(start).Milliseconds(), "err", errString(err))
	if err != nil {
		fmt.Fprintf(stderr, "gh-triage: %v (run %s)\n", err, run)
		return 1
	}
	return 0
}

// env is what assemble takes besides the config: the GitHub client, the
// host it talks to and the account logged in there, the directory the
// state is kept in, the directories a worktree may borrow clones from, and
// whether the lists last read are left alone.
type env struct {
	gh       *github.Client
	host     string
	viewer   string
	stateDir string
	borrow   []string
	noCache  bool
}

// parts are what the TUI runs with. store and lock, when set, are closed
// once it has quit and rn has stopped.
type parts struct {
	set   tui.Settings
	rn    *runner.Runner
	cl    asker
	store *state.Store
	lock  *pendingLock
}

// assemble puts together what the TUI runs with from the config and e. It
// reads and writes only under e.stateDir and the cache directory; nothing
// is sent to GitHub.
func assemble(cfg config.File, warns []string, e env, log *slog.Logger) parts {
	set := settings(cfg)
	set.ConfigWarnings = warns
	set.Viewer = e.viewer
	set.Host = e.host
	set.Chats = chatStore{state.OpenChats(e.stateDir, e.host)}
	if !e.noCache {
		set.Lists = openLists(cfg, e.host, e.viewer, log)
	}
	layout := state.OpenLayout(e.stateDir)
	set.Layout = layout
	if h, err := layout.PreviewHeight(); err != nil {
		log.Warn("could not read the pane's height last set", "err", err.Error())
	} else {
		set.PaneHeight = h
	}
	// One gh-triage at a time keeps the record of merges it waits for;
	// another one runs without waiting merges.
	store, err := state.Open(e.stateDir)
	if err != nil {
		log.Warn("could not open the record of pending merges", "err", err.Error())
		set.StoreProblem = "could not open " + e.stateDir + ": " + err.Error()
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
	rn := runner.New(runnerGitHub{e.gh}, log, runner.Options{Settings: set.Plan, Viewer: e.viewer, Store: rstore, CanWait: rstore != nil,
		ChecksTimeout: cfg.Merge.ChecksTimeout.D, BlockedTimeout: cfg.Merge.BlockedTimeout.D})
	var lock *pendingLock
	if store == nil && err == nil {
		// Taken over once the other gh-triage quits.
		lock = &pendingLock{dir: e.stateDir, rn: rn}
		set.Lock = lock
	}
	cl := asker{
		Claude: claude.New(claude.Options{Bin: cfg.ClaudeCommand(), Model: cfg.Claude.Model, Tools: cfg.Claude.AllowedTools, Language: cfg.Claude.Language, Instructions: cfg.Claude.Instructions, Log: log}),
		ws:     &claude.Workspace{Dir: cfg.CacheDir(), Borrow: e.borrow, Log: log},
	}
	return parts{set: set, rn: rn, cl: cl, store: store, lock: lock}
}

// openLists opens the lists last read by this account on this host. The
// viewer is the one just read, never the cached one: the lists of another
// account are not shown, and nothing is done in its name.
func openLists(cfg config.File, host, viewer string, log *slog.Logger) tui.Lists {
	path := filepath.Join(cfg.CacheDir(), "lists.json")
	c, err := listcache.Open(path, host, viewer)
	if err != nil {
		log.Warn("could not read the lists last read", "err", err.Error())
	}
	return c
}

// prReader reads pull requests by key.
type prReader interface {
	PullRequests(ctx context.Context, keys []model.PRKey) (map[model.PRKey]model.PR, error)
}

// gc removes the worktrees and the conversations kept of pull requests
// that are no longer open. A pull request GitHub does not return (deleted,
// or out of reach) is kept, and so is one whose conversation a gh-triage
// is working on: its lock is taken first, without waiting.
func gc(ctx context.Context, ws *claude.Workspace, chats *state.Chats, gh prReader, out io.Writer) error {
	wts, err := ws.Worktrees()
	if err != nil {
		return err
	}
	var keys []model.PRKey
	hasWorktree := map[model.PRKey]bool{}
	for _, w := range wts {
		keys = append(keys, w.Key)
		hasWorktree[w.Key] = true
	}
	ck, err := chats.Keys()
	if err != nil {
		return err
	}
	for _, k := range ck {
		if !hasWorktree[k] {
			keys = append(keys, k)
		}
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
			unlock, held, who, err := chats.Lock(k)
			if err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
			if !held {
				fmt.Fprintf(out, "kept %s: a gh-triage (%s) is talking with Claude about it\n", k, cmp.Or(who, "another process"))
				continue
			}
			err = func() error {
				defer unlock()
				if hasWorktree[k] {
					if err := ws.Remove(ctx, k); err != nil {
						return err
					}
				}
				return chats.Remove(k)
			}()
			if err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
			fmt.Fprintf(out, "removed %s (%s)\n", k, strings.ToLower(string(p.State)))
			removed++
		}
	}
	fmt.Fprintf(out, "%d removed, %d kept, in %s\n", removed, len(keys)-removed, ws.Dir)
	return nil
}

// chatStore is the conversations kept as the TUI asks for them.
type chatStore struct{ *state.Chats }

func (c chatStore) Load(k model.PRKey) (tui.ChatRecord, bool, error) {
	r, ok, err := c.Chats.Load(k)
	if !ok || err != nil {
		return tui.ChatRecord{}, ok, err
	}
	out := tui.ChatRecord{Key: r.Key, Session: r.Session, Started: r.Started, Dir: r.Dir, Head: r.Head, Model: r.Model, Cost: r.Cost, Rev: r.Rev}
	for _, t := range r.Turns {
		var steps []claude.Step
		for _, s := range t.Steps {
			steps = append(steps, claude.Step{Tool: s.Tool, Arg: s.Arg})
		}
		out.Turns = append(out.Turns, tui.ChatTurn{You: t.You, Text: t.Text, Steps: steps, Err: t.Err, Head: t.Head, At: t.At, Cost: t.Cost})
	}
	return out, true, nil
}

func (c chatStore) Save(r tui.ChatRecord) error {
	in := state.ChatRecord{Key: r.Key, Session: r.Session, Started: r.Started, Dir: r.Dir, Head: r.Head, Model: r.Model, Cost: r.Cost, Rev: r.Rev}
	for _, t := range r.Turns {
		var steps []state.ChatStep
		for _, s := range t.Steps {
			steps = append(steps, state.ChatStep{Tool: s.Tool, Arg: s.Arg})
		}
		in.Turns = append(in.Turns, state.ChatTurn{You: t.You, Text: t.Text, Steps: steps, Err: t.Err, Head: t.Head, At: t.At, Cost: t.Cost})
	}
	return c.Chats.Save(in)
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

// Triage runs in the cache too: a triage is about no repository, and its
// session is resumed from there.
func (a asker) Triage(ctx context.Context, r claude.Request) (claude.Answer, error) {
	if err := os.MkdirAll(a.ws.Dir, 0o755); err != nil {
		return claude.Answer{}, err
	}
	r.Dir = a.ws.Dir
	return a.Claude.Triage(ctx, r)
}

// runnerGitHub is the GitHub client as the runner asks for it.
type runnerGitHub struct{ *github.Client }

func (g runnerGitHub) EnableAutoMerge(ctx context.Context, key model.PRKey, id, head string, method model.MergeMethod) error {
	err := g.Client.EnableAutoMerge(ctx, key, id, head, method)
	if errors.Is(err, github.ErrCleanStatus) {
		return fmt.Errorf("%w: %w", runner.ErrCleanStatus, err)
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
		MarkDone: cfg.Review.MarkDone, HomeOwners: cfg.List.HomeOwners, Columns: cfg.List.Columns, Menu: cfg.Menu.Actions, PreviewHeight: cfg.Preview.Height,
		DefaultPaneHeight: config.DefaultPreviewHeight,
		Collapse:          config.Patterns(cfg.Diff.Collapse), ThemeDark: cfg.Theme.Dark, ThemeLight: cfg.Theme.Light, Spinner: cfg.Theme.Spinner, Shimmer: cfg.Theme.Shimmer, Wave: cfg.Theme.Wave, Editor: editor(cfg.Review.Editor, os.Getenv),
		Refresh: cfg.RefreshInterval(), ReleaseNotes: cfg.Claude.ReleaseNotesCheck, Language: cfg.Claude.Language,
		OpenOnDiff: cfg.Preview.Tab == "diff", PlainDescription: cfg.Description.Style == "plain",
		Plan: plan.Settings{AutoMerge: cfg.Merge.AutoMerge, UpdateBranch: cfg.Merge.UpdateBranch,
			ApprovalChecks: config.Patterns(cfg.Checks.Approval), AttentionPaths: config.Patterns(cfg.Bulk.AttentionPaths)},
	}
	if !cfg.List.StaleDays.Off {
		set.StaleDays = cfg.List.StaleDays.N
	}
	for _, s := range cfg.Select {
		set.Rules = append(set.Rules, plan.Rule{Name: s.Name, Authors: s.Authors, Packages: s.Packages, Title: s.Title, MaxFiles: s.MaxFiles, MaxLines: s.MaxLines,
			UpdatedWithinDays: s.UpdatedWithinDays, CreatedWithinDays: s.CreatedWithinDays,
			Bot: s.Bot, ChecksPassing: s.Checks == "passing", ApprovedByMe: s.ApprovedByMe})
	}
	for _, m := range cfg.Merge.Methods {
		set.Plan.Methods = append(set.Plan.Methods, model.MergeMethod(strings.ToUpper(m)))
	}
	for _, t := range cfg.Tabs {
		tt := tui.Tab{Name: t.Name, Inbox: t.Inbox(), Query: t.Query, DraftsLast: t.Sort == "drafts_last", Limit: t.Limit, ShowSettled: t.ShowSettled}
		for _, g := range t.Groups {
			tt.Groups = append(tt.Groups, plan.Group(g))
		}
		for _, g := range t.Folded {
			tt.Folded = append(tt.Folded, plan.Group(g))
		}
		set.Tabs = append(set.Tabs, tt)
	}
	return set
}

// editor is the command that edits a review: review.editor, then $VISUAL, then
// $EDITOR. It may carry arguments, as "code --wait".
func editor(set string, getenv func(string) string) string {
	return cmp.Or(strings.TrimSpace(set), getenv("VISUAL"), getenv("EDITOR"))
}

// setup writes the config file on first run, then loads it. The warnings
// come back one line each, with where they are, for the TUI to show; they
// are logged too.
func setup(path string, debug bool, level *slog.LevelVar, log *slog.Logger) (config.File, keymap.Map, []string, error) {
	if err := config.WriteTemplate(path, KeysTemplate()); err == nil {
		log.Info("wrote the config file", "config", path)
	} else if !errors.Is(err, fs.ErrExist) {
		log.Warn("could not write the config file", "config", path, "err", err.Error())
	}
	cfg, km, warns, err := load(path, debug, level)
	if err != nil {
		return config.File{}, keymap.Map{}, nil, err
	}
	lines := config.Lines(path, warns)
	for _, l := range lines {
		log.Warn("config", "warning", l)
	}
	return cfg, km, lines, nil
}

// load reads the config file, sets the log level, and builds the keymap.
// A mistake anywhere in the file is reported where it is, all at once,
// with the warnings; with none, the warnings come back.
func load(path string, debug bool, level *slog.LevelVar) (config.File, keymap.Map, config.Problems, error) {
	cfg, warns, err := config.Load(path)
	if err != nil {
		return config.File{}, keymap.Map{}, nil, err
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
			return config.File{}, keymap.Map{}, nil, fmt.Errorf("%s: %w", EnvLogLevel, err)
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
		return config.File{}, keymap.Map{}, nil, config.Report(path, append(ps, warns...))
	}
	return cfg, km, warns, nil
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
	keymap.ScopePopup:   "The menu of enter, and the popups that pick from a list",
	keymap.ScopeConfirm: "A confirm popup",
	keymap.ScopeTriage:  "The list, while Claude's plan is shown",
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
