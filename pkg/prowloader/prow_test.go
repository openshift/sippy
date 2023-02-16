package prowloader

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDateTimeNameComparisons(t *testing.T) {
	tests := []struct {
		name           string
		names          []string
		expectedResult string
	}{
		{
			name: "standard",
			names: []string{`https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/pr-logs/pull/27731/pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade/1626951434970861568/artifacts/e2e-aws-ovn-upgrade/openshift-e2e-test/artifacts/junit/test-failures-summary_20230218-180228.json`,
				`https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/pr-logs/pull/27731/pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade/1626951434970861568/artifacts/e2e-aws-ovn-upgrade/openshift-e2e-test/artifacts/junit/test-failures-summary_20230218-153052.json`},
			expectedResult: `https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/pr-logs/pull/27731/pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade/1626951434970861568/artifacts/e2e-aws-ovn-upgrade/openshift-e2e-test/artifacts/junit/test-failures-summary_20230218-180228.json`,
		},
		{
			name: "reversed",
			names: []string{`https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/pr-logs/pull/27731/pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade/1626951434970861568/artifacts/e2e-aws-ovn-upgrade/openshift-e2e-test/artifacts/junit/test-failures-summary_20230218-153052.json`,
				`https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/pr-logs/pull/27731/pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade/1626951434970861568/artifacts/e2e-aws-ovn-upgrade/openshift-e2e-test/artifacts/junit/test-failures-summary_20230218-180228.json`},
			expectedResult: `https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/pr-logs/pull/27731/pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade/1626951434970861568/artifacts/e2e-aws-ovn-upgrade/openshift-e2e-test/artifacts/junit/test-failures-summary_20230218-180228.json`,
		},
		{
			name: "older date",
			names: []string{`https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/pr-logs/pull/27731/pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade/1626951434970861568/artifacts/e2e-aws-ovn-upgrade/openshift-e2e-test/artifacts/junit/test-failures-summary_20230219-153052.json`,
				`https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/pr-logs/pull/27731/pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade/1626951434970861568/artifacts/e2e-aws-ovn-upgrade/openshift-e2e-test/artifacts/junit/test-failures-summary_20230218-180228.json`},
			expectedResult: `https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/pr-logs/pull/27731/pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade/1626951434970861568/artifacts/e2e-aws-ovn-upgrade/openshift-e2e-test/artifacts/junit/test-failures-summary_20230219-153052.json`,
		},
		{
			name: "invalid date",
			names: []string{`https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/pr-logs/pull/27731/pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade/1626951434970861568/artifacts/e2e-aws-ovn-upgrade/openshift-e2e-test/artifacts/junit/test-failures-summary_20230219153052.json`,
				`https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/pr-logs/pull/27731/pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade/1626951434970861568/artifacts/e2e-aws-ovn-upgrade/openshift-e2e-test/artifacts/junit/test-failures-summary_20230218-180228.json`},
			expectedResult: `https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/pr-logs/pull/27731/pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade/1626951434970861568/artifacts/e2e-aws-ovn-upgrade/openshift-e2e-test/artifacts/junit/test-failures-summary_20230218-180228.json`,
		},
		{
			name: "invalid dates",
			names: []string{`https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/pr-logs/pull/27731/pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade/1626951434970861568/artifacts/e2e-aws-ovn-upgrade/openshift-e2e-test/artifacts/junit/test-failures-summary_20230219153052.json`,
				`https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/pr-logs/pull/27731/pull-ci-openshift-origin-master-e2e-aws-ovn-upgrade/1626951434970861568/artifacts/e2e-aws-ovn-upgrade/openshift-e2e-test/artifacts/junit/test-failures-summary.json`},
			expectedResult: ``,
		},
		{
			name:           "empty names",
			names:          []string{},
			expectedResult: ``,
		},
		{
			name:           "no names",
			expectedResult: ``,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedResult, findMostRecentDateTimeMatch(tt.names), "Test: %s failed mostRecentDateTimeMatch", tt.name)
		})
	}
}
