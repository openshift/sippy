package testgridhelpers

import (
	"reflect"
	"testing"
)

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

func TestURLForJob(t *testing.T) {
	type args struct {
		dashboard string
		jobName   string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "simple",
			args: args{
				dashboard: "redhat-openshift-ocp-release-4.4-informing",
				jobName:   "release-openshift-origin-installer-e2e-azure-compact-4.4",
			},
			want: "https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing#release-openshift-origin-installer-e2e-azure-compact-4.4&grid=old",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := URLForJob(tt.args.dashboard, tt.args.jobName); !reflect.DeepEqual(got.String(), tt.want) {
				t.Errorf("URLForJob() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestURLForJobDetails(t *testing.T) {
	type args struct {
		dashboard string
		jobName   string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "simple",
			args: args{
				dashboard: "redhat-openshift-ocp-release-4.4-informing",
				jobName:   "release-openshift-origin-installer-e2e-azure-compact-4.4",
			},
			want: "https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing/table?grid=old&show-stale-tests=&tab=release-openshift-origin-installer-e2e-azure-compact-4.4",
		}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := URLForJobDetails(tt.args.dashboard, tt.args.jobName); !reflect.DeepEqual(got.String(), tt.want) {
				t.Errorf("URLForJobDetails() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestURLForJobSummary(t *testing.T) {
	type args struct {
		dashboard string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "simple",
			args: args{
				dashboard: "redhat-openshift-ocp-release-4.4-informing",
			},
			want: "https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing/summary",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := URLForJobSummary(tt.args.dashboard); !reflect.DeepEqual(got.String(), tt.want) {
				t.Errorf("URLForJobSummary() = %v, want %v", got, tt.want)
			}
		})
	}
}
