package releasesync

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"k8s.io/klog"
)

type releaseSyncOptions struct {
	db            *db.DB
	httpClient    *http.Client
	releases      []string
	architectures []string
}

func Import(dbc *db.DB, releases, architectures []string) error {
	o := releaseSyncOptions{
		db:            dbc,
		releases:      releases,
		architectures: architectures,
		httpClient:    &http.Client{Timeout: 60 * time.Second},
	}

	return o.Run()
}

func (r *releaseSyncOptions) Run() error {
	for _, release := range r.releases {

		klog.V(2).Infof("Fetching release %s from release controller...\n", release)
		allTags := r.fetchReleaseTags(release)

		for _, tags := range allTags {
			for _, tag := range tags.Tags {
				c := int64(0)
				r.db.DB.Table("release_tags").Where(`"release_tag" = ?`, tag.Name).Count(&c)
				if c > 0 {
					continue
				}

				klog.V(2).Infof("Fetching tag %s from release controller...\n", tag.Name)
				releaseDetails := r.fetchReleaseDetails(tags.Architecture, release, tag)
				releaseTag := releaseDetailsToDB(tags.Architecture, tag, releaseDetails)
				// We skip releases that aren't fully baked (i.e. all jobs run and changelog calculated)
				if releaseTag == nil || (releaseTag.Phase != "Accepted" && releaseTag.Phase != "Rejected") {
					continue
				}

				// PR is many-to-many, find the existing relation. TODO: There must be a more clever way to do this...
				for i, pr := range releaseTag.PullRequests {
					existingPR := models.ReleasePullRequest{}
					result := r.db.DB.Table("release_pull_requests").Where("url = ?", pr.URL).Where("name = ?", pr.Name).First(&existingPR)
					if result.Error == nil {
						releaseTag.PullRequests[i] = existingPR
					}
				}

				if err := r.db.DB.Create(&releaseTag).Error; err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (r *releaseSyncOptions) fetchReleaseDetails(architecture, release string, tag ReleaseTag) ReleaseDetails {
	releaseDetails := ReleaseDetails{}
	releaseName := release
	if architecture != "amd64" {
		releaseName += "-" + architecture
	}

	rcURL := fmt.Sprintf("https://%s.ocp.releases.ci.openshift.org/api/v1/releasestream/%s/release/%s", architecture, releaseName, tag.Name)

	resp, err := r.httpClient.Get(rcURL)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&releaseDetails); err != nil {
		panic(err)
	}

	return releaseDetails
}

func (r *releaseSyncOptions) fetchReleaseTags(release string) []ReleaseTags {
	allTags := make([]ReleaseTags, 0)
	for _, arch := range r.architectures {
		tags := ReleaseTags{
			Architecture: arch,
		}
		releaseName := release
		if arch != "amd64" {
			releaseName += "-" + arch
		}
		uri := fmt.Sprintf("https://%s.ocp.releases.ci.openshift.org/api/v1/releasestream/%s/tags", arch, releaseName)
		resp, err := r.httpClient.Get(uri)
		if err != nil {
			panic(err)
		}
		if resp.StatusCode != http.StatusOK {
			klog.Errorf("release controller returned non-200 error code for %s: %d %s", uri, resp.StatusCode, resp.Status)
			continue
		}

		if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
			klog.Errorf("couldn't decode json: %v", err)
			resp.Body.Close()
			continue
		}
		resp.Body.Close()
		allTags = append(allTags, tags)
	}
	return allTags
}

func releaseDetailsToDB(architecture string, tag ReleaseTag, details ReleaseDetails) *models.ReleaseTag {
	release := models.ReleaseTag{
		Architecture: architecture,
		ReleaseTag:   details.Name,
		Phase:        tag.Phase,
	}
	// 4.10.0-0.nightly-2021-11-04-001635 -> 4.10
	parts := strings.Split(details.Name, ".")
	if len(parts) >= 2 {
		release.Release = strings.Join(parts[:2], ".")
	}

	// Get "nightly" or "ci" from the string
	if len(parts) >= 4 {
		stream := strings.Split(parts[3], "-")
		if len(stream) >= 2 {
			release.Stream = stream[0]
		}
	}

	dateTime := regexp.MustCompile(`.*([0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9]{6})`)
	match := dateTime.FindStringSubmatch(tag.Name)
	if len(match) > 1 {
		t, err := time.Parse("2006-01-02-150405", match[1])
		if err == nil {
			release.ReleaseTime = t
		}
	}

	if len(details.ChangeLog) == 0 {
		return nil // changelog not available yet
	}

	changelog := NewChangelog(tag.Name, string(details.ChangeLog))
	release.KubernetesVersion = changelog.KubernetesVersion()
	release.CurrentOSURL, release.CurrentOSVersion, release.PreviousOSURL, release.PreviousOSVersion, release.OSDiffURL = changelog.CoreOSVersion()
	release.PreviousReleaseTag = changelog.PreviousReleaseTag()
	release.Repositories = changelog.Repositories()
	release.PullRequests = changelog.PullRequests()
	release.JobRuns = releaseJobRunsToDB(details)

	return &release
}

func releaseJobRunsToDB(details ReleaseDetails) []models.ReleaseJobRun {
	rows := make([]models.ReleaseJobRun, 0)
	results := make(map[string]models.ReleaseJobRun)

	if jobs, ok := details.Results["blockingJobs"]; ok {
		for platform, jobResult := range jobs {
			id := idFromURL(jobResult.URL)
			results[id] = models.ReleaseJobRun{
				Name:           id,
				JobName:        platform,
				Kind:           "Blocking",
				State:          jobResult.State,
				URL:            jobResult.URL,
				Retries:        jobResult.Retries,
				TransitionTime: jobResult.TransitionTime,
			}
		}
	}

	if jobs, ok := details.Results["informingJobs"]; ok {
		for platform, jobResult := range jobs {
			id := idFromURL(jobResult.URL)
			results[id] = models.ReleaseJobRun{
				Name:           id,
				JobName:        platform,
				Kind:           "Informing",
				State:          jobResult.State,
				URL:            jobResult.URL,
				Retries:        jobResult.Retries,
				TransitionTime: jobResult.TransitionTime,
			}
		}
	}

	for _, upgrade := range append(details.UpgradesTo, details.UpgradesFrom...) {
		for _, run := range upgrade.History {
			id := idFromURL(run.URL)
			if result, ok := results[id]; ok {
				result.Upgrade = true
				result.UpgradesFrom = upgrade.From
				result.UpgradesTo = upgrade.To
			}
		}
	}

	for _, result := range results {
		rows = append(rows, result)
	}

	return rows
}

func idFromURL(prowURL string) string {
	parsed, err := url.Parse(prowURL)
	if err != nil {
		return ""
	}

	return path.Base(parsed.Path)
}
