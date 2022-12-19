package componentreportserver

import (
	"bytes"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/openshift/sippy/pkg/util/sets"

	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/db"
)

// aws, ovn, arm64, techpreview

func (s *Server) handleFeaturesForComponent(w http.ResponseWriter, req *http.Request) {
	var errToReport error
	buf := &bytes.Buffer{}
	defer func() {
		if errToReport == nil {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(buf, "ERROR: %v: %v", req.URL.Path, errToReport)
		}

		_, err := w.Write(buf.Bytes())
		if err != nil {
			fmt.Printf("ERROR: %v: %v", req.URL.Path, err)
		}
	}()
	segments := strings.Split(req.URL.Path, "/")
	if len(segments) != 3 {
		errToReport = fmt.Errorf("bad format")
		return
	}
	componentName := segments[2]

	featuresForComponent, err := listFeaturesForComponent(s.databaseConnection, componentName)
	if err != nil {
		errToReport = err
		return
	}

	toDisplay, err := featureForComponentForDisplay(featuresForComponent, componentName)
	if err != nil {
		errToReport = err
		return
	}

	fmt.Fprintf(buf, `
<head>
<style>
table, th, td {
  border: 1px solid black;
}
</style>
</head>
<body>
`)
	fmt.Fprintf(buf, "<h1>%v</h1>\n", componentName)
	fmt.Fprintf(buf, "<table>\n")
	fmt.Fprintf(buf, "\t<tr>\n")
	fmt.Fprintf(buf, "\t<th>%v</th>\n", "Feature")
	for _, curr := range toDisplay[0] {
		fmt.Fprintf(buf, "\t<th>%v</th>\n", curr.VariantSelectorName)
	}
	fmt.Fprintf(buf, "\t</tr>\n")

	for _, currRow := range toDisplay {
		fmt.Fprintf(buf, "\t<tr>\n")
		fmt.Fprintf(buf, "\t<td>%v</td>\n", currRow[0].FeatureName)
		for _, currJob := range currRow {
			color := ""
			switch currJob.Status {
			case "Fail":
				color = "#b35656"
			case "Pass":
				color = "##4a8242"
			}
			fmt.Fprintf(buf, "\t<td bgcolor=\"%v\">%v</td>\n", color, currJob.Status)
		}
		fmt.Fprintf(buf, "\t</tr>\n")
	}

	fmt.Fprintf(buf, "</table>\n")
	fmt.Fprintf(buf, "</body>\n")

}

type FeatureForComponent struct {
	ComponentID         int            `json:"component_id"`
	FeatureID           int            `json:"feature_id"`
	VariantSelectorID   int            `json:"variant_selector_id"`
	FailingMark         int            `json:"failing_mark"`
	WorkingMark         int            `json:"working_mark"`
	Status              string         `json:"status"`
	ComponentName       string         `json:"component_name"`
	FeatureName         string         `json:"feature_name"`
	VariantSelectorName string         `json:"job_name"`
	Variants            pq.StringArray `json:"variants" gorm:"type:text[]"`
}

type byComponentAndFeature []FeatureForComponent

func (a byComponentAndFeature) Len() int      { return len(a) }
func (a byComponentAndFeature) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byComponentAndFeature) Less(i, j int) bool {
	if a[i].ComponentName != a[j].ComponentName {
		return strings.Compare(a[i].ComponentName, a[j].ComponentName) < 0
	}
	if a[i].FeatureName != a[j].FeatureName {
		return strings.Compare(a[i].FeatureName, a[j].FeatureName) < 0
	}
	if a[i].VariantSelectorName == "Summary" {
		return true
	}
	if a[i].VariantSelectorName != a[j].VariantSelectorName {
		return strings.Compare(a[i].VariantSelectorName, a[j].VariantSelectorName) < 0
	}

	return strings.Compare(a[i].VariantSelectorName, a[j].VariantSelectorName) < 0
}

func listFeaturesForComponent(dbc *db.DB, componentName string) ([]FeatureForComponent, error) {
	ret := make([]FeatureForComponent, 0)
	q := dbc.DB.Raw(`SELECT * from feature_rollup_with_metadata_2 where component_name=@component_name`, sql.Named("component_name", componentName))
	if q.Error != nil {
		return nil, q.Error
	}
	q.Scan(&ret)
	return ret, nil
}

func featureForComponentForDisplay(in []FeatureForComponent, componentName string) ([][]FeatureForComponent, error) {
	// simple, not efficient, not sparse

	features := sets.NewString()
	jobNameToID := map[string]int{}
	featureToJobs := map[string][]FeatureForComponent{}
	for i := range in {
		curr := in[i]
		features.Insert(curr.FeatureName)
		jobNameToID[curr.VariantSelectorName] = curr.VariantSelectorID
		featureToJobs[curr.FeatureName] = append(featureToJobs[curr.FeatureName], curr)
	}

	// add entries for missing jobs and sort
	for featureName, jobData := range featureToJobs {
		if len(jobData) == len(jobNameToID) {
			sort.Sort(byComponentAndFeature(jobData))
			featureToJobs[featureName] = jobData
		}

		for jobName, jobID := range jobNameToID {
			found := false
			for _, currJobData := range jobData {
				if currJobData.VariantSelectorName == jobName {
					found = true
					break
				}
			}
			if found {
				continue
			}
			jobData = append(jobData, FeatureForComponent{
				ComponentID:         jobData[0].ComponentID,
				FeatureID:           jobData[0].FeatureID,
				VariantSelectorID:   jobID,
				FailingMark:         0,
				WorkingMark:         0,
				Status:              "", // could be "Missing"
				ComponentName:       jobData[0].ComponentName,
				FeatureName:         jobData[0].FeatureName,
				VariantSelectorName: jobName,
				Variants:            nil,
			})
			featureToJobs[featureName] = jobData
		}

		sort.Sort(byComponentAndFeature(jobData))
		featureToJobs[featureName] = jobData
	}

	ret := [][]FeatureForComponent{}
	for _, featureName := range features.List() {
		jobData := featureToJobs[featureName]
		currRow := []FeatureForComponent{summarizeFeatureForComponent(jobData)}
		currRow = append(currRow, jobData...)
		ret = append(ret, currRow)
	}

	// tidy names. This should be replaced by something that actually looks at variants
	for i := range ret {
		for j := range ret[i] {
			currJob := ret[i][j]
			if index := strings.Index(currJob.VariantSelectorName, "4.12"); index > 0 {
				ret[i][j].VariantSelectorName = currJob.VariantSelectorName[index+4:]
			}
			ret[i][j].VariantSelectorName = strings.ReplaceAll(ret[i][j].VariantSelectorName, "-", " ")
		}
	}

	return ret, nil
}

func summarizeFeatureForComponent(in []FeatureForComponent) FeatureForComponent {
	failed := false
	for i := range in {
		curr := in[i]
		if curr.Status == "Fail" {
			failed = true
			break
		}
	}

	ret := FeatureForComponent{
		ComponentID:         in[0].ComponentID,
		FeatureID:           in[0].FeatureID,
		VariantSelectorID:   -2,
		ComponentName:       in[0].ComponentName,
		FeatureName:         in[0].FeatureName,
		VariantSelectorName: "Summary",
		Variants:            nil,
	}
	if failed {
		ret.FailingMark = 1
		ret.WorkingMark = 0
		ret.Status = "Fail"
	} else {
		ret.FailingMark = 0
		ret.WorkingMark = 1
		ret.Status = "Pass"
	}

	return ret
}
