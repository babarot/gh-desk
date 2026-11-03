package playground

import (
	"encoding/json"
	"flag"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "rewrite docs/site/data/scenarios.json")

// The docs embed the playground by a scenario's name, and their build
// stops on a name that is not in this file, so a scenario renamed or
// removed is caught there, not by a reader whose screen never plays.

const namesPath = "../../../docs/site/data/scenarios.json"

func scenarioNames() []byte {
	var names []string
	for _, s := range Scenarios() {
		names = append(names, s.Name)
	}
	b, _ := json.MarshalIndent(names, "", "  ")
	return append(b, '\n')
}

func TestScenarioNamesAreUpToDate(t *testing.T) {
	want := scenarioNames()
	if *update {
		if err := os.WriteFile(namesPath, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(namesPath)
	if err != nil || string(got) != string(want) {
		t.Errorf("%s is not the names of the scenarios; run go test ./demo/internal/playground -run TestScenarioNamesAreUpToDate -update", namesPath)
	}
}
