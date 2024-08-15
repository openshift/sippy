package regressionallowances

import (
	"encoding/json"
	"fmt"
	"net/url"

	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/componentreadiness/resolvedissues"

	log "github.com/sirupsen/logrus"
)

type IntentionalRegression struct {
	JiraComponent             string
	TestID                    string
	TestName                  string
	Variant                   crtype.ColumnIdentification
	PreviousPassPercentage    int
	PreviousSampleSize        int
	RegressedPassPercentage   int
	RegressedSampleSize       int
	JiraBug                   string
	ReasonToAllowInsteadOfFix string
}

func IntentionalRegressionFor(releaseString string, variant crtype.ColumnIdentification, testID string) *IntentionalRegression {
	var targetMap map[string]IntentionalRegression
	switch release(releaseString) {
	case release415:
		targetMap = regressions415
	default:
		return nil
	}

	inKey := keyFor(testID, variant)
	if t, ok := targetMap[inKey]; ok {
		log.Debugf("found approved regression: %+v", t)
		return &t
	}
	return nil
}

type release string

var (
	release415 release = "4.15"
)

var (
	regressions415 = map[string]IntentionalRegression{}
)

type regressionKey struct {
	TestID  string
	Variant crtype.ColumnIdentification
}

func keyFor(testID string, variant crtype.ColumnIdentification) string {
	key := regressionKey{
		TestID: testID,
		Variant: crtype.ColumnIdentification{
			Variants: variant.Variants,
		},
	}
	k, err := json.Marshal(key)
	if err != nil {
		log.WithError(err).Errorf("error marshalling regressionKey")
	}
	return string(k)
}

func mustAddIntentionalRegression(release release, in IntentionalRegression) {
	if err := addIntentionalRegression(release, in); err != nil {
		panic(err)
	}
}

func addIntentionalRegression(release release, in IntentionalRegression) error {
	if len(in.JiraComponent) == 0 {
		return fmt.Errorf("jiraComponent must be specified")
	}
	if len(in.TestID) == 0 {
		return fmt.Errorf("testID must be specified")
	}
	if len(in.TestName) == 0 {
		return fmt.Errorf("testName must be specified")
	}
	if in.PreviousPassPercentage <= 0 {
		return fmt.Errorf("previousPassPercentage must be specified")
	}
	if in.RegressedPassPercentage <= 0 {
		return fmt.Errorf("regressedPassPercentage must be specified")
	}
	if in.PreviousSampleSize <= 0 {
		return fmt.Errorf("previousSampleSize must be specified")
	}
	if in.RegressedSampleSize <= 0 {
		return fmt.Errorf("regressedSampleSize must be specified")
	}
	if len(in.ReasonToAllowInsteadOfFix) == 0 {
		return fmt.Errorf("reasonToAllowInsteadOfFix must be specified")
	}
	if _, err := url.ParseRequestURI(in.JiraBug); err != nil {
		return fmt.Errorf("jiraBug must be a valid URL")
	}
	for _, v := range resolvedissues.TriageMatchVariants.List() {
		if _, ok := in.Variant.Variants[v]; !ok {
			return fmt.Errorf("%s must be specified", v)
		}
	}

	var targetMap map[string]IntentionalRegression

	switch release {
	case release415:
		targetMap = regressions415
	default:
		return fmt.Errorf("unknown release: %q", release)
	}

	inKey := keyFor(in.TestID, in.Variant)
	if _, ok := targetMap[inKey]; ok {
		return fmt.Errorf("test %q was already added", in.TestID)
	}

	targetMap[inKey] = in

	return nil
}
