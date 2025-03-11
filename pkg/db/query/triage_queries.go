package query

/*
func LoadTriagesForRegressions(dbc *db.DB,
	regressionIDs []int, filterClosed bool) ([]models.Bug, error) {
	results := []models.Triage{}

	job := models.ProwJob{}
	q := dbc.DB.Where("id IN ?", regressionIDs)
	if filterClosed {
		q = q.Preload("Bugs", "UPPER(status) != 'CLOSED' and UPPER(status) != 'VERIFIED'")
	} else {
		q = q.Preload("Bugs")
	}
	res := q.First(&job)
	if res.Error != nil {
		return results, res.Error
	}
	log.Debugf("LoadBugsForJobs found %d bugs for job", len(job.Bugs))
	return job.Bugs, nil
}


*/
