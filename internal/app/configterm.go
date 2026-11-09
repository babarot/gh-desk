package app

import (
	"cmp"
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"

	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/listcache"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui"
	"github.com/babarot/gh-desk/internal/tui/keymap"
	"github.com/babarot/gh-desk/internal/version"
)

// The size of gh-desk on the settings page: as wide as the list the page
// drew again.
const termCols, termRows = 132, 30

// errPreviewReads is what the preview says of a read the TUI asks for: it
// reads nothing, and an Offline TUI does not ask.
var errPreviewReads = errors.New("the preview reads nothing from GitHub")

// errPreviewWrites is what the preview says of a write: it does none.
var errPreviewWrites = errors.New("the preview writes nothing to GitHub")

// previewLists are the lists last read, for gh-desk on the settings page to
// show; it keeps nothing. every, when set, is the list of every tab, as the
// made-up pull requests are.
type previewLists struct {
	lists map[string]listcache.List
	inbox *listcache.Inbox
	every *listcache.List
}

func savedLists(s listcache.Saved) *previewLists {
	l := &previewLists{lists: map[string]listcache.List{}, inbox: s.Inbox}
	for _, x := range s.Lists {
		l.lists[x.Query] = x
	}
	return l
}

func (l *previewLists) List(query string) (listcache.List, bool) {
	if l.every != nil {
		e := *l.every
		e.Query = query
		return e, true
	}
	x, ok := l.lists[query]
	return x, ok
}

func (l *previewLists) Inbox() (listcache.Inbox, bool) {
	if l.inbox == nil {
		return listcache.Inbox{}, false
	}
	return *l.inbox, true
}

func (l *previewLists) PutList(listcache.List) error   { return nil }
func (l *previewLists) PutInbox(listcache.Inbox) error { return nil }

// prs are the pull requests of the lists, by node ID.
func (l *previewLists) prs() map[string]model.PR {
	out := map[string]model.PR{}
	add := func(ps []model.PR) {
		for _, p := range ps {
			if p.ID != "" {
				out[p.ID] = p
			}
		}
	}
	if l.every != nil {
		add(l.every.PRs)
	}
	for _, x := range l.lists {
		add(x.PRs)
	}
	if l.inbox != nil {
		add(l.inbox.PRs)
	}
	return out
}

// previewSource is GitHub as the preview knows it: the lists last read. It
// answers what a merge confirm asks from them, with what they do not hold
// taken as the most a merge may do, and refuses everything else.
type previewSource struct {
	byID map[string]model.PR
}

func (s previewSource) Viewer(context.Context) (string, error) { return "", errPreviewReads }
func (s previewSource) SearchPage(context.Context, string, int, string) ([]model.PR, int, string, []string, error) {
	return nil, 0, "", nil, errPreviewReads
}
func (s previewSource) Notifications(context.Context, int) ([]model.Notification, error) {
	return nil, errPreviewReads
}
func (s previewSource) PullRequests(context.Context, []model.PRKey) (map[model.PRKey]model.PR, error) {
	return nil, errPreviewReads
}
func (s previewSource) Detail(context.Context, model.PRKey) (model.Detail, error) {
	return model.Detail{}, errPreviewReads
}
func (s previewSource) Diff(context.Context, model.PR) ([]byte, error) { return nil, errPreviewReads }
func (s previewSource) Comment(context.Context, string) (model.Comment, error) {
	return model.Comment{}, errPreviewReads
}
func (s previewSource) Bodies(context.Context, []string) (map[string]string, error) {
	return nil, errPreviewReads
}
func (s previewSource) BasedOn(context.Context, []model.Branch) (map[model.Branch][]int, error) {
	return nil, errPreviewReads
}
func (s previewSource) Heavy(context.Context, []string) (map[string]model.Heavy, error) {
	return nil, errPreviewReads
}
func (s previewSource) Review(context.Context, model.PR, model.ReviewEvent, string) error {
	return errPreviewWrites
}
func (s previewSource) MarkDone(context.Context, model.Notification) error { return errPreviewWrites }
func (s previewSource) Browse(string) error                                { return errPreviewWrites }

// MergeInfo is what the lists say of each pull request. The repository's
// merge methods are not in them: the preview takes every method and
// auto-merge as allowed, and a pull request whose merge state the lists
// leave out as clean, and the confirm says so.
func (s previewSource) MergeInfo(_ context.Context, ids []string, _ bool) (map[string]model.MergeInfo, error) {
	out := map[string]model.MergeInfo{}
	for _, id := range ids {
		p, ok := s.byID[id]
		if !ok {
			continue
		}
		mi := model.MergeInfo{Key: p.Key, ID: p.ID, State: p.State, Draft: p.Draft, HeadOID: p.HeadOID,
			MergeState: cmp.Or(p.Merge, model.MergeClean), Decision: p.Decision, Rollup: p.Checks, Reviews: p.Reviews,
			Repo: model.RepoMerge{Methods: []model.MergeMethod{model.MethodMerge, model.MethodSquash, model.MethodRebase}, AutoMerge: true}}
		if mi.State == "" {
			mi.State = model.StateOpen
		}
		// Checks still running are waited for, as the real confirm says.
		if p.Checks == model.CheckPending || p.Checks == model.CheckExpected {
			mi.ChecksPartial = true
		}
		out[id] = mi
	}
	return out, nil
}

// previewRunner runs nothing: each pull request ends at once, skipped.
type previewRunner struct{}

func (previewRunner) refuse(items []runner.Item) (string, <-chan runner.Event) {
	ch := make(chan runner.Event, len(items))
	for _, it := range items {
		ch <- runner.Event{Key: it.PR.Key, Result: runner.Skipped, Text: errPreviewWrites.Error()}
	}
	close(ch)
	return "preview", ch
}

func (r previewRunner) Start(_ context.Context, _ runner.Kind, items []runner.Item) (string, <-chan runner.Event) {
	return r.refuse(items)
}
func (r previewRunner) StartReview(_ context.Context, _ runner.ReviewText, items []runner.Item) (string, <-chan runner.Event) {
	return r.refuse(items)
}
func (r previewRunner) StartClose(_ context.Context, _ runner.CloseText, items []runner.Item) (string, <-chan runner.Event) {
	return r.refuse(items)
}
func (r previewRunner) StartComment(_ context.Context, _ string, items []runner.Item) (string, <-chan runner.Event) {
	return r.refuse(items)
}
func (r previewRunner) Resume(_ context.Context, ps []model.Pending) (string, <-chan runner.Event) {
	items := make([]runner.Item, len(ps))
	for i, p := range ps {
		items[i] = runner.Item{PR: model.PR{ID: p.ID, Key: p.Key}}
	}
	return r.refuse(items)
}

// keyInput is what is typed in the page's terminal, waiting to be read. A
// write never waits; a read waits for a key, and ends once it is closed.
type keyInput struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    []byte
	closed bool
}

func newKeyInput() *keyInput {
	in := &keyInput{}
	in.cond = sync.NewCond(&in.mu)
	return in
}

func (in *keyInput) Write(p []byte) {
	in.mu.Lock()
	in.buf = append(in.buf, p...)
	in.mu.Unlock()
	in.cond.Broadcast()
}

func (in *keyInput) Close() {
	in.mu.Lock()
	in.closed = true
	in.mu.Unlock()
	in.cond.Broadcast()
}

func (in *keyInput) Read(p []byte) (int, error) {
	in.mu.Lock()
	defer in.mu.Unlock()
	for len(in.buf) == 0 && !in.closed {
		in.cond.Wait()
	}
	if len(in.buf) == 0 {
		return 0, io.EOF
	}
	n := copy(p, in.buf)
	in.buf = in.buf[n:]
	return n, nil
}

// previewTerminal is gh-desk on the settings page (ADR-109): the lists last
// read, read only, following the file as it is edited.
type previewTerminal struct {
	lists  *previewLists
	viewer string
	log    *slog.Logger

	mu   sync.Mutex
	set  tui.Settings
	keys keymap.Map
	run  *termRun
}

// termRun is one run of gh-desk on the page.
type termRun struct {
	in     *keyInput
	cancel context.CancelFunc
	done   chan struct{}
	// next are the settings to give it, the last applied winning.
	next chan termSettings
}

// termSettings are settings with their keys.
type termSettings struct {
	set  tui.Settings
	keys keymap.Map
}

func newPreviewTerminal(shown *preview, cfg config.File, km keymap.Map) *previewTerminal {
	t := &previewTerminal{lists: shown.shown, viewer: shown.viewer, log: slog.New(slog.DiscardHandler)}
	t.set, t.keys = t.settings(cfg), km
	return t
}

// settings are the file's, made safe for the preview: it reads nothing,
// starts no editor and keeps nothing.
func (t *previewTerminal) settings(cfg config.File) tui.Settings {
	set := Settings(cfg)
	set.Viewer = t.viewer
	set.Lists = t.lists
	set.Version = version.String()
	set.Offline = true
	// The merge confirm takes a merge that waits as the gh-desk that holds
	// the record would; nothing runs, so nothing waits.
	set.CanWait = true
	set.Editor = ""
	return set
}

func (t *previewTerminal) Run(ctx context.Context, out io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	t.mu.Lock()
	if was := t.run; was != nil {
		was.cancel()
		t.mu.Unlock()
		<-was.done
		t.mu.Lock()
	}
	r := &termRun{in: newKeyInput(), cancel: cancel, done: make(chan struct{}), next: make(chan termSettings, 1)}
	t.run = r
	set, km := t.set, t.keys
	t.mu.Unlock()
	defer func() {
		r.in.Close()
		t.mu.Lock()
		if t.run == r {
			t.run = nil
		}
		t.mu.Unlock()
		close(r.done)
	}()

	p := tui.NewProgram(ctx, r.in, out, termCols, termRows, previewSource{byID: t.lists.prs()}, previewRunner{}, nil, km, t.log, set)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case a := <-r.next:
				p.Apply(a.set, a.keys)
			}
		}
	}()
	return p.Run()
}

func (t *previewTerminal) Input(p []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.run != nil {
		t.run.in.Write(p)
	}
}

func (t *previewTerminal) Apply(src []byte) error {
	cfg, _, err := config.Parse("config.toml", src)
	if err != nil {
		return err
	}
	km, ps := applyKeys(cfg.Keys)
	if len(ps) > 0 {
		return config.Report("config.toml", ps)
	}
	set := t.settings(cfg)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.set, t.keys = set, km
	if r := t.run; r != nil {
		// Settings not taken yet are dropped for these.
		select {
		case <-r.next:
		default:
		}
		r.next <- termSettings{set, km}
	}
	return nil
}
