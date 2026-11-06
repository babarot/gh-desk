// Package app puts gh-desk together: it reads the flags and the config
// file, opens the log, builds the keymap, and starts the TUI. It is the only
// package that knows every other one; the rest receive what they need from
// here.
package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/babarot/gh-desk/internal/claude"
	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/github"
	"github.com/babarot/gh-desk/internal/listcache"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/mcpserver"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/state"
	"github.com/babarot/gh-desk/internal/tui"
	"github.com/babarot/gh-desk/internal/tui/chat"
	"github.com/babarot/gh-desk/internal/tui/keymap"
	"github.com/babarot/gh-desk/internal/version"
)

// EnvLogLevel overrides [log] level in the config file.
const EnvLogLevel = "GH_DESK_LOG_LEVEL"

// Run runs gh-desk with args (without the program name) and returns the
// exit code: 1 when a command failed, 2 when it was called wrongly.
func Run(args []string, stdout, stderr io.Writer) int {
	root := newRootCommand(stdout, stderr)
	root.SetArgs(args)
	cmd, err := root.ExecuteC()
	if err == nil {
		return 0
	}
	if code, ok := errors.AsType[exitCode](err); ok {
		return int(code)
	}
	fmt.Fprintf(stderr, "gh-desk: %v\n", err)
	fmt.Fprint(stderr, cmd.UsageString())
	return 2
}

// start opens the log, reads the config and reaches GitHub, then runs the
// TUI, or with gcOnly gh desk gc, and returns the exit code.
func start(o *options, gcOnly bool, stdout, stderr io.Writer) int {
	level := new(slog.LevelVar)
	// What goes to the log file is kept in memory too, for the TUI to show.
	recs := logging.NewRecords(0, nil)
	log, closeLog, err := logging.Open(logging.Options{Level: level, Records: recs})
	if err != nil {
		fmt.Fprintf(stderr, "gh-desk: open the log file: %v\n", err)
		return 1
	}
	defer closeLog.Close()
	run := logging.NewID()
	log = log.With(logging.KeyRun, run)

	cfg, km, warns, err := setup(o.config, o.debug, level, log)
	if err != nil {
		log.Error("setup failed", "err", err.Error())
		fmt.Fprintln(stderr, err)
		return 1
	}
	log.Info("start", logging.KeyVersion, version.String(), "config", o.config, "log", logging.DefaultPath(),
		"log_level", level.Level().String(), "refresh", cfg.RefreshInterval().String())

	gh, err := github.New(log, github.Options{})
	if err != nil {
		log.Error("github client", "err", err.Error())
		fmt.Fprintf(stderr, "gh-desk: %v (is gh logged in? try gh auth login)\n", err)
		return 1
	}
	if gcOnly {
		ws := &claude.Workspace{Dir: cfg.CacheDir(), Log: log}
		chats := state.OpenChats(state.DefaultDir(), github.DefaultHost())
		if err := gc(context.Background(), ws, chats, runs(cfg), gh, stdout); err != nil {
			log.Error("gc", "err", err.Error())
			fmt.Fprintf(stderr, "gh-desk: gc: %v\n", err)
			return 1
		}
		return 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	viewer, err := gh.Viewer(ctx)
	if err != nil {
		log.Error("viewer", "err", err.Error())
		fmt.Fprintf(stderr, "gh-desk: could not read who you are on GitHub: %v\n", err)
		return 1
	}
	exe, err := os.Executable()
	if err != nil {
		log.Warn("gh-desk's own MCP server is left out: where this program is is unknown", "err", err.Error())
	}
	p := assemble(cfg, warns, env{gh: gh, host: github.DefaultHost(), viewer: viewer, stateDir: state.DefaultDir(),
		borrow: claude.GhqRoots(ctx), noCache: o.noCache, config: absPath(o.config), exe: exe, debug: o.debug,
		release: version.IsRelease(), notify: os.Getenv("GH_NO_EXTENSION_UPDATE_NOTIFIER") == "", extensions: github.ExtensionsDir()}, log)
	p.set.LogRecords = recs
	start := time.Now()
	err = tui.Run(ctx, source{gh}, p.rn, p.cl, km, log, p.set)
	// Stop the batches and wait for them to stop writing the record before
	// the lock goes, so another gh-desk never shares it.
	cancel()
	p.rn.Wait()
	if p.store != nil {
		p.store.Close()
	}
	if p.lock != nil {
		p.lock.close()
	}
	p.release()
	log.Info("exit", logging.KeyDurationMS, time.Since(start).Milliseconds(), "err", errString(err))
	if err != nil {
		fmt.Fprintf(stderr, "gh-desk: %v (run %s)\n", err, run)
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
	// config is the config file read, and exe this program, which gh-desk's
	// own MCP server is started as, logging at debug with debug; with exe
	// empty it is not given.
	config string
	exe    string
	debug  bool
	// release is whether this build is a release, and notify whether it
	// may tell of its releases: release.check and gh's
	// GH_NO_EXTENSION_UPDATE_NOTIFIER both leave it on. extensions is
	// where gh installs its extensions.
	release, notify bool
	extensions      string
}

// parts are what the TUI runs with. store and lock, when set, are closed
// once it has quit and rn has stopped.
type parts struct {
	set   tui.Settings
	rn    *runner.Runner
	cl    asker
	store *state.Store
	lock  *pendingLock
	// release removes the files of this run given to Claude.
	release func()
}

// assemble puts together what the TUI runs with from the config and e. It
// reads and writes only under e.stateDir and the cache directory; nothing
// is sent to GitHub.
func assemble(cfg config.File, warns []string, e env, log *slog.Logger) parts {
	set := Settings(cfg)
	set.ConfigWarnings = warns
	set.Viewer = e.viewer
	set.Version = version.String()
	set.Host = e.host
	set.Chats = chatStore{state.OpenChats(e.stateDir, e.host)}
	if !e.noCache {
		set.Lists = openLists(cfg, e.host, e.viewer, log)
	}
	set.Release = releaseOf(cfg, e, log)
	layout := state.OpenLayout(e.stateDir)
	set.Layout = layout
	if h, err := layout.PreviewHeight(); err != nil {
		log.Warn("could not read the pane's height last set", "err", err.Error())
	} else {
		set.PaneHeight = h
	}
	// One gh-desk at a time keeps the record of merges it waits for;
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
		log.Info("another gh-desk holds the record of pending merges")
	}
	rn := runner.New(runnerGitHub{e.gh}, log, runner.Options{Settings: set.Plan, Viewer: e.viewer, Store: rstore, CanWait: rstore != nil,
		WaitTimeout: cfg.Checks.WaitTimeout.D, BlockedTimeout: cfg.Merge.BlockedTimeout.D,
		Excluded: excludedBy(e.gh, set.Exclude, e.viewer, set.Plan, time.Now)})
	var lock *pendingLock
	if store == nil && err == nil {
		// Taken over once the other gh-desk quits.
		lock = &pendingLock{dir: e.stateDir, rn: rn}
		set.Lock = lock
	}
	opt := claude.Options{Bin: cfg.ClaudeCommand(), Model: cfg.Claude.Model, Tools: cfg.Claude.AllowedTools, Language: cfg.Claude.Language, Instructions: cfg.Claude.Instructions, Log: log}
	release := func() {}
	if servers := mcpServers(cfg, e.config, e.exe, e.debug); len(servers) > 0 || len(cfg.Claude.Plugins) > 0 {
		var err error
		release, err = conversationFiles(cfg, servers, &opt)
		if err != nil {
			log.Warn("could not write the MCP servers and skills for Claude", "err", err.Error())
			set.ConfigWarnings = append(set.ConfigWarnings, "claude.mcp and claude.plugins are left out: "+err.Error())
			opt.MCPConfig, opt.MCPTools, opt.PluginDir = "", nil, ""
			release = func() {}
		} else {
			_, opt.GitHub = servers[mcpserver.Name]
			opt.Label = mcpserver.Label
			log.Debug("given to Claude", "servers", slices.Sorted(maps.Keys(servers)), "tools", opt.MCPTools,
				"plugins", cfg.PluginDirs(), "plugin", opt.PluginDir)
		}
	}
	cl := asker{
		Claude: claude.New(opt),
		ws:     &claude.Workspace{Dir: cfg.CacheDir(), Borrow: e.borrow, Log: log},
	}
	return parts{set: set, rn: rn, cl: cl, store: store, lock: lock, release: release}
}

// runs are the directories of the running gh-desk, in the cache.
func runs(cfg config.File) state.Runs { return state.Runs{Dir: filepath.Join(cfg.CacheDir(), "runs")} }

// absPath is p made absolute, as the MCP server runs in another directory;
// p as it is when that fails.
func absPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

// mcpServers are the MCP servers of the conversation: the user's, and
// gh-desk's own when it is on (ADR-080), started as this program's mcp
// command with the config file read and --debug, so that it logs as this
// run does.
func mcpServers(cfg config.File, configPath, exe string, debug bool) map[string]map[string]any {
	out := maps.Clone(cfg.Claude.MCP.Servers)
	if !cfg.BuiltinMCP() || exe == "" {
		return out
	}
	if out == nil {
		out = map[string]map[string]any{}
	}
	tools := make([]any, len(mcpserver.Tools))
	for i, t := range mcpserver.Tools {
		tools[i] = t
	}
	args := []any{"mcp", "--config", configPath}
	if debug {
		args = append(args, "--debug")
	}
	out[mcpserver.Name] = map[string]any{"command": exe, "args": args, "tools": tools}
	return out
}

// conversationFiles writes what the conversation with Claude is given
// beside its flags, the MCP servers and the skills, in a directory of this
// run, and sets them in opt. release removes them.
func conversationFiles(cfg config.File, servers map[string]map[string]any, opt *claude.Options) (release func(), err error) {
	dir, release, err := runs(cfg).New()
	if err != nil {
		return nil, err
	}
	if len(servers) > 0 {
		opt.MCPConfig, opt.MCPTools, err = claude.WriteMCPConfig(dir, cfg.CacheDir(), servers)
		if err != nil {
			release()
			return nil, err
		}
	}
	// Load checked the plugins; they are read again, as they may have
	// changed since, and the copy is checked once more.
	if skills, ps := claude.PluginSkills(cfg.PluginDirs(), skillTools(cfg)); len(ps) > 0 {
		release()
		return nil, errors.New(ps[0].Message)
	} else if len(skills) > 0 {
		opt.PluginDir, err = claude.WritePlugin(dir, skills, skillTools(cfg))
		if err != nil {
			release()
			return nil, err
		}
	}
	return release, nil
}

// skillTools are the tools a skill may grant in allowed-tools: the
// built-in ones Claude is given, Skill, and the tools of the MCP servers.
func skillTools(cfg config.File) []string {
	tools := append(slices.Clone(cfg.Claude.AllowedTools), "Skill")
	for _, name := range slices.Sorted(maps.Keys(cfg.Claude.MCP.Servers)) {
		for _, t := range config.MCPTools(cfg.Claude.MCP.Servers[name]) {
			tools = append(tools, "mcp__"+name+"__"+t)
		}
	}
	if cfg.BuiltinMCP() {
		for _, t := range mcpserver.Tools {
			tools = append(tools, "mcp__"+mcpserver.Name+"__"+t)
		}
	}
	return tools
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
// or out of reach) is kept, and so is one whose conversation a gh-desk
// is working on: its lock is taken first, without waiting.
func gc(ctx context.Context, ws *claude.Workspace, chats *state.Chats, rs state.Runs, gh prReader, out io.Writer) error {
	// The files of a gh-desk that did not exit cleanly.
	if n, err := rs.Sweep(); err != nil {
		return err
	} else if n > 0 {
		fmt.Fprintf(out, "removed the files of %d gh-desk no longer running\n", n)
	}
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
				fmt.Fprintf(out, "kept %s: a gh-desk (%s) is talking with Claude about it\n", k, cmp.Or(who, "another process"))
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

func (c chatStore) Load(k model.PRKey) (chat.Record, bool, error) {
	r, ok, err := c.Chats.Load(k)
	if !ok || err != nil {
		return chat.Record{}, ok, err
	}
	out := chat.Record{Key: r.Key, Session: r.Session, Started: r.Started, Dir: r.Dir, Head: r.Head, Model: r.Model, Cost: r.Cost, Rev: r.Rev}
	for _, t := range r.Turns {
		var steps []claude.Step
		for _, s := range t.Steps {
			steps = append(steps, claude.Step{Tool: s.Tool, Arg: s.Arg})
		}
		out.Turns = append(out.Turns, chat.Turn{You: t.You, Text: t.Text, Steps: steps, Err: t.Err, Head: t.Head, At: t.At, Cost: t.Cost})
	}
	return out, true, nil
}

func (c chatStore) Save(r chat.Record) error {
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

// Settings are the config values the TUI uses. The demo takes them too.
func Settings(cfg config.File) tui.Settings {
	set := tui.Settings{
		MarkDone: cfg.Review.MarkDone, HomeOwners: cfg.List.HomeOwners, Columns: cfg.List.Columns, Menu: cfg.Menu.Actions, PreviewHeight: cfg.Preview.Height,
		DefaultPaneHeight: config.DefaultPreviewHeight,
		Collapse:          config.Patterns(cfg.Diff.Collapse), ThemeDark: cfg.Theme.Dark, ThemeLight: cfg.Theme.Light, Spinner: cfg.Theme.Spinner, Shimmer: cfg.Theme.Shimmer, Editor: editor(cfg.Review.Editor, os.Getenv),
		Refresh: cfg.RefreshInterval(), Bell: cfg.List.Bell, ReleaseNotes: cfg.Claude.ReleaseNotesCheck, Language: cfg.Claude.Language,
		OpenOnDiff: cfg.Preview.Tab == "diff", PlainDescription: cfg.Description.Style == "plain",
		Plan: plan.Settings{AutoMerge: cfg.Merge.AutoMerge, UpdateBranch: cfg.Merge.UpdateBranch,
			ApprovalChecks: config.Patterns(cfg.Checks.Approval), AttentionPaths: config.Patterns(cfg.Bulk.AttentionPaths)},
	}
	if !cfg.List.StaleDays.Off {
		set.StaleDays = cfg.List.StaleDays.N
	}
	for _, s := range cfg.Select {
		set.Rules = append(set.Rules, rule(s.Name, s.Conditions))
	}
	for _, e := range cfg.Exclude {
		set.Exclude = append(set.Exclude, rule(e.Name, e.Conditions))
	}
	for _, a := range cfg.AutoApprove {
		q := a.Query
		if q == "" {
			q = autoQuery(a.Repos)
		}
		set.AutoApprove = append(set.AutoApprove, tui.AutoApprove{Rule: rule(a.Name, a.Conditions), Query: q, DryRun: a.DryRun})
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
	if err := config.WriteTemplate(path); err == nil {
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
	_, sps := claude.PluginSkills(cfg.PluginDirs(), skillTools(cfg))
	for _, p := range sps {
		ps = append(ps, config.Problem{Key: []string{"claude", "plugins"}, Index: p.Plugin,
			Message: fmt.Sprintf("claude.plugins[%d]: %s", p.Plugin, p.Message)})
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

// KeysManual is the [keys] part of the manual: every action at its default
// keys, commented out, scope by scope.
func KeysManual() string {
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
	keymap.ScopeLog:     "The log viewer",
	keymap.ScopeChat:    "The Claude screen",
	keymap.ScopeCompose: "Writing a review",
	keymap.ScopeInput:   "Typing in the filter",
	keymap.ScopeHelp:    "The ? list of keys",
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

// rule is the rule named name with the conditions c, as the config file
// gives them.
func rule(name string, c config.Conditions) plan.Rule {
	return plan.Rule{Name: name, Repos: c.Repos, Authors: c.Authors, Packages: c.Packages, Title: c.Title,
		MaxFiles: c.MaxFiles, MaxLines: c.MaxLines, MinFiles: c.MinFiles, MinLines: c.MinLines,
		UpdatedWithinDays: c.UpdatedWithinDays, CreatedWithinDays: c.CreatedWithinDays,
		UpdatedBeforeDays: c.UpdatedBeforeDays, CreatedBeforeDays: c.CreatedBeforeDays,
		Bot: c.Bot, Mine: c.Mine, Draft: c.Draft, Checks: checks(c.Checks), Decisions: decisions(c.Decision),
		Labels: c.Labels, States: states(c.States), ApprovedByMe: c.ApprovedByMe, ReviewedByMe: c.ReviewedByMe, Requested: c.Requested, Reasons: c.Reasons}
}

// states are the states the config file names, as the model has them.
func states(ss []string) []model.PRState {
	var out []model.PRState
	for _, s := range ss {
		out = append(out, model.PRState(strings.ToUpper(s)))
	}
	return out
}

// checks are the states of checks the config file names.
func checks(ss []string) []plan.Checks {
	var out []plan.Checks
	for _, s := range ss {
		out = append(out, plan.Checks(s))
	}
	return out
}

// decisions are the review decisions the config file names, as the model
// has them; "none" is plan.NoDecision.
func decisions(ss []string) []model.ReviewDecision {
	var out []model.ReviewDecision
	for _, s := range ss {
		out = append(out, model.ReviewDecision(strings.ToUpper(s)))
	}
	return out
}
