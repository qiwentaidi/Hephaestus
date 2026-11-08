package ai

import "testing"

func TestNormalizePentestTaskReference(t *testing.T) {
	tests := []struct {
		name            string
		raw             string
		explicitVersion int
		wantID          string
		wantVersion     int
	}{
		{name: "canonical", raw: "task-1", wantID: "task-1"},
		{name: "legacy latest", raw: "task-1/v0", wantID: "task-1"},
		{name: "legacy version", raw: "task-1/v3", wantID: "task-1", wantVersion: 3},
		{name: "explicit wins", raw: "task-1/v3", explicitVersion: 2, wantID: "task-1", wantVersion: 2},
		{name: "non version suffix", raw: "task-1/value", wantID: "task-1/value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			id, version, err := normalizePentestTaskReference(test.raw, test.explicitVersion)
			if err != nil {
				t.Fatal(err)
			}
			if id != test.wantID || version != test.wantVersion {
				t.Fatalf("got %q/v%d, want %q/v%d", id, version, test.wantID, test.wantVersion)
			}
		})
	}
}
