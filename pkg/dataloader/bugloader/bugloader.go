package bugloader

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	bqgo "cloud.google.com/go/bigquery"
	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

const (
	// Unfortunate cross-project join
	ComponentMappingProject = "openshift-gce-devel"
	ComponentMappingDataset = "ci_analysis_us"
	ComponentMappingTable   = "component_mapping_latest"

	TicketDataQuery = `WITH TicketData AS (
  SELECT
    t.*,
    c.message AS comment
  FROM
    openshift-ci-data-analysis.jira_data.tickets_dedup t
  LEFT JOIN UNNEST(t.comments) AS c
  WHERE t.summary IS NOT NULL AND last_changed_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 14 DAY)
)
SELECT
  t.issue.key as key,
  t.issue.id AS jira_id,
  t.summary as summary,
  j.name AS link_name,
  t.last_changed_time as last_changed_time,
  t.status.name as status,
  ARRAY(SELECT name FROM UNNEST(affects_versions)) as affects_versions,
  ARRAY(SELECT name FROM UNNEST(fix_versions)) as fix_versions,
  ARRAY(SELECT name FROM UNNEST(target_versions)) as target_versions,
  ARRAY(SELECT name FROM UNNEST(components)) as components,
  t.labels as labels
FROM
  TicketData t`
)

type BugLoader struct {
	dbc    *db.DB
	bqc    *bigquery.Client
	errors []error
}

type bigQueryBug struct {
	ID              uint               `json:"id" bigquery:"id"`
	Key             string             `json:"key" bigquery:"key"`
	Status          string             `json:"status" bigquery:"status"`
	LastChangedTime bqgo.NullTimestamp `json:"last_changed_time" bigquery:"last_changed_time"`
	Summary         string             `json:"summary" bigquery:"summary"`
	AffectsVersions []string           `json:"affects_versions" bigquery:"affects_versions"`
	FixVersions     []string           `json:"fix_versions" bigquery:"fix_versions"`
	TargetVersions  []string           `json:"target_versions" bigquery:"target_versions"`
	Components      []string           `json:"components" bigquery:"components"`
	Labels          []string           `json:"labels" bigquery:"labels"`
	JiraID          string             `bigquery:"jira_id"`
	LinkName        string             `bigquery:"link_name"`
}

func New(dbc *db.DB, bqc *bigquery.Client) *BugLoader {
	return &BugLoader{
		dbc: dbc,
		bqc: bqc,
	}
}

func (bl *BugLoader) Name() string {
	return "bugs"
}

func (bl *BugLoader) Errors() []error {
	return bl.errors
}

func (bl *BugLoader) addError(logger *log.Entry, err error, msg string) {
	logger.WithError(err).Error(msg)
	bl.errors = append(bl.errors, errors.Wrap(err, msg))
}

// load updated bugs from BQ and cross-reference with tests, jobs, and triage from postgres
func (bl *BugLoader) loadLatestBugs() (bugsFromDb []*models.Bug, triages []models.Triage, ok bool) {
	logger := log.WithField("func", "bugloader.loadLatestBugs")

	// Fetch known tests and ownerships from postgres
	testCache, err := query.LoadTestCache(bl.dbc, []string{"TestOwnerships"})
	if err != nil {
		bl.addError(logger, err, "error loading test cache")
		return
	}
	// Fetch (from bigquery) bugs that mention known (postgres) tests, so we can update mappings later
	testBugs, err := bl.getTestBugMappings(context.TODO(), testCache)
	if err != nil {
		bl.addError(logger, err, "error loading test bug mappings")
		return
	}
	logger.WithField("bugs", len(testBugs)).Info("Loaded test bugs")

	// Fetch known jobs from postgres
	jobCache, err := query.LoadProwJobCache(bl.dbc)
	if err != nil {
		bl.addError(logger, err, "error loading prow job cache")
		return
	}
	// Fetch (from bigquery) bugs that mention known (postgres) jobs, so we can update mappings later
	jobBugs, err := bl.getJobBugMappings(context.TODO(), jobCache)
	if err != nil {
		bl.addError(logger, err, "error loading bug-job mappings")
		return
	}
	logger.WithField("bugs", len(jobBugs)).Info("Loaded job bugs")

	// Fetch bugs triaged to component readiness regressions if not already picked up above,
	// sometimes the test name is forgotten in the bug, sometimes the mapping breaks due to
	// weird whitespace issues:
	triages, err = query.ListTriages(bl.dbc)
	if err != nil {
		bl.addError(logger, err, "error loading triages")
		return
	}
	triageBugs, err := bl.getTriageBugMappings(context.TODO(), triages)
	if err != nil {
		bl.addError(logger, err, "error loading triage bug mappings")
		return
	}
	logger.WithField("bugs", len(triageBugs)).Info("Loaded triage bugs")

	// Merge all the bugs together (deduplicating by ID)
	allBugs := testBugs
	for _, b := range jobBugs {
		if _, seen := allBugs[b.ID]; seen {
			allBugs[b.ID].Jobs = b.Jobs // merge if both tests and jobs were mentioned
			continue
		}
		allBugs[b.ID] = b
	}
	for _, b := range triageBugs {
		if _, seen := allBugs[b.ID]; !seen {
			allBugs[b.ID] = b
		}
	}
	logger.WithField("bugs", len(allBugs)).Info("Loaded all job bugs")

	// flatten the map into a slice for return
	bugsFromDb = make([]*models.Bug, 0, len(allBugs))
	for _, b := range allBugs {
		bugsFromDb = append(bugsFromDb, b)
	}

	ok = true // nothing failed...
	return
}

// Upsert latest bugs and mappings to tests/jobs in postgres
func (bl *BugLoader) updateBugsInDb(latestBugs []*models.Bug) {
	logger := log.WithField("func", "bugloader.updateBugsInDb")
	updatedBugs := 0
	for _, bug := range latestBugs {
		res := bl.dbc.DB.Clauses(clause.OnConflict{
			UpdateAll: true,
		}).Create(bug)
		if res.Error != nil {
			bl.addError(logger, res.Error, fmt.Sprintf("error creating bug: %v", bug))
			continue
		}
		updatedBugs++
		// With gorm we need to explicitly replace the associations to tests and jobs to get them to take effect:
		err := bl.dbc.DB.Model(bug).Association("Tests").Replace(bug.Tests)
		if err != nil {
			bl.addError(logger, err, fmt.Sprintf("error updating bug test associations: %v", bug))
			continue
		}
		err = bl.dbc.DB.Model(bug).Association("Jobs").Replace(bug.Jobs)
		if err != nil {
			bl.addError(logger, err, fmt.Sprintf("error updating bug job associations: %v", bug))
			continue
		}
	}
	logger.WithField("bugs", updatedBugs).Info("created or updated bugs")
}

// Some triage records may have been aligned to bugs that did not mention a test name and were just imported.
// If so we need to establish the db link between these and the new bug records in postgres.
// Also watch out for triages that changed bug url, and fix that linkage.
func (bl *BugLoader) updateTriageBugLinks(triages []models.Triage) {
	logger := log.WithField("func", "bugloader.updateTriageBugLinks")
	logger.Infof("ensuring triages have correct refs to their bugs")
	for _, t := range triages {
		if t.BugID != nil && t.URL == t.Bug.URL {
			continue // Ignore bugs that already seem properly linked
		}

		var bug models.Bug
		res := bl.dbc.DB.Where("url = ?", t.URL).First(&bug)
		if res.Error != nil {
			// Someone could have put in a bad url, we won't let that error out our reconcile job.
			logger.WithError(res.Error).Warnf("error looking up bug which should exist by this point: %s", t.URL)
			continue
		}

		info := fmt.Sprintf("linking triage %q (%d) to bug %q (%d)", t.Description, t.ID, bug.Summary, bug.ID)
		logger.Info(info)
		t.Bug = &bug
		t.BugID = &bug.ID
		res = bl.dbc.DB.WithContext(context.WithValue(context.Background(), models.CurrentUserKey, "bug-loader")).Save(t)
		if res.Error != nil {
			bl.addError(logger, res.Error, "error "+info)
		}
	}
}

func (bl *BugLoader) Load() {
	// methods below record errors in bl.errors, so we don't need to return them
	if latestBugs, triages, ok := bl.loadLatestBugs(); ok { // no errors preventing processing
		bl.updateBugsInDb(latestBugs)
		bl.updateTriageBugLinks(triages)
	}
}

// getTestBugMappings looks for jira cards that contain a test name from the ci-test-mapping database in bigquery.  We
// search the Jira comments, description and summary for the test name.
func (bl *BugLoader) getTestBugMappings(ctx context.Context, testCache map[string]*models.Test) (map[uint]*models.Bug, error) {
	bugs := make(map[uint]*models.Bug)

	// `WHERE j.name != upgrade` is because there's a test named just `upgrade` in some junits,
	// and querying against Jira produces thousands of tickets that mention `upgrade`; so just ignore it.
	querySQL := fmt.Sprintf(
		`%s
		JOIN %s.%s.%s j
		  ON STRPOS(t.summary, j.name) > 0
		  OR STRPOS(t.description, j.name) > 0
		  OR STRPOS(t.comment, j.name ) > 0
        WHERE j.name != "upgrade"`,
		TicketDataQuery, ComponentMappingProject, ComponentMappingDataset, ComponentMappingTable)
	log.Debug(querySQL)
	q := bl.bqc.BQ.Query(querySQL)

	it, err := q.Read(ctx)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to execute query")
	}

	// create a lookup of all the tests that are mapped to the same test id
	testsForUID := MapTestCacheByUniqueID(testCache)
	// and keep track of the tests we've seen for each bug id so we don't add duplicates
	testsSeenForBug := make(map[uint]sets.Set[string])

	for {
		var bqb bigQueryBug
		err := it.Next(&bqb)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.WithMessage(err, "failed to iterate over bug results")
		}

		// Make sure data in BQ is sane
		if bqb.JiraID == "" || bqb.LinkName == "" {
			continue
		}

		jiraID, err := strconv.ParseUint(bqb.JiraID, 10, 64)
		if err != nil {
			bl.errors = append(bl.errors, errors.WithMessagef(err, "failed to convert jira id %s", bqb.JiraID))
			continue
		}
		bqb.ID = uint(jiraID)

		// look up sippy DB's record for this test name
		test, found := testCache[bqb.LinkName]
		if !found {
			// This is probably common since we're using ci-test-mapping test names, and sippy may not know all of them
			log.Debugf("test name was in jira issue but not known by sippy: %s", bqb.LinkName)
			continue
		}

		tests := []models.Test{*test}
		// if we found test ownership, include all tests from the same ownership
		for _, mapping := range test.TestOwnerships {
			if mappedTests, found := testsForUID[mapping.UniqueID]; found {
				tests = append(tests, mappedTests...)
			}
		}

		// map a bug for this jira id if we haven't already
		if _, found := bugs[bqb.ID]; !found {
			bugs[bqb.ID] = bigQueryBugToModel(bqb)
		}

		// track the tests we've seen for this bug id and add non-duplicates
		seen := testsSeenForBug[bqb.ID]
		if seen == nil {
			seen = sets.New[string]()
			testsSeenForBug[bqb.ID] = seen
		}
		for _, test := range tests {
			if !seen.Has(test.Name) {
				seen.Insert(test.Name)
				bugs[bqb.ID].Tests = append(bugs[bqb.ID].Tests, test)
			}
		}
	}

	return bugs, nil
}

// MapTestCacheByUniqueID takes a map of tests by name, with the TestOwnership preloaded, and returns a map of
// all the tests that share the same unique ID (same TestOwnership).
func MapTestCacheByUniqueID(testForID map[string]*models.Test) map[string][]models.Test {
	testForUniqueID := make(map[string][]models.Test, len(testForID))
	for _, test := range testForID {
		for _, mapping := range test.TestOwnerships {
			if mapping.UniqueID != "" {
				testForUniqueID[mapping.UniqueID] = append(testForUniqueID[mapping.UniqueID], *test)
			}
		}
	}
	return testForUniqueID
}

// getJobBugMappings looks for jira cards that contain a job name from the jobs table in bigquery.  We
// search the Jira comments, description and summary for the job name.
func (bl *BugLoader) getJobBugMappings(ctx context.Context, jobCache map[string]*models.ProwJob) (map[uint]*models.Bug, error) {
	bugs := make(map[uint]*models.Bug)

	querySQL := TicketDataQuery + `
		JOIN (
            SELECT DISTINCT prowjob_job_name AS name
            FROM openshift-gce-devel.ci_analysis_us.jobs
            WHERE prowjob_job_name IS NOT NULL
              AND prowjob_job_name != ""
        ) j
        ON STRPOS(t.summary, j.name) > 0
        OR STRPOS(t.description, j.name) > 0
        OR STRPOS(t.comment, j.name) > 0
    `
	log.Debug(querySQL)
	q := bl.bqc.BQ.Query(querySQL)

	it, err := q.Read(ctx)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to execute query")
	}

	for {
		var bwj bigQueryBug
		err := it.Next(&bwj)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.WithMessage(err, "failed to iterate over bug results")
		}

		// Make sure data in BQ is sane
		if bwj.JiraID == "" || bwj.LinkName == "" {
			continue
		}

		intID, err := strconv.Atoi(bwj.JiraID)
		if err != nil {
			bl.errors = append(bl.errors, errors.WithMessagef(err, "failed to convert jira id %s", bwj.JiraID))
			continue

		}
		bwj.ID = uint(intID)

		if _, ok := jobCache[bwj.LinkName]; !ok {
			// This is probably common because sippy probably doesn't know about *all* jobs like the BQ table does
			log.Debugf("job name was in jira issue but not known by sippy: %s", bwj.LinkName)
			continue
		}

		if _, ok := bugs[bwj.ID]; !ok {
			bugs[bwj.ID] = bigQueryBugToModel(bwj)
		}

		bugs[bwj.ID].Jobs = append(bugs[bwj.ID].Jobs, *jobCache[bwj.LinkName])
	}

	return bugs, nil
}

// getTriageBugMappings looks for jira cards in bigquery that were traiged to a regression n bigquery.
// Once found we then associate them to their records in the triage table.
func (bl *BugLoader) getTriageBugMappings(ctx context.Context, triages []models.Triage) (map[uint]*models.Bug, error) {
	bugs := make(map[uint]*models.Bug)

	jiraKeys := make([]string, len(triages))
	for i, triage := range triages {
		key, err := parseBugKeyFromURL(triage.URL)
		if err != nil {
			log.WithError(err).Errorf("failed to parse bug key from %s", triage.URL)
			return bugs, err
		}
		jiraKeys[i] = key
	}

	// need to remove a problematic piece of the shared query for this case, but I'd like to keep
	// the rest:
	sharedQuery := strings.ReplaceAll(TicketDataQuery, "j.name AS link_name,", "")

	querySQL := fmt.Sprintf(
		`%s WHERE t.issue.key IN UNNEST(@keys)`,
		sharedQuery)
	log.Debug(querySQL)
	q := bl.bqc.BQ.Query(querySQL)
	q.Parameters = append(q.Parameters, bqgo.QueryParameter{Name: "keys", Value: jiraKeys})

	it, err := q.Read(ctx)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to execute query")
	}

	for {
		var bwt bigQueryBug
		err := it.Next(&bwt)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.WithMessage(err, "failed to iterate over bug results")
		}

		// Make sure data in BQ is sane
		if bwt.JiraID == "" {
			continue
		}

		intID, err := strconv.Atoi(bwt.JiraID)
		if err != nil {
			bl.errors = append(bl.errors, errors.WithMessagef(err, "failed to convert jira id %s", bwt.JiraID))
			continue
		}
		bwt.ID = uint(intID)

		if _, ok := bugs[bwt.ID]; !ok {
			bugs[bwt.ID] = bigQueryBugToModel(bwt)
		}

	}

	return bugs, nil
}

// ConvertBigQueryBugToModel converts a BigQuery bug representation to the model's Bug struct.
func bigQueryBugToModel(bqBug bigQueryBug) *models.Bug {
	lastChange := time.Now()
	if bqBug.LastChangedTime.Valid {
		lastChange = bqBug.LastChangedTime.Timestamp
	}
	return &models.Bug{
		ID:              bqBug.ID,
		Key:             bqBug.Key,
		Status:          bqBug.Status,
		LastChangeTime:  lastChange,
		Summary:         bqBug.Summary,
		AffectsVersions: pq.StringArray(bqBug.AffectsVersions),
		FixVersions:     pq.StringArray(bqBug.FixVersions),
		TargetVersions:  pq.StringArray(bqBug.TargetVersions),
		Components:      pq.StringArray(bqBug.Components),
		Labels:          pq.StringArray(bqBug.Labels),
		URL:             fmt.Sprintf("https://issues.redhat.com/browse/%s", bqBug.Key),
	}
}

func parseBugKeyFromURL(jiraURL string) (string, error) {
	parsedURL, err := url.Parse(jiraURL)
	if err != nil {
		return "", err
	}

	pathSegments := strings.Split(parsedURL.Path, "/")
	if len(pathSegments) < 3 || pathSegments[len(pathSegments)-2] != "browse" {
		return "", errors.New("invalid Jira URL format")
	}

	return pathSegments[len(pathSegments)-1], nil
}
