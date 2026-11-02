package plan

import "testing"

// A pull request's body is written by whoever opened it. Reading the
// updates a dependency bot lists in it never panics, and finds none in a
// body without them.
func FuzzBumps(f *testing.F) {
	f.Add("| Package | Update | Change | Pending |\n|---|---|---|---|\n| [golang.org/x/net](https://example.com) | minor | `v0.1.0` -> `v0.2.0` | |\n")
	f.Add("Bumps [lodash](https://example.com) from 4.17.20 to 4.17.21.")
	f.Add("| Package | Update | Change |\n|---|\n|")
	f.Add("")
	f.Fuzz(func(t *testing.T, body string) {
		for _, b := range Bumps(body) {
			if b.Package == "" {
				t.Fatalf("a bump without a package in %q: %+v", body, b)
			}
		}
	})
}
