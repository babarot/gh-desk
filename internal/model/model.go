// Package model holds the types every other package shares: what a pull
// request is and how one is named. It has no behavior beyond reading and
// writing those names, and imports nothing outside the standard library, so
// any package may use it.
package model

import (
	"fmt"
	"strconv"
	"strings"
)

// PRKey names a pull request as owner/repo#number.
type PRKey struct {
	Owner  string
	Repo   string
	Number int
}

// String writes the key as owner/repo#number.
func (k PRKey) String() string {
	return fmt.Sprintf("%s/%s#%d", k.Owner, k.Repo, k.Number)
}

// Repository is the key's owner/repo.
func (k PRKey) Repository() string {
	return k.Owner + "/" + k.Repo
}

// ParsePRKey reads owner/repo#number.
func ParsePRKey(s string) (PRKey, error) {
	repo, num, ok := strings.Cut(s, "#")
	if !ok {
		return PRKey{}, fmt.Errorf("%q is not owner/repo#number", s)
	}
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return PRKey{}, fmt.Errorf("%q is not owner/repo#number", s)
	}
	n, err := strconv.Atoi(num)
	if err != nil || n <= 0 {
		return PRKey{}, fmt.Errorf("%q is not owner/repo#number", s)
	}
	return PRKey{Owner: owner, Repo: name, Number: n}, nil
}

// MarshalText writes the key as owner/repo#number, so it can be a JSON
// string.
func (k PRKey) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

// UnmarshalText reads owner/repo#number.
func (k *PRKey) UnmarshalText(b []byte) error {
	got, err := ParsePRKey(string(b))
	if err != nil {
		return err
	}
	*k = got
	return nil
}
