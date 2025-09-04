package releaseloader

import (
	"fmt"
)

func (ocp *OCPProject) GetName() string {
	return "OpenShift Container Platform (OCP)"
}

func (ocp *OCPProject) GetStreams() []string {
	return []string{"nightly", "ci"}
}

func (ocp *OCPProject) ResolveRelease(release string) string {
	return release
}

func (ocp *OCPProject) BuildReleaseStreams(releases []string) []string {
	return buildReleaseStreams(releases, ocp.GetStreams())
}

func (ocp *OCPProject) BuildTagsURL(release, architecture string) string {
	return buildTagsURL(architecture, "ocp", buildReleaseName(release, architecture))
}

func (ocp *OCPProject) BuildDetailsURL(release, architecture, tag string) string {
	return buildDetailsURL(architecture, "ocp", buildReleaseName(release, architecture), tag)
}

func (okd *OKDProject) GetName() string {
	return "Origin Kubernetes Distribution (OKD)"
}

func (okd *OKDProject) GetStreams() []string {
	return []string{"okd-scos"}
}

func (okd *OKDProject) ResolveRelease(release string) string {
	// For origin, we need to add the -okd suffix to the release tag before saving it to the database ie. 4.15 -> 4.15-okd
	return fmt.Sprintf("%v%s", release, "-okd")
}

func (okd *OKDProject) BuildReleaseStreams(releases []string) []string {
	return buildReleaseStreams(releases, okd.GetStreams())
}

func (okd *OKDProject) BuildTagsURL(release, architecture string) string {
	return buildTagsURL(architecture, "origin", buildReleaseName(release, architecture))
}

func (okd *OKDProject) BuildDetailsURL(release, architecture, tag string) string {
	return buildDetailsURL(architecture, "origin", buildReleaseName(release, architecture), tag)
}

func buildReleaseStreams(releases []string, streams []string) []string {
	releaseStreams := make([]string, 0, len(releases)*len(streams))
	for _, release := range releases {
		for _, stream := range streams {
			releaseStreams = append(releaseStreams, fmt.Sprintf("%s.0-0.%s", release, stream))
		}
	}
	return releaseStreams
}

func buildReleaseName(release, architecture string) string {
	if architecture != "amd64" {
		release += "-" + architecture
	}
	return release
}

func buildTagsURL(arch, platform, release string) string {
	return fmt.Sprintf("%s/%s/tags", buildReleaseURL(arch, platform), release)
}

func buildDetailsURL(arch, platform, release, tag string) string {
	return fmt.Sprintf("%s/%s/release/%s", buildReleaseURL(arch, platform), release, tag)
}

func buildReleaseURL(arch, platform string) string {
	return fmt.Sprintf("https://%s.%s.releases.ci.openshift.org/api/v1/releasestream", arch, platform)
}
