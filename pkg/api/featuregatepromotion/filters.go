package featuregatepromotion

import (
	"fmt"

	"github.com/openshift/sippy/pkg/filter"
)

// GateTestFilter returns the canonical filter for annotation-based tests
// belonging to a feature gate. This is the authoritative definition used both
// to generate HATEOAS links and to query data for promotion evaluation.
func GateTestFilter(featureGate string) filter.Filter {
	return filter.Filter{
		Items: []filter.FilterItem{
			{Field: "name", Operator: filter.OperatorContains, Value: fmt.Sprintf("FeatureGate:%s]", featureGate)},
			{Field: "variants", Not: true, Operator: filter.OperatorHasEntry, Value: "never-stable"},
			{Field: "variants", Not: true, Operator: filter.OperatorHasEntry, Value: "aggregated"},
		},
		LinkOperator: filter.LinkOperatorAnd,
	}
}

// InstallTestFilter returns the canonical filter for install capability tests
// belonging to an Install feature gate. Only applicable when the feature gate
// name contains "Install". This is the authoritative definition used both
// to generate HATEOAS links and to query data for promotion evaluation.
func InstallTestFilter(featureGate string) filter.Filter {
	return filter.Filter{
		Items: []filter.FilterItem{
			{Field: "name", Operator: filter.OperatorContains, Value: "install should succeed"},
			{Field: "variants", Operator: filter.OperatorContains, Value: fmt.Sprintf("Capability:%s", featureGate)},
			{Field: "variants", Not: true, Operator: filter.OperatorHasEntry, Value: "never-stable"},
			{Field: "variants", Not: true, Operator: filter.OperatorHasEntry, Value: "aggregated"},
		},
		LinkOperator: filter.LinkOperatorAnd,
	}
}

// CapabilityRegressionsFilter returns the filter for identifying tests with
// low pass rates on jobs owned by this feature gate's capability.
func CapabilityRegressionsFilter(featureGate string) filter.Filter {
	return filter.Filter{
		Items: []filter.FilterItem{
			{Field: "variants", Not: true, Operator: filter.OperatorHasEntry, Value: "never-stable"},
			{Field: "variants", Not: true, Operator: filter.OperatorHasEntry, Value: "aggregated"},
			{Field: "variants", Operator: filter.OperatorHasEntry, Value: fmt.Sprintf("Capability:%s", featureGate)},
			{Field: "current_working_percentage", Operator: filter.OperatorArithmeticLessThan, Value: "92"},
			{Field: "current_runs", Operator: filter.OperatorArithmeticGreaterThanOrEquals, Value: "0"},
			{Field: "name", Not: true, Operator: filter.OperatorContains, Value: "install should succeed"},
			{Field: "name", Not: true, Operator: filter.OperatorContains, Value: "openshift-tests should work"},
			{Field: "name", Not: true, Operator: filter.OperatorContains, Value: "infrastructure should work"},
		},
		LinkOperator: filter.LinkOperatorAnd,
	}
}
