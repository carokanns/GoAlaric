package main

import "testing"

func TestPipelineAndCampaignResolveSameCandidatePolicy(t *testing.T) {
	cases := []struct {
		name        string
		changeClass string
		override    *bool
		wantClass   string
		policy      string
	}{
		{name: "implementation", changeClass: "implementation", wantClass: "implementation", policy: comparisonPolicyExactEquivalence},
		{name: "eval", changeClass: "eval", wantClass: "eval", policy: comparisonPolicyBehavioral},
		{name: "search", changeClass: "search", wantClass: "search", policy: comparisonPolicyBehavioral},
		{name: "correctness", changeClass: "correctness", wantClass: "correctness", policy: comparisonPolicyBehavioral},
		{name: "mixed", changeClass: "mixed", wantClass: "mixed", policy: comparisonPolicyBehavioral},
		{name: "legacy exact override", override: boolPointer(true), wantClass: "implementation", policy: comparisonPolicyExactEquivalence},
		{name: "legacy behavioral override", override: boolPointer(false), wantClass: "mixed", policy: comparisonPolicyBehavioral},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pipeline, err := pipelineCandidateDefinition(tc.changeClass, tc.override)
			if err != nil {
				t.Fatal(err)
			}
			campaign, err := campaignCandidateDefinition(tc.changeClass, tc.override)
			if err != nil {
				t.Fatal(err)
			}
			if pipeline != campaign {
				t.Fatalf("pipeline=%+v campaign=%+v", pipeline, campaign)
			}
			if pipeline.ChangeClass != tc.wantClass || pipeline.ComparisonPolicy != tc.policy {
				t.Fatalf("definition=%+v, want class=%q policy=%q", pipeline, tc.wantClass, tc.policy)
			}
		})
	}
}

func TestCandidatePolicyRejectsContradictorySemanticOverride(t *testing.T) {
	if _, err := resolveCandidateDefinition("search", boolPointer(true)); err == nil {
		t.Fatal("search class accepted an exact-equivalence override")
	}
	if _, err := resolveCandidateDefinition("implementation", boolPointer(false)); err == nil {
		t.Fatal("implementation class accepted a behavioral override")
	}
}

func TestCandidatePolicyRejectsUnknownChangeClass(t *testing.T) {
	if _, err := resolveCandidateDefinition("unknown", nil); err == nil {
		t.Fatal("unknown change class was accepted")
	}
}

func boolPointer(value bool) *bool {
	return &value
}
