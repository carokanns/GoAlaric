package main

import (
	"flag"
	"fmt"
	"strings"
)

const (
	changeClassImplementation = "implementation"
	changeClassEval           = "eval"
	changeClassSearch         = "search"
	changeClassCorrectness    = "correctness"
	changeClassMixed          = "mixed"

	comparisonPolicyExactEquivalence = "exact-equivalence"
	comparisonPolicyBehavioral       = "behavioral"
)

type candidateDefinition struct {
	ChangeClass      string
	ComparisonPolicy string
}

// resolveCandidateDefinition is the sole policy source for pipeline and
// campaign-init. Exact equivalence means identical fixed-depth bestmove, score
// and node count; behavioral candidates report those differences instead.
func resolveCandidateDefinition(changeClass string, semanticOverride *bool) (candidateDefinition, error) {
	changeClass = strings.ToLower(strings.TrimSpace(changeClass))
	if changeClass == "" {
		// Older invocations had only --semantic-preserving=false. Preserve their
		// behavioral policy while marking their unspecified class as mixed.
		if semanticOverride != nil && !*semanticOverride {
			return candidateDefinition{ChangeClass: changeClassMixed, ComparisonPolicy: comparisonPolicyBehavioral}, nil
		}
		changeClass = changeClassImplementation
	}

	exact := false
	switch changeClass {
	case changeClassImplementation:
		exact = true
	case changeClassEval, changeClassSearch, changeClassCorrectness, changeClassMixed:
		// These classes intentionally permit changed fixed-depth search results.
	default:
		return candidateDefinition{}, fmt.Errorf("invalid change class %q; use implementation, eval, search, correctness or mixed", changeClass)
	}
	if semanticOverride != nil && *semanticOverride != exact {
		return candidateDefinition{}, fmt.Errorf("--semantic-preserving=%t contradicts change class %q, whose policy is %s", *semanticOverride, changeClass, comparisonPolicyForExact(exact))
	}
	return candidateDefinition{ChangeClass: changeClass, ComparisonPolicy: comparisonPolicyForExact(exact)}, nil
}

func comparisonPolicyForExact(exact bool) string {
	if exact {
		return comparisonPolicyExactEquivalence
	}
	return comparisonPolicyBehavioral
}

func policyRequiresExactEquivalence(policy string) bool {
	return policy == comparisonPolicyExactEquivalence
}

func boolFlagOverride(fs *flag.FlagSet, name string, value bool) *bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	if !found {
		return nil
	}
	return &value
}

func pipelineCandidateDefinition(changeClass string, semanticOverride *bool) (candidateDefinition, error) {
	return resolveCandidateDefinition(changeClass, semanticOverride)
}

func campaignCandidateDefinition(changeClass string, semanticOverride *bool) (candidateDefinition, error) {
	return resolveCandidateDefinition(changeClass, semanticOverride)
}
