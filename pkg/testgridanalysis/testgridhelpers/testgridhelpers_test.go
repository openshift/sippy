package testgridhelpers

import "testing"

func Test_normalizeURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{
			"https://testgrid.k8s.io-redhat-openshift-ocp-release-4.4-informing?table-&show-stale-tests=&tab=release-openshift-ocp-e2e-aws-scaleup-rhel7-4.4",
			"https---testgrid.k8s.io-redhat-openshift-ocp-release-4.4-informing-table-&show-stale-tests=&tab=release-openshift-ocp-e2e-aws-scaleup-rhel7-4.4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := normalizeURL(tt.url); got != tt.want {
				t.Errorf("normalizeURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
