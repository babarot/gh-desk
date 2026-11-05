package recording

import (
	"fmt"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

// Viewer is the login the demo runs as. Every name in the demo is made up:
// the organization, its repositories, and the people in it.
const Viewer = "kai"

// pr is a made-up pull request and what the demo tells of it besides the
// list's fields.
type pr struct {
	model.PR
	body     string
	diff     string
	files    []string
	checks   []model.Check
	comments []model.Comment
}

func key(repo string, n int) model.PRKey { return model.PRKey{Owner: "acme", Repo: repo, Number: n} }

// prs are the pull requests of the demo, made at now, in the order the
// searches find them.
func prs(now time.Time) []pr {
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	me := model.Reviewer{Login: Viewer}
	platform := model.Reviewer{Login: "platform", Team: true}
	web := model.Reviewer{Login: "web", Team: true}
	ok := []model.Check{{Name: "test", State: model.CheckSuccess, Required: true}, {Name: "lint", State: model.CheckSuccess}}
	n := 0
	mk := func(repo string, num int, title, author string, add, del, files int, checks model.CheckState, updated time.Duration) model.PR {
		n++
		k := key(repo, num)
		return model.PR{ID: fmt.Sprintf("PR_%d", n), Key: k, Title: title, Author: author, State: model.StateOpen,
			URL: "https://github.com/" + k.Repository() + fmt.Sprintf("/pull/%d", num), HeadRef: "topic", HeadRepo: k.Repository(), HeadOID: fmt.Sprintf("%07x0", n*0x1f3a5),
			BaseRef: "main", Additions: add, Deletions: del, ChangedFiles: files, Checks: checks,
			Decision: model.DecisionReviewRequired, Created: ago(updated + 26*time.Hour), Updated: ago(updated)}
	}
	bot := func(p model.PR) model.PR {
		p.AuthorIsBot = true
		p.Requested = []model.Reviewer{me}
		p.Labels = []string{"dependencies"}
		return p
	}

	out := []pr{
		{PR: with(mk("api", 482, "Retry idempotent requests with backoff", "alice", 146, 38, 5, model.CheckSuccess, 25*time.Minute),
			func(p *model.PR) { p.Requested = []model.Reviewer{me}; p.Comments = 2 }),
			body: retryBody, diff: retryDiff, files: []string{"client/client.go", "client/retry.go", "client/retry_test.go", "go.mod", "go.sum"},
			checks: ok, comments: []model.Comment{
				{Author: "dave", Body: "Does this cover the 429s the gateway sends, or only 5xx?", Created: ago(3 * time.Hour)},
				{Author: "alice", Body: "Both: 429 waits for `Retry-After` when it is there.", Created: ago(2 * time.Hour)},
			}},
		{PR: with(mk("web", 219, "Show the next invoice on the billing page", "bob", 312, 95, 11, model.CheckPending, 2*time.Hour),
			func(p *model.PR) { p.Requested = []model.Reviewer{me, web} }),
			body:  "## What\n\nThe billing page shows the next invoice: its date, the amount, and the plan it is for.\n\n## Why\n\nPeople open a ticket to ask what they will be charged next.\n",
			diff:  "diff --git a/src/billing/Invoice.tsx b/src/billing/Invoice.tsx\n--- a/src/billing/Invoice.tsx\n+++ b/src/billing/Invoice.tsx\n@@ -1,4 +1,9 @@\n import { usePlan } from \"../plan\";\n+import { formatMoney } from \"../money\";\n \n export function Invoice() {\n   const plan = usePlan();\n+  const next = plan.nextInvoice;\n+  if (!next) return null;\n+  return <p>Next invoice: {formatMoney(next.amount)} on {next.date}</p>;\n }\n",
			files: []string{"src/billing/Invoice.tsx"}, checks: []model.Check{{Name: "test", State: model.CheckPending, Required: true}}},
		{PR: with(mk("cli", 77, "Add --json to the status command", "carol", 88, 12, 3, model.CheckSuccess, 5*time.Hour),
			func(p *model.PR) {
				p.Requested = []model.Reviewer{me}
				p.Reviews = []model.Review{{Author: "dave", State: model.ReviewApproved}}
			}),
			body:  "Print the status as JSON with `--json`, for scripts.\n\n```console\n$ acme status --json\n{\"deploy\":\"ok\",\"version\":\"1.8.2\"}\n```\n",
			diff:  "diff --git a/cmd/status.go b/cmd/status.go\n--- a/cmd/status.go\n+++ b/cmd/status.go\n@@ -12,6 +12,10 @@ func status(cmd *cobra.Command) error {\n \ts, err := client.Status(cmd.Context())\n \tif err != nil {\n \t\treturn err\n \t}\n+\tif jsonOut {\n+\t\treturn json.NewEncoder(cmd.OutOrStdout()).Encode(s)\n+\t}\n \tfmt.Fprintln(cmd.OutOrStdout(), s)\n",
			files: []string{"cmd/status.go"}, checks: ok},
		{PR: with(mk("infra", 130, "Move the staging cluster to the new node pool", "erin", 64, 51, 4, model.CheckSuccess, 7*time.Hour),
			func(p *model.PR) { p.Requested = []model.Reviewer{platform} }),
			body: "Staging moves to the `general-2` node pool, as production did last week.\n", files: []string{"staging/cluster.tf"}, checks: ok},
		{PR: with(mk("api", 479, "Drop the v1 webhooks endpoint", "frank", 12, 230, 7, model.CheckSuccess, 30*time.Hour),
			func(p *model.PR) { p.Requested = []model.Reviewer{platform}; p.Draft = true }),
			body: "No one has called `/v1/webhooks` for ninety days.\n", checks: ok},
		{PR: with(mk("web", 210, "Lazy-load the charts on the dashboard", "grace", 54, 20, 3, model.CheckSuccess, 26*time.Hour),
			func(p *model.PR) {
				p.Requested = []model.Reviewer{me}
				p.Reviews = []model.Review{{Author: "henry", State: model.ReviewCommented}}
			}),
			body: "The charts load once they scroll into view.\n", checks: ok},

		{PR: bot(mk("api", 486, "Update module golang.org/x/net to v0.47.0", "renovate", 4, 4, 2, model.CheckSuccess, 40*time.Minute)),
			body: renovate("golang.org/x/net", "require", "minor", "v0.46.0", "v0.47.0"), files: []string{"go.mod", "go.sum"}, checks: ok,
			diff: "diff --git a/go.mod b/go.mod\n--- a/go.mod\n+++ b/go.mod\n@@ -5,3 +5,3 @@ go 1.26\n require (\n-\tgolang.org/x/net v0.46.0\n+\tgolang.org/x/net v0.47.0\n )\n"},
		{PR: bot(mk("web", 224, "Update dependency vite to v7.1.4", "renovate", 3, 3, 2, model.CheckSuccess, 1*time.Hour)),
			body: renovate("vite", "devDependencies", "patch", "7.1.3", "7.1.4"), files: []string{"package.json", "pnpm-lock.yaml"}, checks: ok},
		{PR: bot(mk("web", 223, "Update dependency typescript to v5.9.3", "renovate", 3, 3, 2, model.CheckSuccess, 1*time.Hour)),
			body: renovate("typescript", "devDependencies", "patch", "5.9.2", "5.9.3"), files: []string{"package.json", "pnpm-lock.yaml"}, checks: ok},
		{PR: bot(mk("infra", 133, "Update actions/checkout action to v7", "renovate", 2, 2, 1, model.CheckSuccess, 3*time.Hour)),
			body: renovate("actions/checkout", "action", "major", "v6", "v7"), files: []string{".github/workflows/deploy.yaml"}, checks: ok},
		{PR: bot(mk("cli", 80, "Bump github.com/spf13/cobra from 1.9.1 to 1.10.1", "dependabot", 3, 3, 2, model.CheckSuccess, 4*time.Hour)),
			body: "Bumps [github.com/spf13/cobra](https://github.com/spf13/cobra) from 1.9.1 to 1.10.1.\n", files: []string{"go.mod", "go.sum"}, checks: ok},
		{PR: bot(mk("docs", 41, "Update dependency astro to v6", "renovate", 3, 3, 2, model.CheckFailure, 6*time.Hour)),
			body: renovate("astro", "dependencies", "major", "5.14.1", "6.0.2"), files: []string{"package.json", "pnpm-lock.yaml"},
			checks: []model.Check{{Name: "build", State: model.CheckFailure, Required: true}}},

		{PR: with(mk("api", 488, "Log the request ID on every error", Viewer, 41, 9, 3, model.CheckSuccess, 15*time.Minute),
			func(p *model.PR) {
				p.Reviews = []model.Review{{Author: "alice", State: model.ReviewApproved}}
				p.Decision = model.DecisionApproved
			}),
			body: "Every error the API logs has the request ID, so a report can be found in the logs.\n", checks: ok},
		{PR: with(mk("docs", 39, "Explain the rate limits", Viewer, 120, 4, 2, model.CheckSuccess, 50*time.Hour),
			func(p *model.PR) { p.Draft = true }),
			body: "A page on the rate limits and the headers that tell them.\n", checks: ok},

		{PR: with(mk("web", 205, "Remember the theme the user picked", "henry", 23, 6, 2, model.CheckSuccess, 9*time.Hour),
			func(p *model.PR) { p.Comments = 4 }),
			body: "The theme is kept in local storage.\n", checks: ok},
	}
	return out
}

func with(p model.PR, f func(*model.PR)) model.PR {
	f(&p)
	return p
}

// searchOf is the tab of the default config whose query found p.
func searchOf(p model.PR) string {
	switch {
	case p.Author == Viewer:
		return "mine"
	case len(p.Requested) == 0:
		return "involved"
	}
	return "review"
}

// notifications are the inbox of the demo.
func notifications(all []pr) []model.Notification {
	reasons := map[model.PRKey]string{key("api", 482): "review_requested", key("web", 219): "review_requested",
		key("api", 488): "author", key("web", 205): "mention", key("cli", 77): "review_requested"}
	var out []model.Notification
	for i, p := range all {
		r, ok := reasons[p.Key]
		if !ok {
			continue
		}
		out = append(out, model.Notification{ThreadID: fmt.Sprintf("T_%d", i), Key: p.Key, Title: p.Title, Reason: r,
			Unread: i%2 == 0, Updated: p.Updated})
	}
	return out
}

// renovate is a description as Renovate writes it.
func renovate(pkg, typ, update, from, to string) string {
	return fmt.Sprintf(`This PR contains the following updates:

| Package | Type | Update | Change |
|---|---|---|---|
| %s | %s | %s | `+"`%s` -> `%s`"+` |

---

### Release Notes

<details>
<summary>%s</summary>

### %s

- Fixes and improvements.

</details>

---

### Configuration

📅 **Schedule**: Branch creation - At any time.

🚦 **Automerge**: Disabled by config.
`, pkg, typ, update, from, to, pkg, to)
}

const retryBody = `## What

The client retries a request that failed with a 5xx or a 429, when the
request is idempotent: GET, HEAD, PUT and DELETE, and a POST that carries an
` + "`Idempotency-Key`" + `.

- Waits grow from 200ms to 3.2s, with jitter
- A 429 waits for ` + "`Retry-After`" + ` when the server sends it
- At most four tries; the last error is returned

## Why

The gateway drops a few requests during a deploy, and every caller has its
own loop around the client to get past it.
`

const retryDiff = `diff --git a/client/retry.go b/client/retry.go
new file mode 100644
--- /dev/null
+++ b/client/retry.go
@@ -0,0 +1,24 @@
+package client
+
+import (
+	"math/rand/v2"
+	"net/http"
+	"time"
+)
+
+const maxTries = 4
+
+// retryable is whether a request may be sent again after resp.
+func retryable(req *http.Request, resp *http.Response) bool {
+	if !idempotent(req) {
+		return false
+	}
+	return resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
+}
+
+// backoff is the wait before try n, from 200ms up, with jitter.
+func backoff(n int) time.Duration {
+	d := 200 * time.Millisecond << n
+	return d/2 + rand.N(d/2)
+}
diff --git a/client/client.go b/client/client.go
--- a/client/client.go
+++ b/client/client.go
@@ -41,9 +41,18 @@ func (c *Client) Do(req *http.Request) (*http.Response, error) {
 	req.Header.Set("User-Agent", c.agent)
-	resp, err := c.http.Do(req)
-	if err != nil {
-		return nil, err
+	var resp *http.Response
+	var err error
+	for n := 0; n < maxTries; n++ {
+		resp, err = c.http.Do(req)
+		if err != nil || !retryable(req, resp) || n == maxTries-1 {
+			break
+		}
+		resp.Body.Close()
+		time.Sleep(wait(resp, n))
 	}
 	return resp, err
 }
diff --git a/go.sum b/go.sum
--- a/go.sum
+++ b/go.sum
@@ -1,2 +1,2 @@
-golang.org/x/net v0.46.0 h1:...
+golang.org/x/net v0.47.0 h1:...
`
