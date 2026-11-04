package logging

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// Records keeps what the log file has: the same records, at the same
// level, with the attributes added by With and the call's own.
func TestRecordsKeepWhatTheFileHas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.log")
	recs := NewRecords(0, nil)
	level := new(slog.LevelVar)
	l, c, err := Open(Options{Path: path, Level: level, Records: recs})
	if err != nil {
		t.Fatal(err)
	}
	l = l.With(KeyRun, "r1")
	l.Debug("hidden")
	l.Info("merge done", KeyPR, "o/r#1", KeyStep, "merge")
	l.WithGroup("gh").Warn("slow", "ms", 1200, slog.Group("rate", "left", 10))
	level.Set(slog.LevelDebug)
	l.Debug("shown now")
	c.Close()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var file []string
	for sc := bufio.NewScanner(f); sc.Scan(); {
		var rec map[string]any
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatal(err)
		}
		file = append(file, fmt.Sprint(rec["level"], " ", rec["msg"]))
	}
	var mem []string
	for _, r := range recs.All() {
		mem = append(mem, fmt.Sprint(r.Level, " ", r.Msg))
	}
	if !slices.Equal(file, mem) {
		t.Errorf("file has %q, memory has %q", file, mem)
	}

	all := recs.All()
	if v, ok := all[0].Attr(KeyRun); !ok || v != "r1" {
		t.Errorf("With's attribute is missing: %+v", all[0])
	}
	if v, _ := all[0].Attr(KeyPR); v != "o/r#1" {
		t.Errorf("the call's attribute is missing: %+v", all[0])
	}
	if v, _ := all[1].Attr("gh.rate.left"); v != "10" {
		t.Errorf("groups are not flattened: %+v", all[1])
	}
}

// Records keeps the latest records up to its limit, timed by its clock,
// and says when one is added.
func TestRecordsKeepTheLatest(t *testing.T) {
	at := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	recs := NewRecords(3, func() time.Time { return at })
	l := Memory(recs, slog.LevelInfo)
	changed := recs.Changed()
	l.Info("m0")
	select {
	case <-changed:
	default:
		t.Fatal("Changed was not closed when a record was added")
	}
	for i := 1; i < 5; i++ {
		l.Info(fmt.Sprintf("m%d", i))
	}
	var got []string
	for _, r := range recs.All() {
		got = append(got, r.Msg)
		if !r.Time.Equal(at) {
			t.Errorf("record %s timed %v, not by the clock given", r.Msg, r.Time)
		}
	}
	if want := []string{"m2", "m3", "m4"}; !slices.Equal(got, want) {
		t.Errorf("kept %q, want %q", got, want)
	}
}
