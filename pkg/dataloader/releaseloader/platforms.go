package releaseloader

import (
	"fmt"
	"strings"
)

// PlatformRelease represents a platform that releases are built for (OCP, OKD, etc)
type PlatformRelease interface {
	// GetName returns the human-readable name of the platform
	GetName() string

	// GetAlias returns a short alias for the platform
	GetAlias() string

	// GetStreams returns the available release streams for this platform
	GetStreams() []string

	// BuildReleaseStreams builds the full stream names for given releases
	BuildReleaseStreams(releases []string) []string

	// BuildTagsURL builds the URL to fetch release tags for a specific release and architecture
	BuildTagsURL(release, architecture string) string

	// BuildDetailsURL builds the URL to fetch release details for a specific tag
	BuildDetailsURL(release, architecture, tag string) string
}

// OCPRelease implements PlatformRelease for OpenShift Container Platform
type OCPRelease struct{}

func (ocp *OCPRelease) GetName() string {
	return "OpenShift Container Platform"
}

func (ocp *OCPRelease) GetAlias() string {
	return "ocp"
}

func (ocp *OCPRelease) GetStreams() []string {
	return []string{"nightly", "ci"}
}

func (ocp *OCPRelease) BuildReleaseStreams(releases []string) []string {
	releaseStreams := make([]string, 0)
	for _, release := range releases {
		for _, stream := range ocp.GetStreams() {
			releaseStreams = append(releaseStreams, fmt.Sprintf("%s.0-0.%s", release, stream))
		}
	}
	return releaseStreams
}

func (ocp *OCPRelease) BuildTagsURL(release, architecture string) string {
	releaseName := release
	if architecture != "amd64" {
		releaseName += "-" + architecture
	}
	return fmt.Sprintf("https://%s.ocp.releases.ci.openshift.org/api/v1/releasestream/%s/tags", architecture, releaseName)
}

func (ocp *OCPRelease) BuildDetailsURL(release, architecture, tag string) string {
	releaseName := release
	if architecture != "amd64" {
		releaseName += "-" + architecture
	}
	return fmt.Sprintf("https://%s.ocp.releases.ci.openshift.org/api/v1/releasestream/%s/release/%s", architecture, releaseName, tag)
}

// OKDRelease implements PlatformRelease for OKD (Origin Kubernetes Distribution)
type OKDRelease struct{}

func (okd *OKDRelease) GetName() string {
	return "OKD"
}

func (okd *OKDRelease) GetAlias() string {
	return "okd"
}

func (okd *OKDRelease) GetStreams() []string {
	return []string{"okd-scos"} //TODO: Add other streams
}

func (okd *OKDRelease) BuildReleaseStreams(releases []string) []string {
	releaseStreams := make([]string, 0)
	for _, release := range releases {
		for range okd.GetStreams() {
			// OKD uses a different naming pattern
			releaseStreams = append(releaseStreams, fmt.Sprintf("%s-0.okd", release))
		}
	}
	return releaseStreams
}

func (okd *OKDRelease) BuildTagsURL(release, architecture string) string {
	// OKD uses the origin release controller
	releaseName := release
	if architecture != "amd64" {
		releaseName += "-" + architecture
	}
	return fmt.Sprintf("https://origin-release.apps.ci.l2s4.p1.openshiftapps.com/api/v1/releasestream/%s/tags", releaseName)
}

func (okd *OKDRelease) BuildDetailsURL(release, architecture, tag string) string {
	// OKD uses the origin release controller
	releaseName := release
	if architecture != "amd64" {
		releaseName += "-" + architecture
	}
	return fmt.Sprintf("https://origin-release.apps.ci.l2s4.p1.openshiftapps.com/api/v1/releasestream/%s/release/%s", releaseName, tag)
}

// GetPlatformReleases returns platform release implementations based on the platform string
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
