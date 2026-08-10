package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestValidateResult(t *testing.T) {
	minimum := 1
	tests := []struct {
		name     string
		test     testCase
		bestMove string
		score    string
		wantErr  bool
	}{
		{
			name:     "exact draw score",
			test:     testCase{AcceptableMoves: []string{"a1b1"}, ExpectedScore: "cp -5"},
			bestMove: "a1b1",
			score:    "cp -5",
		},
		{
			name:     "winning score",
			test:     testCase{AcceptableMoves: []string{"a1b1"}, MinimumScoreCP: &minimum},
			bestMove: "a1b1",
			score:    "cp 200",
		},
		{
			name:     "wrong move",
			test:     testCase{AcceptableMoves: []string{"a1b1"}},
			bestMove: "a1a2",
			wantErr:  true,
		},
		{
			name:     "wrong exact score",
			test:     testCase{AcceptableMoves: []string{"a1b1"}, ExpectedScore: "cp -5"},
			bestMove: "a1b1",
			score:    "cp 0",
			wantErr:  true,
		},
		{
			name:     "score below minimum",
			test:     testCase{AcceptableMoves: []string{"a1b1"}, MinimumScoreCP: &minimum},
			bestMove: "a1b1",
			score:    "cp 0",
			wantErr:  true,
		},
		{
			name:     "mate score is not cp",
			test:     testCase{AcceptableMoves: []string{"a1b1"}, MinimumScoreCP: &minimum},
			bestMove: "a1b1",
			score:    "mate 3",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResult(tt.test, tt.bestMove, tt.score)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateResult() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGeneratedSuite(t *testing.T) {
	data, err := os.ReadFile("../../scripts/material_draw_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases suite
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if cases.SchemaVersion != 1 {
		t.Fatalf("schema version = %d, want 1", cases.SchemaVersion)
	}
	if len(cases.Cases) != 16 {
		t.Fatalf("cases = %d, want 16", len(cases.Cases))
	}

	kinds := map[string]int{}
	ids := map[string]bool{}
	for _, test := range cases.Cases {
		if test.ID == "" || ids[test.ID] {
			t.Fatalf("missing or duplicate id %q", test.ID)
		}
		ids[test.ID] = true
		kinds[test.Kind]++
		if test.FEN == "" || len(test.AcceptableMoves) == 0 || len(test.ForbiddenMoves) == 0 {
			t.Fatalf("incomplete case %q", test.ID)
		}
		switch test.Kind {
		case "force_material_draw":
			if test.ExpectedScore != "cp -5" || test.MinimumScoreCP != nil {
				t.Fatalf("invalid draw expectation in %q", test.ID)
			}
		case "avoid_material_draw":
			if test.ExpectedScore != "" || test.MinimumScoreCP == nil || *test.MinimumScoreCP != 1 {
				t.Fatalf("invalid win expectation in %q", test.ID)
			}
		default:
			t.Fatalf("unknown kind %q", test.Kind)
		}
	}
	if kinds["force_material_draw"] != 8 || kinds["avoid_material_draw"] != 8 {
		t.Fatalf("case distribution = %#v, want 8 force and 8 avoid", kinds)
	}
}
