package playground

import (
	"fmt"
	"strings"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

// fixture is a pull request of the world as written here, its times as
// how long before the start.
type fixture struct {
	key, title, author string
	bot                bool
	// in are the demo's tabs whose search finds it: review, mine,
	// involved.
	in        []string
	requested []model.Reviewer
	reviews   []model.Review
	isDraft   bool
	branch    string
	created   time.Duration
	updated   time.Duration
	labels    []string
	checks    []check
	body      string
	comments  []comment
	diff      string
	behind    bool
	methods   []model.MergeMethod
	// answer is Claude's review, and followUp its answer to whatever is
	// asked after.
	answer, followUp string
	draft            draftReview
}

type comment struct {
	author string
	ago    time.Duration
	body   string
}

func (f fixture) pull(start time.Time) *pull {
	k, err := model.ParsePRKey(f.key)
	if err != nil {
		panic(err)
	}
	p := &pull{in: f.in, body: f.body, checks: f.checks, diff: f.diff, behind: f.behind, methods: f.methods,
		answer: f.answer, followUp: f.followUp, draft: f.draft}
	if p.methods == nil {
		p.methods = []model.MergeMethod{model.MethodSquash, model.MethodMerge}
	}
	p.PR = model.PR{
		ID: "PR_" + oid(f.key)[:12], Key: k, Title: f.title, URL: "https://github.com/" + k.Repository() + "/pull/" + fmt.Sprint(k.Number),
		State: model.StateOpen, Draft: f.isDraft, Author: f.author, AuthorIsBot: f.bot,
		Created: start.Add(-f.created), Updated: start.Add(-f.updated), HeadRef: f.branch, HeadRepo: k.Repository(), HeadOID: oid(f.key + " head"), BaseRef: "main",
		Requested: f.requested, Reviews: f.reviews, Labels: f.labels,
	}
	for _, c := range f.comments {
		p.comments = append(p.comments, model.Comment{Author: c.author, Body: c.body, Created: start.Add(-c.ago)})
	}
	p.files, p.Additions, p.Deletions = stat(f.diff)
	p.ChangedFiles = len(p.files)
	p.Checks = p.rollup()
	return p
}

// stat is the files a diff changes and its lines added and deleted.
func stat(diff string) (files []string, added, deleted int) {
	for l := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "diff --git "):
			_, b, _ := strings.Cut(l, " b/")
			files = append(files, b)
		case strings.HasPrefix(l, "+++ "), strings.HasPrefix(l, "--- "):
		case strings.HasPrefix(l, "+"):
			added++
		case strings.HasPrefix(l, "-"):
			deleted++
		}
	}
	return files, added, deleted
}

const (
	minute = time.Minute
	hour   = time.Hour
	day    = 24 * time.Hour
)

func you() []model.Reviewer             { return []model.Reviewer{{Login: Viewer}} }
func team(slug string) []model.Reviewer { return []model.Reviewer{{Login: slug, Team: true}} }

func ok(name string, required bool) check {
	return check{name: name, state: model.CheckSuccess, required: required}
}
func failed(name string, required bool) check {
	return check{name: name, state: model.CheckFailure, required: required}
}

// running is a check that passes after it has run for d.
func running(name string, required bool, d time.Duration) check {
	return check{name: name, state: model.CheckSuccess, required: required, after: d}
}

func goChecks() []check {
	return []check{ok("test (ubuntu-latest)", true), ok("test (macos-latest)", true), ok("lint", true), ok("CodeQL", false)}
}

func webChecks() []check {
	return []check{ok("build", true), ok("unit", true), ok("e2e (chromium)", false), ok("Vercel Preview", false)}
}

func fixtures() []fixture {
	return []fixture{
		{
			key: "acme/api#482", title: "Retry webhook deliveries with exponential backoff", author: "hana",
			in: []string{"review"}, requested: you(), branch: "hana/webhook-backoff",
			created: 26 * hour, updated: 35 * minute, labels: []string{"enhancement"},
			checks: goChecks(),
			body: `## What

Webhook deliveries that fail are retried with exponential backoff instead of being dropped after the first failure.

- Up to **5 attempts**, waiting 1s, 2s, 4s and 8s between them, with jitter
- A ` + "`429`" + ` or a ` + "`5xx`" + ` is retried; any other ` + "`4xx`" + ` is not
- Each attempt is recorded in ` + "`webhook_attempts`" + `, so the dashboard can show them

## Why

About 2% of deliveries fail on a timeout of the receiver and never arrive (#451). Most of them would succeed a few seconds later.

## How to test

` + "```sh\ngo test ./internal/webhook/...\n```" + `

| Status | Retried |
|---|---|
| 200 | no |
| 404 | no |
| 429 | yes |
| 503 | yes |
`,
			comments: []comment{
				{"lena", 20 * hour, "Should the jitter be configurable? Some receivers rate-limit per second."},
				{"hana", 19 * hour, "I kept it fixed at ±20% for now; happy to add a setting in a follow-up."},
			},
			diff: diffWebhook,
			answer: `I read the change in ` + "`internal/webhook/deliver.go`" + ` and its tests.

**Looks good overall.** The backoff is bounded, the jitter keeps retries from lining up, and non-retryable statuses stop at once.

Two things worth raising:

1. ` + "`deliver.go:58`" + ` — the request body is read once into ` + "`body`" + ` and reused, which is right, but ` + "`req.GetBody`" + ` is not set, so a redirect on a retry would send an empty body. Not blocking; redirects are rare for webhooks.
2. ` + "`deliver.go:71`" + ` — ` + "`time.Sleep`" + ` ignores ` + "`ctx`" + `. A shutdown waits up to 8s per delivery in flight. Use a ` + "`select`" + ` on ` + "`ctx.Done()`" + ` and a timer instead.

Neither blocks a merge, but (2) is cheap to fix now.`,
			followUp: `The retries stop at the fifth attempt (` + "`maxAttempts`" + ` in ` + "`deliver.go:14`" + `), and each attempt is recorded before it is made, so a crash in between leaves an attempt marked as started, not lost.

This is the demo: Claude's answers here are written in advance. Installed, gh-desk asks Claude Code in a worktree of the pull request's head.`,
			draft: draftReview{model.EventComment, "Nice change. Two notes:\n\n1. `deliver.go:71`: `time.Sleep` ignores `ctx`, so a shutdown waits for the backoff. A `select` on `ctx.Done()` and a timer would stop at once.\n2. `deliver.go:58`: `req.GetBody` is not set, so a redirect on a retry sends an empty body. Not blocking.\n"},
		},
		{
			key: "acme/web#1290", title: "Add a dark mode toggle to the settings page", author: "kenji",
			in: []string{"review"}, requested: you(), branch: "kenji/dark-mode",
			created: 3 * day, updated: 2 * hour, labels: []string{"ui"},
			checks:   []check{running("build", true, 25*time.Second), running("unit", true, 40*time.Second), ok("Vercel Preview", false)},
			behind:   true,
			body:     "Adds a toggle under **Settings → Appearance** with three choices: light, dark, and the system's.\n\nThe choice is kept in `localStorage` and applied before the first paint, so the page does not flash.\n\n![screenshot](https://example.com/dark-mode.png)\n",
			diff:     diffDarkMode,
			answer:   "The toggle and the early script in `index.html` look right; the theme is applied before React mounts, so there is no flash.\n\n**One issue:** `ThemeToggle.tsx:22` writes to `localStorage` inside a `try` but reads it outside one in `theme.ts:5`. Safari in private mode throws on the read, and the whole settings page fails to render. Wrap the read too.\n\nThe branch is behind `main`; gh-desk updates it before merging when you merge from here.",
			followUp: "The system choice follows `prefers-color-scheme` live, through the `change` listener in `theme.ts:18`.\n\nThis is the demo: Claude's answers here are written in advance.",
			draft:    draftReview{model.EventRequestChanges, "`theme.ts:5` reads `localStorage` outside a `try`. Safari in private mode throws there, and the settings page fails to render. Could you wrap the read as the write is?"},
		},
		{
			key: "acme/api#479", title: "Drop support for Go 1.22", author: "lena",
			in: []string{"review"}, requested: you(), branch: "lena/go-1.23",
			created: 5 * day, updated: 6 * hour,
			checks:   []check{ok("test (ubuntu-latest)", true), failed("test (macos-latest)", true), ok("lint", true)},
			reviews:  []model.Review{{Author: "omar", State: model.ReviewChangesRequested}},
			body:     "Go 1.22 is out of support. This raises `go` in `go.mod` to 1.23 and uses `iter.Seq` where we had our own iterator type.",
			comments: []comment{{"omar", 4 * day, "The macOS job fails on the new `os.CopyFS` call; it needs a fix before this goes in."}},
			diff:     diffGoVersion,
			answer:   "The macOS job fails in `internal/store/copy_test.go:31`: `os.CopyFS` keeps the mode of the files, and the test compares with `0o644` while the fixture is `0o600` on macOS checkouts. omar's request for changes is about the same failure.\n\nThis blocks the merge until the test passes.",
			followUp: "This is the demo: Claude's answers here are written in advance.",
			draft:    draftReview{model.EventComment, "The macOS failure is `copy_test.go:31`: `os.CopyFS` keeps the file mode, so the comparison with `0o644` fails where the fixture is `0o600`."},
		},
		{
			key: "acme/infra#215", title: "Move the staging cluster to the new VPC", author: "omar",
			in: []string{"review"}, requested: team("acme/platform"), branch: "omar/staging-vpc",
			created: 2 * day, updated: 3 * hour, labels: []string{"infra", "staging"},
			checks: []check{ok("terraform plan", true), ok("tflint", true)},
			body:   "Moves `staging` to `vpc-staging-2`, which has the private subnets the new database needs.\n\nThe plan replaces the node group; staging is down for about ten minutes while it does.",
			diff:   diffVPC, methods: []model.MergeMethod{model.MethodMerge},
			answer:   "The plan replaces the node group, as the description says. The new subnets are referenced by name through `data \"aws_subnets\"`, so nothing is hard-coded.\n\n`.github/workflows/plan.yaml` changes too: the job now runs on `pull_request_target`, which gives the workflow secrets to pull requests from forks. Check that this is wanted; it is the one change here I would not merge without a second look.",
			followUp: "This is the demo: Claude's answers here are written in advance.",
			draft:    draftReview{model.EventComment, "`plan.yaml` now runs on `pull_request_target`, which hands the secrets to pull requests from forks. Is that needed for the new VPC?"},
		},
		{
			key: "acme/design-system#88", title: "Button: add a loading state", author: "priya",
			in: []string{"review"}, requested: team("acme/frontend"), branch: "priya/button-loading",
			created: 4 * day, updated: 20 * hour, labels: []string{"component"},
			checks:   webChecks(),
			reviews:  []model.Review{{Author: "kenji", State: model.ReviewApproved}},
			body:     "`<Button loading>` shows a spinner in place of the icon and sets `aria-busy`. The width stays the same, so the layout does not jump.",
			diff:     diffButton,
			answer:   "Small and well tested. `aria-busy` is set, and clicks are ignored while loading, which the new test covers. Nothing blocks a merge.",
			followUp: "This is the demo: Claude's answers here are written in advance.",
			draft:    draftReview{model.EventApprove, "Looks good. Thanks for the test of the clicks while loading."},
		},
		{
			key: "acme/web#1284", title: "Fix the date picker on Safari", author: "lena",
			in: []string{"review"}, branch: "lena/safari-date",
			created: 6 * day, updated: 1 * day,
			checks:  webChecks(),
			reviews: []model.Review{{Author: Viewer, State: model.ReviewCommented}},
			body:    "Safari parses `2024-03-01` as UTC and shows the day before in the Americas. The picker now builds the date from its parts.",
			comments: []comment{
				{Viewer, 2 * day, "Could you add a test with a negative offset?"},
				{"lena", 1 * day, "Added one for `America/Los_Angeles`."},
			},
			diff:     diffDatePicker,
			answer:   "The fix builds the date with `new Date(y, m - 1, d)`, which is local time on every browser. The test you asked for is there, in `DatePicker.test.ts:40`. Nothing blocks a merge.",
			followUp: "This is the demo: Claude's answers here are written in advance.",
			draft:    draftReview{model.EventApprove, "Thanks for the test. Looks good to me."},
		},
		{
			key: "acme/api#486", title: "fix(deps): update module golang.org/x/net to v0.33.0", author: "renovate", bot: true,
			in: []string{"review"}, branch: "renovate/golang.org-x-net-0.x",
			created: 9 * hour, updated: 9 * hour, labels: []string{"dependencies"},
			checks:   goChecks(),
			body:     renovateBody("golang.org/x/net", "patch", "v0.32.0", "v0.33.0"),
			diff:     diffXNet,
			answer:   "A patch release of `golang.org/x/net` that fixes a parsing issue in `html`. Only `go.mod` and `go.sum` change. Safe to merge.",
			followUp: "This is the demo: Claude's answers here are written in advance.",
			draft:    draftReview{model.EventApprove, ""},
		},
		{
			key: "acme/web#1293", title: "chore(deps): update dependency vite to v7", author: "renovate", bot: true,
			in: []string{"review"}, branch: "renovate/vite-7.x",
			created: 14 * hour, updated: 14 * hour, labels: []string{"dependencies", "major"},
			checks:   webChecks(),
			body:     renovateBody("vite", "major", "6.3.5", "7.0.0") + "\n### Release Notes\n\n**vite 7.0.0**\n\n- **Breaking:** Node.js 18 is no longer supported; Node.js 20.19 or later is required.\n- **Breaking:** the default `build.target` is now `baseline-widely-available`.\n- `splitVendorChunkPlugin` is removed.\n",
			diff:     diffVite,
			answer:   "A major update. The release notes drop Node.js 18 and change the default `build.target`. `.nvmrc` here says `18`, so the deploy would break even though CI passes (CI runs on Node 22). Update `.nvmrc` before merging.",
			followUp: "This is the demo: Claude's answers here are written in advance.",
			draft:    draftReview{model.EventComment, "vite 7 needs Node.js 20.19 or later, and `.nvmrc` still says 18. Could we raise it in this PR?"},
		},
		{
			key: "acme/web#1291", title: "chore(deps): lock file maintenance", author: "renovate", bot: true,
			in: []string{"review"}, branch: "renovate/lock-file-maintenance",
			created: 1 * day, updated: 1 * day, labels: []string{"dependencies"},
			checks:   webChecks(),
			body:     "This PR contains the following updates:\n\n| Update | Change |\n|---|---|\n| lockFileMaintenance | All locks refreshed |\n\n🔧 This Pull Request updates lock files to use the latest dependency versions.\n",
			diff:     diffLock,
			answer:   "Only `pnpm-lock.yaml` changes, within the ranges in `package.json`. Safe to merge.",
			followUp: "This is the demo: Claude's answers here are written in advance.",
			draft:    draftReview{model.EventApprove, ""},
		},
		{
			key: "acme/infra#219", title: "Bump hashicorp/aws from 5.80.0 to 5.82.1", author: "dependabot", bot: true,
			in: []string{"review"}, branch: "dependabot/terraform/hashicorp/aws-5.82.1",
			created: 2 * day, updated: 2 * day, labels: []string{"dependencies"},
			checks: []check{ok("terraform plan", true), ok("tflint", true)},
			body:   "Bumps [hashicorp/aws](https://github.com/hashicorp/terraform-provider-aws) from 5.80.0 to 5.82.1.\n\n<details>\n<summary>Release notes</summary>\n\n- resource/aws_eks_node_group: fix a perpetual diff on `launch_template`\n\n</details>\n",
			diff:   diffAWS, methods: []model.MergeMethod{model.MethodMerge},
			answer:   "Minor updates of the provider; the plan shows no change. Safe to merge.",
			followUp: "This is the demo: Claude's answers here are written in advance.",
			draft:    draftReview{model.EventApprove, ""},
		},
		{
			key: "acme/api#485", title: "chore(deps): update actions/checkout action to v5", author: "renovate", bot: true,
			in: []string{"review"}, branch: "renovate/actions-checkout-5.x",
			created: 3 * day, updated: 3 * day, labels: []string{"dependencies"},
			checks:   goChecks(),
			body:     renovateBody("actions/checkout", "major", "v4", "v5"),
			diff:     diffCheckout,
			answer:   "`actions/checkout` v5 runs on Node 24; nothing else changes for this workflow. It changes a file under `.github/workflows`, so the merge popup marks it.",
			followUp: "This is the demo: Claude's answers here are written in advance.",
			draft:    draftReview{model.EventApprove, ""},
		},
		{
			key: "acme/api#477", title: "Add rate limits per API key", author: Viewer,
			in: []string{"mine"}, branch: "octocat/rate-limits",
			created: 4 * day, updated: 50 * minute,
			checks:  goChecks(),
			reviews: []model.Review{{Author: "hana", State: model.ReviewApproved}},
			body:    "Each API key gets its own token bucket, 100 requests a minute by default, set per key in the admin page.",
			comments: []comment{
				{"hana", 3 * hour, "LGTM. One nit: `limiter.go:40` could reuse the clock from the server."},
				{Viewer, 50 * minute, "Done, thanks!"},
			},
			diff:     diffRateLimit,
			answer:   "The bucket refills lazily on each request, so idle keys cost nothing. hana's nit is addressed. Ready to merge.",
			followUp: "This is the demo: Claude's answers here are written in advance.",
		},
		{
			key: "acme/web#1270", title: "WIP: new onboarding flow", author: Viewer,
			in: []string{"mine"}, isDraft: true, branch: "octocat/onboarding",
			created: 50 * day, updated: 41 * day,
			checks:   []check{ok("build", true), failed("unit", true)},
			body:     "Not ready yet: the copy is a placeholder.",
			diff:     diffOnboarding,
			answer:   "The flow is a draft; `Onboarding.test.tsx` fails on the step count, which changed from 3 to 4.",
			followUp: "This is the demo: Claude's answers here are written in advance.",
		},
		{
			key: "acme/infra#210", title: "Rotate the database credentials every month", author: Viewer,
			in: []string{"mine"}, branch: "octocat/rotate-db",
			created: 8 * day, updated: 2 * day,
			checks:   []check{ok("terraform plan", true), ok("tflint", true)},
			reviews:  []model.Review{{Author: "omar", State: model.ReviewChangesRequested}},
			body:     "Adds a rotation schedule to the secret of the staging and production databases.",
			comments: []comment{{"omar", 2 * day, "Rotation needs the app to reload the secret; can we land acme/api#470 first?"}},
			diff:     diffRotate, methods: []model.MergeMethod{model.MethodMerge},
			answer:   "omar asked for changes: the app reads the password once at start, so a rotation would break it until a restart.",
			followUp: "This is the demo: Claude's answers here are written in advance.",
		},
		{
			key: "acme/mobile#342", title: "Show a banner when offline", author: "sam",
			in: []string{"involved"}, branch: "sam/offline-banner",
			created: 2 * day, updated: 4 * hour,
			checks:   []check{ok("ios", true), ok("android", true)},
			body:     "A banner at the top says the app is offline, and goes away when the network comes back.",
			comments: []comment{{"sam", 4 * hour, "@octocat could you check the copy? I wasn't sure about \"You're offline\" vs \"No connection\"."}},
			diff:     diffOffline,
			answer:   "The banner listens to `NetInfo` and debounces for 2s, so a short drop does not flash it. sam asked you about the copy in the last comment.",
			followUp: "This is the demo: Claude's answers here are written in advance.",
		},
		{
			key: "acme/api#470", title: "Reload the database secret without a restart", author: "kenji",
			in: []string{"involved"}, branch: "kenji/reload-secret",
			created: 7 * day, updated: 1 * day,
			checks:   goChecks(),
			reviews:  []model.Review{{Author: "hana", State: model.ReviewApproved}},
			body:     "The pool reads the password from the secret again when a connection fails to authenticate.",
			diff:     diffReload,
			answer:   "The pool retries once with a fresh secret on an authentication failure, which is what acme/infra#210 needs. Approved by hana.",
			followUp: "This is the demo: Claude's answers here are written in advance.",
		},
	}
}

// notifications are the inbox at the start.
func notifications(start time.Time) []model.Notification {
	n := func(id, key, title, reason string, unread bool, ago time.Duration, comment bool) model.Notification {
		k, _ := model.ParsePRKey(key)
		url := ""
		if comment {
			url = "https://api.github.com/repos/" + k.Repository() + "/issues/comments/" + id
		}
		return model.Notification{ThreadID: id, Key: k, Title: title, Reason: reason, Unread: unread, Updated: start.Add(-ago), LatestCommentURL: url}
	}
	return []model.Notification{
		n("1001", "acme/api#482", "Retry webhook deliveries with exponential backoff", "review_requested", true, 35*minute, false),
		n("1002", "acme/mobile#342", "Show a banner when offline", "mention", true, 4*hour, true),
		n("1003", "acme/api#477", "Add rate limits per API key", "comment", true, 3*hour, true),
		n("1004", "acme/web#1290", "Add a dark mode toggle to the settings page", "review_requested", false, 2*hour, false),
		n("1005", "acme/infra#210", "Rotate the database credentials every month", "author", false, 2*day, true),
		n("1006", "acme/web#1284", "Fix the date picker on Safari", "comment", false, 1*day, true),
	}
}

func renovateBody(pkg, update, from, to string) string {
	return fmt.Sprintf("This PR contains the following updates:\n\n| Package | Update | Change |\n|---|---|---|\n| %s | %s | `%s` -> `%s` |\n\n---\n\n### Configuration\n\n📅 **Schedule**: Branch creation - At any time, Automerge - At any time.\n\n🚦 **Automerge**: Disabled by config.\n", pkg, update, from, to)
}
