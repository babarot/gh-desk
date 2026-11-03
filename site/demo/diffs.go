package demo

import (
	"fmt"
	"strings"
)

// patch writes a file's patch as git does. Each hunk is written as its
// lines, the first of them "@@ N section" with N the hunk's first line
// before the change; the counts of the header are worked out here. A file
// with no lines before (added) is written with N 0.
func patch(path string, hunks ...string) string {
	var b strings.Builder
	added := strings.HasPrefix(hunks[0], "@@ 0")
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n", path, path)
	if added {
		b.WriteString("new file mode 100644\n")
	}
	fmt.Fprintf(&b, "index %s..%s 100644\n", oid(path)[:7], oid(path + "2")[:7])
	if added {
		b.WriteString("--- /dev/null\n")
	} else {
		fmt.Fprintf(&b, "--- a/%s\n", path)
	}
	fmt.Fprintf(&b, "+++ b/%s\n", path)
	shift := 0
	for _, h := range hunks {
		head, body, _ := strings.Cut(strings.TrimPrefix(h, "\n"), "\n")
		var start int
		var section string
		fields := strings.SplitN(strings.TrimPrefix(head, "@@ "), " ", 2)
		fmt.Sscan(fields[0], &start)
		if len(fields) == 2 {
			section = " " + fields[1]
		}
		lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
		old, now := 0, 0
		for _, l := range lines {
			switch {
			case strings.HasPrefix(l, "+"):
				now++
			case strings.HasPrefix(l, "-"):
				old++
			default:
				old++
				now++
			}
		}
		newStart := start + shift
		if added {
			newStart = 1
		}
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@%s\n", start, old, newStart, now, section)
		for _, l := range lines {
			if l == "" {
				l = " "
			}
			b.WriteString(l)
			b.WriteByte('\n')
		}
		shift += now - old
	}
	return b.String()
}

var diffWebhook = patch("internal/webhook/deliver.go", `
@@ 1 package webhook
 package webhook

 import (
 	"bytes"
 	"context"
+	"math/rand/v2"
 	"net/http"
 	"time"
 )

+// maxAttempts is how many times a delivery is tried before it is given up.
+const maxAttempts = 5
+
 // Deliver sends the event to the endpoint of the subscription.`, `
@@ 40 func (d *Deliverer) Deliver(ctx context.Context, s Subscription, ev Event) error {
 	body, err := ev.Encode()
 	if err != nil {
 		return err
 	}
-	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(body))
-	if err != nil {
-		return err
-	}
-	resp, err := d.client.Do(req)
-	if err != nil {
-		return err
+	var last error
+	for attempt := 1; attempt <= maxAttempts; attempt++ {
+		d.store.RecordAttempt(ctx, s.ID, ev.ID, attempt)
+		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(body))
+		if err != nil {
+			return err
+		}
+		resp, err := d.client.Do(req)
+		if err == nil {
+			resp.Body.Close()
+			if !retryable(resp.StatusCode) {
+				return statusError(resp.StatusCode)
+			}
+			last = statusError(resp.StatusCode)
+		} else {
+			last = err
+		}
+		time.Sleep(backoff(attempt))
 	}
-	defer resp.Body.Close()
-	return statusError(resp.StatusCode)
+	return last
+}
+
+// retryable reports whether a delivery that got status is tried again.
+func retryable(status int) bool {
+	return status == http.StatusTooManyRequests || status >= 500
+}
+
+// backoff is how long to wait after the attempt: 1s, 2s, 4s, 8s, with
+// 20% of jitter either way.
+func backoff(attempt int) time.Duration {
+	d := time.Second << (attempt - 1)
+	jitter := time.Duration(rand.Int64N(int64(d) * 2 / 5))
+	return d - d/5 + jitter
 }`) + patch("internal/webhook/deliver_test.go", `
@@ 88 func TestDeliver(t *testing.T) {
 		t.Fatal(err)
 	}
 }
+
+func TestDeliverRetries(t *testing.T) {
+	for _, tt := range []struct {
+		status   int
+		attempts int
+	}{
+		{http.StatusOK, 1},
+		{http.StatusNotFound, 1},
+		{http.StatusTooManyRequests, 5},
+		{http.StatusServiceUnavailable, 5},
+	} {
+		srv, got := countingServer(tt.status)
+		d := newTestDeliverer(srv)
+		_ = d.Deliver(context.Background(), Subscription{URL: srv.URL}, testEvent)
+		if *got != tt.attempts {
+			t.Errorf("status %d: %d attempts, want %d", tt.status, *got, tt.attempts)
+		}
+	}
+}`) + patch("internal/store/migrations/0042_webhook_attempts.sql", `
@@ 0
+CREATE TABLE webhook_attempts (
+    subscription_id BIGINT NOT NULL REFERENCES subscriptions (id),
+    event_id        UUID NOT NULL,
+    attempt         SMALLINT NOT NULL,
+    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
+    PRIMARY KEY (subscription_id, event_id, attempt)
+);`)

var diffDarkMode = patch("src/settings/ThemeToggle.tsx", `
@@ 0
+import { useState } from "react";
+import { Theme, applyTheme, savedTheme } from "../theme";
+
+const choices: { value: Theme; label: string }[] = [
+  { value: "light", label: "Light" },
+  { value: "dark", label: "Dark" },
+  { value: "system", label: "System" },
+];
+
+export function ThemeToggle() {
+  const [theme, setTheme] = useState<Theme>(savedTheme());
+
+  function choose(t: Theme) {
+    setTheme(t);
+    applyTheme(t);
+    try {
+      localStorage.setItem("theme", t);
+    } catch {
+      // private mode: the choice lasts until the tab is closed
+    }
+  }
+
+  return (
+    <fieldset className="theme-toggle">
+      <legend>Appearance</legend>
+      {choices.map((c) => (
+        <label key={c.value}>
+          <input type="radio" name="theme" checked={theme === c.value} onChange={() => choose(c.value)} />
+          {c.label}
+        </label>
+      ))}
+    </fieldset>
+  );
+}`) + patch("src/theme.ts", `
@@ 0
+export type Theme = "light" | "dark" | "system";
+
+export function savedTheme(): Theme {
+  const t = localStorage.getItem("theme");
+  return t === "light" || t === "dark" ? t : "system";
+}
+
+export function applyTheme(t: Theme) {
+  const dark = t === "dark" || (t === "system" && matchMedia("(prefers-color-scheme: dark)").matches);
+  document.documentElement.dataset.theme = dark ? "dark" : "light";
+}
+
+matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
+  applyTheme(savedTheme());
+});`) + patch("src/settings/SettingsPage.tsx", `
@@ 12 export function SettingsPage() {
       <ProfileSection />
       <NotificationSection />
+      <ThemeToggle />
     </main>
   );`) + patch("src/styles/tokens.css", `
@@ 1 :root {
 :root {
   --bg: #ffffff;
   --fg: #1f2328;
+}
+
+[data-theme="dark"] {
+  --bg: #0d1117;
+  --fg: #e6edf3;
 }`)

var diffGoVersion = patch("go.mod", `
@@ 1 module github.com/acme/api
 module github.com/acme/api

-go 1.22
+go 1.23`) + patch("internal/store/iter.go", `
@@ 1 package store
 package store

-// Iterator walks over the rows of a query.
-type Iterator[T any] struct {
-	rows *sql.Rows
-	scan func(*sql.Rows) (T, error)
-}
-
-// Next reports whether there is another row.
-func (it *Iterator[T]) Next() bool { return it.rows.Next() }
+import "iter"
+
+// All walks over the rows of a query.
+func All[T any](rows *sql.Rows, scan func(*sql.Rows) (T, error)) iter.Seq2[T, error] {
+	return func(yield func(T, error) bool) {
+		defer rows.Close()
+		for rows.Next() {
+			if !yield(scan(rows)) {
+				return
+			}
+		}
+	}
+}`) + patch("internal/store/copy_test.go", `
@@ 24 func TestCopyFixtures(t *testing.T) {
 	dst := t.TempDir()
-	if err := copyDir(dst, "testdata"); err != nil {
+	if err := os.CopyFS(dst, os.DirFS("testdata")); err != nil {
 		t.Fatal(err)
 	}
 	fi, err := os.Stat(filepath.Join(dst, "users.json"))
 	if err != nil {
 		t.Fatal(err)
 	}
 	if fi.Mode().Perm() != 0o644 {`)

var diffVPC = patch("envs/staging/main.tf", `
@@ 1 module "cluster" {
 data "aws_vpc" "main" {
-  tags = { Name = "vpc-staging" }
+  tags = { Name = "vpc-staging-2" }
 }

+data "aws_subnets" "private" {
+  filter {
+    name   = "vpc-id"
+    values = [data.aws_vpc.main.id]
+  }
+  tags = { Tier = "private" }
+}
+
 module "cluster" {
   source     = "../../modules/eks"
   name       = "staging"
   vpc_id     = data.aws_vpc.main.id
-  subnet_ids = ["subnet-0a1b2c", "subnet-3d4e5f"]
+  subnet_ids = data.aws_subnets.private.ids
 }`) + patch(".github/workflows/plan.yaml", `
@@ 1 name: plan
 name: plan
 on:
-  pull_request:
+  pull_request_target:
     paths: ["envs/**", "modules/**"]`)

var diffButton = patch("src/Button/Button.tsx", `
@@ 8 export function Button({ icon, children, onClick, ...props }: ButtonProps) {
-export function Button({ icon, children, onClick, ...props }: ButtonProps) {
+export function Button({ icon, loading, children, onClick, ...props }: ButtonProps) {
   return (
-    <button className={styles.button} onClick={onClick} {...props}>
-      {icon}
+    <button className={styles.button} aria-busy={loading || undefined} onClick={loading ? undefined : onClick} {...props}>
+      {loading ? <Spinner size="small" /> : icon}
       {children}
     </button>
   );`) + patch("src/Button/Button.test.tsx", `
@@ 30 it("calls onClick", async () => {
   expect(onClick).toHaveBeenCalledOnce();
 });
+
+it("ignores clicks while loading", async () => {
+  const onClick = vi.fn();
+  render(<Button loading onClick={onClick}>Save</Button>);
+  await userEvent.click(screen.getByRole("button"));
+  expect(onClick).not.toHaveBeenCalled();
+  expect(screen.getByRole("button")).toHaveAttribute("aria-busy", "true");
+});`)

var diffDatePicker = patch("src/components/DatePicker.tsx", `
@@ 14 function parse(value: string): Date {
 function parse(value: string): Date {
-  return new Date(value);
+  const [y, m, d] = value.split("-").map(Number);
+  return new Date(y, m - 1, d);
 }`) + patch("src/components/DatePicker.test.ts", `
@@ 36 describe("parse", () => {
   });
+
+  it("keeps the day west of UTC", () => {
+    vi.stubEnv("TZ", "America/Los_Angeles");
+    expect(parse("2024-03-01").getDate()).toBe(1);
+  });
 });`)

var diffXNet = patch("go.mod", `
@@ 5 require (
 require (
 	github.com/jackc/pgx/v5 v5.7.2
-	golang.org/x/net v0.32.0
+	golang.org/x/net v0.33.0
 	golang.org/x/sync v0.10.0
 )`) + patch("go.sum", `
@@ 40
-golang.org/x/net v0.32.0 h1:ZqPmj8Kzc+Y6e0+skZsuACbx+wzMgo5MQsJh9Qd6aYI=
-golang.org/x/net v0.32.0/go.mod h1:CwU0IoeOlnQQWJ6ioyFrfRuomB8GKF6KbYXZVyeXNfs=
+golang.org/x/net v0.33.0 h1:74SYHlV8BIgHIFC/LrYkOGIwL19eTYXQ5wc6TBuO36I=
+golang.org/x/net v0.33.0/go.mod h1:HXLR5J+9DxmrqMwG9qjGCxZ+zKXxBru04zlTvWlWuN4=`)

var diffVite = patch("package.json", `
@@ 20 "devDependencies": {
     "typescript": "5.8.3",
-    "vite": "6.3.5",
+    "vite": "7.0.0",
     "vitest": "3.2.4"`) + patch("pnpm-lock.yaml", `
@@ 812
-  vite@6.3.5:
-    resolution: {integrity: sha512-cZn6NGlBGd0KT4ywBTcNBFM7XoPn8yXZ5JQ6kUDEpqY/A6kqGPQXSWxwMhAGg6zrf9ef0m0yeFVFGyw==}
-    engines: {node: ^18.0.0 || ^20.0.0 || >=22.0.0}
+  vite@7.0.0:
+    resolution: {integrity: sha512-ixXJB1YRgDIw2OszKQS9WxGHKwLdCsbQNkpJN171udl6szi/rIySHL6/Os3s2+oE4P/FLD4dxg4mD7Wust+u5g==}
+    engines: {node: ^20.19.0 || >=22.12.0}`)

var diffLock = patch("pnpm-lock.yaml", `
@@ 102
-  '@types/node@22.15.29':
-    resolution: {integrity: sha512-LNdjOkUDlU1RZb8e1kOIUpN1qQUlzGkEtbVNo53vbrwDg5om6oduhm4SiUaPW5ASTXhAiP0jInWG8Qx9fVlOeQ==}
+  '@types/node@22.15.33':
+    resolution: {integrity: sha512-wzoocdnnpSxZ+6CjW4ADCK1jVmd1S/J3ArNWfn8FDDQtRm8dkDg7TA+mvek2wNrfCgwuZxqEOiB9B1XCJ6+dbw==}`, `
@@ 2210
-  postcss@8.5.4:
-    resolution: {integrity: sha512-QSa9EBe+uwlGTFmHsPKokv3B/oEMQZxfqW0QqNCyhpa6mB1afzulwn8hihglqAb2pOw+BJgNlmXQ8la2VeHB7w==}
+  postcss@8.5.6:
+    resolution: {integrity: sha512-3Ybi1tAuwAP9s0r1UQ2J4n5Y0G05bJkpUIO0/bI9MhwmD70S5aTWbXGBwxHrelT+XM1k6dM0pk+SwNkpTRN7Pg==}`)

var diffAWS = patch(".terraform.lock.hcl", `
@@ 4
 provider "registry.terraform.io/hashicorp/aws" {
-  version     = "5.80.0"
-  constraints = "~> 5.80"
+  version     = "5.82.1"
+  constraints = "~> 5.82"
   hashes = [`) + patch("versions.tf", `
@@ 3 terraform {
     aws = {
       source  = "hashicorp/aws"
-      version = "~> 5.80"
+      version = "~> 5.82"
     }`)

var diffCheckout = patch(".github/workflows/ci.yaml", `
@@ 14 jobs:
     runs-on: ${{ matrix.os }}
     steps:
-      - uses: actions/checkout@v4
+      - uses: actions/checkout@v5
       - uses: actions/setup-go@v5`)

var diffRateLimit = patch("internal/ratelimit/limiter.go", `
@@ 0
+package ratelimit
+
+import (
+	"sync"
+	"time"
+)
+
+// Limiter keeps a token bucket for each API key.
+type Limiter struct {
+	mu      sync.Mutex
+	now     func() time.Time
+	buckets map[string]*bucket
+	rate    int
+}
+
+type bucket struct {
+	tokens float64
+	last   time.Time
+}
+
+// Allow reports whether key may make a request now, taking a token if so.
+func (l *Limiter) Allow(key string) bool {
+	l.mu.Lock()
+	defer l.mu.Unlock()
+	now := l.now()
+	b, ok := l.buckets[key]
+	if !ok {
+		b = &bucket{tokens: float64(l.rate), last: now}
+		l.buckets[key] = b
+	}
+	b.tokens = min(float64(l.rate), b.tokens+now.Sub(b.last).Minutes()*float64(l.rate))
+	b.last = now
+	if b.tokens < 1 {
+		return false
+	}
+	b.tokens--
+	return true
+}`) + patch("internal/server/middleware.go", `
@@ 30 func (s *Server) auth(next http.Handler) http.Handler {
 		key := r.Header.Get("Authorization")
+		if !s.limiter.Allow(key) {
+			w.Header().Set("Retry-After", "60")
+			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
+			return
+		}
 		next.ServeHTTP(w, r)`)

var diffOnboarding = patch("src/onboarding/Onboarding.tsx", `
@@ 0
+export function Onboarding() {
+  const steps = [<Welcome />, <Profile />, <Team />, <Done />];
+  const [step, setStep] = useState(0);
+  return <Wizard step={steps[step]} onNext={() => setStep(step + 1)} />;
+}`)

var diffRotate = patch("modules/database/secret.tf", `
@@ 12 resource "aws_secretsmanager_secret" "db" {
   name = "${var.env}/db/password"
 }
+
+resource "aws_secretsmanager_secret_rotation" "db" {
+  secret_id           = aws_secretsmanager_secret.db.id
+  rotation_lambda_arn = var.rotation_lambda_arn
+
+  rotation_rules {
+    schedule_expression = "rate(30 days)"
+  }
+}`)

var diffOffline = patch("src/components/OfflineBanner.tsx", `
@@ 0
+export function OfflineBanner() {
+  const online = useDebounced(useNetInfo().isConnected, 2000);
+  if (online !== false) return null;
+  return <Banner tone="warning">You're offline</Banner>;
+}`) + patch("src/App.tsx", `
@@ 20 export function App() {
     <SafeAreaProvider>
+      <OfflineBanner />
       <Navigation />`)

var diffReload = patch("internal/store/pool.go", `
@@ 52 func (p *Pool) connect(ctx context.Context) (*pgx.Conn, error) {
 	conn, err := pgx.ConnectConfig(ctx, p.config)
+	if isAuthFailure(err) {
+		// The secret may have been rotated: read it again and retry once.
+		if p.config.Password, err = p.secrets.Password(ctx); err != nil {
+			return nil, err
+		}
+		conn, err = pgx.ConnectConfig(ctx, p.config)
+	}
 	return conn, err`)
