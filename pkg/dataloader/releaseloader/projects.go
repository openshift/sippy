package releaseloader

import (
	"fmt"
	"strings"

	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
)

func (ocp *OCPProject) GetName() string {
	return "OpenShift Container Platform (OCP)"
}

func (ocp *OCPProject) GetStreams() []string {
	return []string{"nightly", "ci"}
}

func (ocp *OCPProject) GetRcDomain(architecture string) (domain string) {
	return architecture + ".ocp.releases.ci.openshift.org"
}

func (ocp *OCPProject) IsProjectRelease(release v1.Release) bool {
	return release.Product == "OCP"
}

func (ocp *OCPProject) FullReleaseStream(release, stream, architecture string) string {
	releaseStream := fmt.Sprintf("%s.0-0.%s", release, stream)
	if architecture != "amd64" {
		releaseStream += "-" + architecture
	}
	return releaseStream
}

func (okd *OKDProject) GetName() string {
	return "Origin Kubernetes Distribution (OKD)"
}

func (okd *OKDProject) GetStreams() []string {
	return []string{"okd-scos"}
}

func (okd *OKDProject) GetRcDomain(architecture string) (domain string) {
	return architecture + ".origin.releases.ci.openshift.org"
}

func (okd *OKDProject) IsProjectRelease(release v1.Release) bool {
	return release.Product == "OKD"
}

func (okd *OKDProject) FullReleaseStream(release, stream, architecture string) string {
	if architecture != "amd64" {
		return "" // OKD only ever uses amd64 for now
	}
	return strings.Replace(release, "-okd", ".0-0.", 1) + stream
}
