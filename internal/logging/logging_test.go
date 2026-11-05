package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want slog.Level
		ok   bool
	}{
		{"debug", slog.LevelDebug, true},
		{"INFO", slog.LevelInfo, true},
		{"", slog.LevelInfo, true},
		{"warn", slog.LevelWarn, true},
		{"error", slog.LevelError, true},
		{"trace", slog.LevelInfo, false},
	}
	for _, tt := range tests {
		got, err := ParseLevel(tt.in)
		if got != tt.want || (err == nil) != tt.ok {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v, ok=%v", tt.in, got, err, tt.want, tt.ok)
		}
	}
}

func TestNewWritesJSONLinesAtLevel(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, slog.LevelInfo).With(KeyRun, "r1")
	l.Debug("hidden")
	l.Info("shown", KeyPR, "o/r#1")
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %q", len(lines), buf.String())
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["msg"] != "shown" || rec[KeyRun] != "r1" || rec[KeyPR] != "o/r#1" {
		t.Errorf("record = %v", rec)
	}
}

func TestOpenRotates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.log")
	write := func(name, s string) {
		if err := os.WriteFile(name, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(path, "current-and-too-big")
	write(path+".1", "one")
	write(path+".2", "two")
	l, c, err := Open(Options{Path: path, MaxSize: 5, Keep: 2})
	if err != nil {
		t.Fatal(err)
	}
	l.Info("fresh")
	c.Close()
	read := func(name string) string {
		b, err := os.ReadFile(name)
		if err != nil {
			return ""
		}
		return string(b)
	}
	if got := read(path + ".1"); got != "current-and-too-big" {
		t.Errorf(".1 = %q", got)
	}
	if got := read(path + ".2"); got != "one" {
		t.Errorf(".2 = %q", got)
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Errorf(".3 exists beyond keep=2")
	}
	if got := read(path); !strings.Contains(got, `"msg":"fresh"`) {
		t.Errorf("new file = %q", got)
	}
}

func TestOpenKeepsSmallFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.log")
	if err := os.WriteFile(path, []byte("small\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l, c, err := Open(Options{Path: path, MaxSize: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	l.Info("more")
	c.Close()
	b, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(b), "small\n") {
		t.Errorf("file = %q; want appended", b)
	}
}

func TestNewID(t *testing.T) {
	a, b := NewID(), NewID()
	if len(a) != 8 || a == b {
		t.Errorf("NewID = %q, %q", a, b)
	}
}

func TestOpenNoRotateAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.log")
	if err := os.WriteFile(path, []byte("current-and-too-big\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l, c, err := Open(Options{Path: path, MaxSize: 5, NoRotate: true})
	if err != nil {
		t.Fatal(err)
	}
	l.Info("fresh")
	c.Close()
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Errorf("the file was moved aside")
	}
	if b, _ := os.ReadFile(path); !strings.HasPrefix(string(b), "current-and-too-big\n") || !strings.Contains(string(b), `"msg":"fresh"`) {
		t.Errorf("file = %q", b)
	}
}
