package releaseloader

import (
	"fmt"
	"strings"
)

func (ocp *OCPRelease) GetName() string {
	return "OpenShift Container Platform (OCP)"
}

func (ocp *OCPRelease) GetAlias() string {
	return "ocp"
}

func (ocp *OCPRelease) GetStreams() []string {
	return []string{"nightly", "ci"}
}

func (ocp *OCPRelease) BuildReleaseStreams(releases []string) []string {
	return buildReleaseStreams(releases, ocp.GetStreams())
}

func (ocp *OCPRelease) BuildTagsURL(release, architecture string) string {
	return buildTagsURL(architecture, ocp.GetAlias(), buildReleaseName(release, architecture))
}

func (ocp *OCPRelease) BuildDetailsURL(release, architecture, tag string) string {
	return buildDetailsURL(architecture, ocp.GetAlias(), buildReleaseName(release, architecture), tag)
}

func (okd *OKDRelease) GetName() string {
	return "Origin Kubernetes Distribution (OKD)"
}

func (okd *OKDRelease) GetAlias() string {
	return "origin"
}

func (okd *OKDRelease) GetStreams() []string {
	return []string{"okd-scos"}
}

func (okd *OKDRelease) BuildReleaseStreams(releases []string) []string {
	return buildReleaseStreams(releases, okd.GetStreams())
}

func (okd *OKDRelease) BuildTagsURL(release, architecture string) string {
	return buildTagsURL(architecture, okd.GetAlias(), buildReleaseName(release, architecture))
}

func (okd *OKDRelease) BuildDetailsURL(release, architecture, tag string) string {
	return buildDetailsURL(architecture, okd.GetAlias(), buildReleaseName(release, architecture), tag)
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

func GetPlatformReleases(platform string) ([]PlatformRelease, error) {
	switch strings.ToLower(platform) {
	case "ocp":
		return []PlatformRelease{&OCPRelease{}}, nil
	case "okd":
		return []PlatformRelease{&OKDRelease{}}, nil
	case "all":
		return []PlatformRelease{&OCPRelease{}, &OKDRelease{}}, nil
	default:
		return nil, fmt.Errorf("invalid platform: %s", platform)
	}
}
