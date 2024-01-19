package regressionallowances

import (
	"fmt"

	"github.com/openshift/sippy/pkg/apis/api"
)

type IntentionalRegression struct {
	JiraComponent             string
	TestID                    string
	TestName                  string
	Variant                   api.ComponentReportColumnIdentification
	PreviousPassPercentage    int
	PreviousSampleSize        int
	RegressedPassPercentage   int
	RegressedSampleSize       int
	ReasonToAllowInsteadOfFix string
}

func IntentionalRegressionFor(releaseString string, variant api.ComponentReportColumnIdentification, testID string) *IntentionalRegression {
	var targetMap map[regressionKey]IntentionalRegression
	switch release(releaseString) {
	case release415:
		targetMap = regressions_415
	default:
		return nil
	}

	inKey := keyFor(testID, variant)
	t := targetMap[inKey]
	return &t
}

type release string

var (
	release415 release = "4.15"
)

var (
	regressions_415 = map[regressionKey]IntentionalRegression{}
)

type regressionKey struct {
	testID  string
	variant api.ComponentReportColumnIdentification
}

func keyFor(testID string, variant api.ComponentReportColumnIdentification) regressionKey {
	return regressionKey{
		testID: testID,
		variant: api.ComponentReportColumnIdentification{
			Network:  variant.Network,
			Upgrade:  variant.Upgrade,
			Arch:     variant.Arch,
			Platform: variant.Platform,
		},
	}
}

func mustAddIntentionalRegression(release release, in IntentionalRegression) {
	if err := addIntentionalRegression(release, in); err != nil {
		panic(err)
	}
}

func addIntentionalRegression(release release, in IntentionalRegression) error {
	if len(in.JiraComponent) == 0 {
		return fmt.Errorf("JiraComponent must be specified.")
	}
	if len(in.TestID) == 0 {
		return fmt.Errorf("TestID must be specified.")
	}
	if len(in.TestName) == 0 {
		return fmt.Errorf("TestName must be specified.")
	}
	if in.PreviousPassPercentage <= 0 {
		return fmt.Errorf("PreviousPassPercentage must be specified.")
	}
	if in.RegressedPassPercentage <= 0 {
		return fmt.Errorf("RegressedPassPercentage must be specified.")
	}
	if in.PreviousSampleSize <= 0 {
		return fmt.Errorf("PreviousSampleSize must be specified.")
	}
	if in.RegressedSampleSize <= 0 {
		return fmt.Errorf("RegressedSampleSize must be specified.")
	}
	if len(in.ReasonToAllowInsteadOfFix) == 0 {
		return fmt.Errorf("ReasonToAllowInsteadOfFix must be specified.")
	}
	if len(in.Variant.Network) == 0 {
		return fmt.Errorf("Network must be specified.")
	}
	if len(in.Variant.Arch) == 0 {
		return fmt.Errorf("Arch must be specified.")
	}
	if len(in.Variant.Platform) == 0 {
		return fmt.Errorf("Platform must be specified.")
	}
	if len(in.Variant.Upgrade) == 0 {
		return fmt.Errorf("Upgrade must be specified.")
	}

	var targetMap map[regressionKey]IntentionalRegression

	switch release {
	case release415:
		targetMap = regressions_415
	default:
		fmt.Errorf("unknown release: %q", release)
	}

	inKey := keyFor(in.TestID, in.Variant)
	if _, ok := targetMap[inKey]; ok {
		return fmt.Errorf("test %q was already added", in.TestID)
	}

	targetMap[inKey] = in

	return nil
}
