package releaseloader

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm/clause"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

const (
	releaseTagsTable = "release_tags"
	succeeded        = "Succeeded"
	failed           = "Failed"
)

type ReleaseLoader struct {
	db            *db.DB
	httpClient    *http.Client
	releases      map[string]v1.Release
	architectures []string
	projects      []PayloadProject
	errors        []error
}

func New(dbc *db.DB, releases, architectures []string, releaseConfigs []v1.Release) *ReleaseLoader {
	configForRelease := make(map[string]v1.Release, len(releaseConfigs))
	for _, config := range releaseConfigs {
		if config.Capabilities[v1.PayloadTagsCap] {
			configForRelease[config.Release] = config
		}
	}
	if len(releases) > 0 {
		filteredRCs := make(map[string]v1.Release, len(releases))
		for _, release := range releases {
			if config, ok := configForRelease[release]; ok {
				filteredRCs[release] = config
			} else {
				log.Warningf("release %q is not configured to load payload tags", release)
			}
		}
		configForRelease = filteredRCs
	}

	return &ReleaseLoader{
		db:            dbc,
		httpClient:    &http.Client{Timeout: 60 * time.Second},
		releases:      configForRelease,
		architectures: architectures,
		projects:      []PayloadProject{&OCPProject{}, &OKDProject{}},
	}
}

func (r *ReleaseLoader) Name() string {
	return "releases"
}

func (r *ReleaseLoader) Errors() []error {
	return r.errors
}

func (r *ReleaseLoader) Load() {
	for _, project := range r.projects {
		projectName := project.GetName()
		for _, rs := range buildReleaseStreams(r.releases, r.architectures, project) {
			log.Infof("Fetching releaseStream %s from %s release controller...", rs.Name, projectName)
			for _, tag := range r.fetchReleaseTags(rs) {
				mReleaseTag := models.ReleaseTag{}
				r.db.DB.Table(releaseTagsTable).Where(`"release_tag" = ?`, tag.Name).Find(&mReleaseTag)
				// expect Phase to be populated if the record is present
				if len(mReleaseTag.Phase) > 0 {
					if mReleaseTag.Phase != tag.Phase {
						log.Warningf("Phase change detected (%q to %q) -- updating tag %s...", mReleaseTag.Phase, tag.Phase, tag.Name)
						mReleaseTag.Phase = tag.Phase
						mReleaseTag.Forced = true
						if err := r.db.DB.Clauses(clause.OnConflict{UpdateAll: true}).Table(releaseTagsTable).Save(mReleaseTag).Error; err != nil {
							log.WithError(err).Errorf("error updating release tag")
							r.errors = append(r.errors, errors.Wrapf(err, "error updating release tag %s for new phase: %s -> %s", tag.Name, mReleaseTag.Phase, tag.Phase))
						}
					}
					continue
				}

				log.Infof("Fetching tag %s from %s release controller...", tag.Name, projectName)
				releaseTag := r.buildReleaseTag(rs, tag)

				if releaseTag == nil {
					continue
				}

				if err := r.db.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&releaseTag, 100).Error; err != nil {
					r.errors = append(r.errors, errors.Wrapf(err, "error creating release tag: %s", releaseTag.ReleaseTag))
				}
			}
		}
	}
}

func (r *ReleaseLoader) buildReleaseTag(rs ReleaseStream, tag ReleaseTag) *models.ReleaseTag {
	releaseDetails := r.fetchReleaseDetails(rs, tag)
	if releaseDetails == nil {
		return nil
	}
	releaseTag := releaseDetailsToDB(rs, tag, *releaseDetails)

	// We skip releases that aren't fully baked (i.e. all jobs run and changelog calculated)
	if releaseTag == nil || (releaseTag.Phase != api.PayloadAccepted && releaseTag.Phase != api.PayloadRejected) {
		return nil
	}

	if len(releaseTag.PullRequests) > 0 {
		releaseTag.PullRequests = r.resolveReleasePullRequests(releaseTag.PullRequests)
	}

	return releaseTag
}

func (r *ReleaseLoader) resolveReleasePullRequests(pullRequests []models.ReleasePullRequest) []models.ReleasePullRequest {
	if len(pullRequests) == 0 {
		return pullRequests
	}

	type prKey struct{ url, name string }
	prIndexMap := make(map[prKey]int, len(pullRequests))
	orConditions := make([]string, 0, len(pullRequests))
	args := make([]any, 0, len(pullRequests)*2)

	for i, pr := range pullRequests {
		key := prKey{pr.URL, pr.Name}
		if _, exists := prIndexMap[key]; !exists {
			prIndexMap[key] = i
			orConditions = append(orConditions, "(url = ? AND name = ?)")
			args = append(args, key.url, key.name)
		}
	}

	existingPRs := bulkFetchPRsFromTbl(r.db, orConditions, args)

	for _, existingPR := range existingPRs {
		if index, ok := prIndexMap[prKey{existingPR.URL, existingPR.Name}]; ok {
			pullRequests[index] = existingPR
		}
	}

	return pullRequests
}

// bulkFetchPRsFromTbl is a function variable to allow mocking in tests
var bulkFetchPRsFromTbl = func(dbConn *db.DB, orConditions []string, args []any) []models.ReleasePullRequest {
	// Execute batch query to find existing PRs
	var pullRequests []models.ReleasePullRequest
	if err := dbConn.DB.Table("release_pull_requests").Where(strings.Join(orConditions, " OR "), args...).Find(&pullRequests).Error; err != nil {
		panic(err)
	}

	return pullRequests
}

func (r *ReleaseLoader) fetchReleaseDetails(rs ReleaseStream, tag ReleaseTag) *ReleaseDetails {
	releaseDetails := ReleaseDetails{}
	rcURL := rs.buildDetailsURL(tag.Name)

	resp, err := r.httpClient.Get(rcURL)
	if err != nil {
		log.WithError(err).Errorf("error fetching release details from %s", rcURL)
		return nil
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&releaseDetails); err != nil {
		log.WithError(err).Errorf("error decoding release details JSON from %s", rcURL)
		return nil
	}

	return &releaseDetails
}

func (r *ReleaseLoader) fetchReleaseTags(rs ReleaseStream) []ReleaseTag {
	uri := rs.buildTagsURL()
	resp, err := r.httpClient.Get(uri)
	if err != nil {
		log.WithError(err).Errorf("failed to connect to release controller at %s", uri)
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		log.Warnf("release controller returned non-200 error code for %s: %d %s", uri, resp.StatusCode, resp.Status)
		return nil
	}

	tags := ReleaseTags{}
	err = json.NewDecoder(resp.Body).Decode(&tags)
	defer resp.Body.Close()
	if err != nil {
		log.Errorf("couldn't decode json: %v", err)
		return nil
	}
	return tags.Tags
}

func releaseDetailsToDB(rs ReleaseStream, tag ReleaseTag, details ReleaseDetails) *models.ReleaseTag {
	release := models.ReleaseTag{
		Release:      rs.Release.Release,
		Stream:       rs.Stream,
		Architecture: rs.Architecture,
		ReleaseTag:   details.Name,
		Phase:        tag.Phase,
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

	if len(details.ChangeLogJSON.Components) > 0 {
		jsonChangeLog := parseChangeLogJSON(tag.Name, details.ChangeLogJSON)

		release.KubernetesVersion = jsonChangeLog.KubernetesVersion
		release.CurrentOSURL = jsonChangeLog.CurrentOSURL
		release.CurrentOSVersion = jsonChangeLog.CurrentOSVersion
		release.PreviousOSURL = jsonChangeLog.PreviousOSURL
		release.PreviousOSVersion = jsonChangeLog.PreviousOSVersion
		release.OSDiffURL = jsonChangeLog.OSDiffURL

		release.PreviousReleaseTag = jsonChangeLog.PreviousReleaseTag
		release.Repositories = jsonChangeLog.Repositories
		release.PullRequests = jsonChangeLog.PullRequests

	} else {
		changelog := NewChangelog(tag.Name, string(details.ChangeLog))
		release.KubernetesVersion = changelog.KubernetesVersion()
		release.CurrentOSURL, release.CurrentOSVersion, release.PreviousOSURL, release.PreviousOSVersion, release.OSDiffURL = changelog.CoreOSVersion()
		release.PreviousReleaseTag = changelog.PreviousReleaseTag()
		release.Repositories = changelog.Repositories()
		release.PullRequests = changelog.PullRequests()
	}
	release.JobRuns = releaseJobRunsToDB(details)

	// set forced flag
	failedBlocking := false

	for _, jRun := range release.JobRuns {
		if jRun.State == failed {
			if jRun.Kind == "Blocking" {
				failedBlocking = true
				break
			}
		}
	}

	if release.Phase == "Accepted" {
		release.Forced = failedBlocking
	} else if release.Phase == "Rejected" {
		release.Forced = !failedBlocking
	}

	return &release
}

func parseChangeLogJSON(releaseTag string, changeLogJSON ChangeLog) models.ReleaseTag {
	releaseChangeLogJSON := models.ReleaseTag{}

	releaseChangeLogJSON.PreviousReleaseTag = changeLogJSON.From.Name

	for _, c := range changeLogJSON.Components {
		if c.Name == "Kubernetes" {
			releaseChangeLogJSON.KubernetesVersion = c.Version
		} else if strings.Contains(c.Name, "CoreOS") {
			releaseChangeLogJSON.CurrentOSVersion = c.Version
			releaseChangeLogJSON.CurrentOSURL = c.VersionURL
			releaseChangeLogJSON.PreviousOSURL = c.FromURL
			releaseChangeLogJSON.PreviousOSVersion = c.From
			releaseChangeLogJSON.OSDiffURL = c.DiffURL
		}
	}

	type prlocator struct {
		name string
		url  string
	}

	releaseRepoRows := make([]models.ReleaseRepository, 0)
	releasePRRows := make(map[prlocator]models.ReleasePullRequest)
	for _, ui := range changeLogJSON.UpdatedImages {

		releaseRepoRow := models.ReleaseRepository{
			Name:    ui.Name,
			Head:    ui.Path,
			DiffURL: ui.FullChangeLog,
		}

		releaseRepoRows = append(releaseRepoRows, releaseRepoRow)

		for _, commit := range ui.Commits {
			releasePRRow := models.ReleasePullRequest{
				Name:          ui.Name,
				Description:   commit.Subject,
				URL:           commit.PullURL,
				PullRequestID: fmt.Sprintf("%d", commit.PullID),
			}

			// saves the last one..
			for _, value := range commit.Issues {
				releasePRRow.BugURL = value
			}

			for _, value := range commit.Bugs {
				releasePRRow.BugURL = value
			}

			prl := prlocator{
				url:  releasePRRow.URL,
				name: releasePRRow.Name,
			}
			if _, ok := releasePRRows[prl]; ok {
				log.Warningf("duplicate PR in %q: %q, %q", releaseTag, releasePRRow.URL, releasePRRow.Name)
			} else {
				releasePRRows[prl] = releasePRRow
			}
		}

	}

	releaseChangeLogJSON.Repositories = releaseRepoRows

	releasePullRequestResult := make([]models.ReleasePullRequest, 0)
	items := 0
	for _, v := range releasePRRows {
		// We had a case of a release payload changelog that contained 235,000 pull requests. Sippy got stuck on it
		// so this check is here to prevent something like that from ever happening again.  2,500 seems like a very
		// reasonable upper bound.
		if items > 2500 {
			log.Warningf("%q had more than 2,500 PR's! Ignoring the rest to protect ourself.", releaseTag)
			break
		}
		releasePullRequestResult = append(releasePullRequestResult, v)
		items++
	}

	releaseChangeLogJSON.PullRequests = releasePullRequestResult

	return releaseChangeLogJSON
}

func releaseJobRunsToDB(details ReleaseDetails) []models.ReleaseJobRun {
	rows := make([]models.ReleaseJobRun, 0)
	results := make(map[uint]models.ReleaseJobRun)

	if jobs, ok := details.Results["blockingJobs"]; ok {
		for platform, jobResult := range jobs {
			id, err := idFromURL(jobResult.URL)
			if id == 0 || err != nil {
				log.WithFields(map[string]interface{}{
					"id":         id,
					"releaseTag": details.Name,
					"url":        jobResult.URL,
					"platform":   platform,
					"error":      err,
				}).Warningf("invalid ID or missing URL for job")
				continue
			}

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
			id, err := idFromURL(jobResult.URL)
			if id == 0 || err != nil {
				log.WithFields(map[string]interface{}{
					"id":         id,
					"releaseTag": details.Name,
					"url":        jobResult.URL,
					"platform":   platform,
					"error":      err,
				}).Warningf("invalid ID or missing URL for job")
				continue
			}

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

	// For all upgrades, update the row for the corresponding prow job.
	for _, upgrade := range append(details.UpgradesTo, details.UpgradesFrom...) {
		for _, run := range upgrade.History {
			id, err := idFromURL(run.URL)
			if id == 0 || err != nil {
				log.WithFields(map[string]interface{}{
					"id":         id,
					"releaseTag": details.Name,
					"url":        run.URL,
					"error":      err,
				}).Warningf("invalid ID or missing URL for job")
				continue
			}

			if result, ok := results[id]; ok {
				result.Upgrade = true
				result.UpgradesFrom = upgrade.From
				result.UpgradesTo = upgrade.To
				results[id] = result
			}
		}
	}

	for _, result := range results {
		rows = append(rows, result)
	}

	return rows
}

func idFromURL(prowURL string) (uint, error) {
	if prowURL == "" {
		return 0, fmt.Errorf("prowURL should not be blank")
	}

	parsed, err := url.Parse(prowURL)
	if err != nil {
		return 0, err
	}

	base := path.Base(parsed.Path)
	prowID, err := strconv.ParseUint(base, 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(prowID), nil
}

func (rs *ReleaseStream) buildTagsURL() string {
	return fmt.Sprintf("%s/%s/tags", rs.baseReleaseStreamURL(), rs.Name)
}

func (rs *ReleaseStream) buildDetailsURL(tag string) string {
	return fmt.Sprintf("%s/%s/release/%s", rs.baseReleaseStreamURL(), rs.Name, tag)
}

func (rs *ReleaseStream) baseReleaseStreamURL() string {
	return fmt.Sprintf("https://%s/api/v1/releasestream", rs.Domain)
}

// buildReleaseStreams builds relevant release streams for specified releases that belong to the project.
func buildReleaseStreams(releases map[string]v1.Release, architectures []string, project PayloadProject) []ReleaseStream {
	releaseStreams := make([]ReleaseStream, 0, len(releases)*len(project.GetStreams()))
	for release, config := range releases {
		if project.IsProjectRelease(config) {
			for _, stream := range project.GetStreams() {
				for _, arch := range architectures {
					if name := project.FullReleaseStream(release, stream, arch); name != "" {
						releaseStreams = append(releaseStreams, ReleaseStream{
							Name:         name,
							Release:      config,
							Stream:       stream,
							Architecture: arch,
							Domain:       project.GetRcDomain(arch),
						})
					}
				}
			}
		}
	}
	return releaseStreams
}
