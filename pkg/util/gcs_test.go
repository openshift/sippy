package util

import "testing"

func TestGCSObjectPathFromURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "legacy bucket",
			url:  "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.17-e2e-aws-ovn/1234567890",
			want: "logs/periodic-ci-openshift-release-master-nightly-4.17-e2e-aws-ovn/1234567890",
		},
		{
			name: "public bucket",
			url:  "https://prow.ci.openshift.org/view/gs/test-platform-results-public/logs/periodic-ci-openshift-release-master-nightly-4.17-e2e-aws-ovn/1234567890",
			want: "logs/periodic-ci-openshift-release-master-nightly-4.17-e2e-aws-ovn/1234567890",
		},
		{
			name: "origin-ci-test bucket",
			url:  "https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/periodic-ci-openshift-release-master-ci-4.7-e2e-aws-serial/1537826070202421248",
			want: "logs/periodic-ci-openshift-release-master-ci-4.7-e2e-aws-serial/1537826070202421248",
		},
		{
			name: "trailing slash preserved",
			url:  "https://prow.ci.openshift.org/view/gs/test-platform-results-public/logs/some-job/999/",
			want: "logs/some-job/999/",
		},
		{
			name: "pr-logs path",
			url:  "https://prow.ci.openshift.org/view/gs/test-platform-results-public/pr-logs/pull/openshift_sippy/1/pull-ci-openshift-sippy-main-images/123",
			want: "pr-logs/pull/openshift_sippy/1/pull-ci-openshift-sippy-main-images/123",
		},
		{
			name: "example.com /gs/ fixture",
			url:  "https://example.com/gs/test-platform-results-public/logs/job/1",
			want: "logs/job/1",
		},
		{
			name: "non-prow /gs/ after unrelated prefix",
			url:  "x/gs/other/logs/job/1",
			want: "",
		},
		{
			name: "non-prow /gs/ nested in path",
			url:  "https://example.com/x/gs/other/logs/job/1",
			want: "",
		},
		{
			name: "no gs path",
			url:  "https://example.com/some/other/path",
			want: "",
		},
		{
			name: "empty URL",
			url:  "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GCSObjectPathFromURL(tt.url); got != tt.want {
				t.Errorf("GCSObjectPathFromURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGCSObjectPrefixFromURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "adds trailing slash",
			url:  "https://prow.ci.openshift.org/view/gs/test-platform-results-public/logs/some-job/999",
			want: "logs/some-job/999/",
		},
		{
			name: "keeps existing trailing slash",
			url:  "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/some-job/999/",
			want: "logs/some-job/999/",
		},
		{
			name: "unrecognized URL",
			url:  "https://example.com/some/other/path",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GCSObjectPrefixFromURL(tt.url); got != tt.want {
				t.Errorf("GCSObjectPrefixFromURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
