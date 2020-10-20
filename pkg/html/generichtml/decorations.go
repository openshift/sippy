package generichtml

import (
	"fmt"
	"net/url"
	"regexp"
	"text/template"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

const (
	up       = `<i class="fa fa-arrow-up" title="Increased %0.2f%%" style="font-size:28px;color:green"></i>`
	down     = `<i class="fa fa-arrow-down" title="Decreased %0.2f%%" style="font-size:28px;color:red"></i>`
	flatup   = `<i class="fa fa-arrows-h" title="Increased %0.2f%%" style="font-size:28px;color:darkgray"></i>`
	flatdown = `<i class="fa fa-arrows-h" title="Decreased %0.2f%%" style="font-size:28px;color:darkgray"></i>`
	Flat     = `<i class="fa fa-arrows-h" style="font-size:28px;color:darkgray"></i>`
)

func GetArrow(totalRuns int, currPassPercentage, prevPassPercentage float64) string {
	delta := 5.0
	if totalRuns > 80 {
		delta = 2
	}

	if currPassPercentage > prevPassPercentage+delta {
		return fmt.Sprintf(up, currPassPercentage-prevPassPercentage)
	} else if currPassPercentage < prevPassPercentage-delta {
		return fmt.Sprintf(down, prevPassPercentage-currPassPercentage)
	} else if currPassPercentage > prevPassPercentage {
		return fmt.Sprintf(flatup, currPassPercentage-prevPassPercentage)
	} else {
		return fmt.Sprintf(flatdown, prevPassPercentage-currPassPercentage)
	}
}

func GetArrowForTestResult(curr sippyprocessingv1.TestResult, prev *sippyprocessingv1.TestResult) string {
	if prev == nil {
		return Flat
	}
	return GetArrow(curr.Successes+curr.Failures, curr.PassPercentage, prev.PassPercentage)
}

func GetArrowForFailedTestResult(curr sippyprocessingv1.FailingTestResult, prev *sippyprocessingv1.FailingTestResult) string {
	if prev == nil {
		return Flat
	}
	return GetArrow(curr.TestResultAcrossAllJobs.Successes+curr.TestResultAcrossAllJobs.Failures, curr.TestResultAcrossAllJobs.PassPercentage, prev.TestResultAcrossAllJobs.PassPercentage)
}

type ColorizationCriteria struct {
	MinRedPercent    float64
	MinYellowPercent float64
	MinGreenPercent  float64
}

var StandardColors = ColorizationCriteria{
	MinRedPercent:    0,  // failure.  In this range, there is a systemic failure so severe that a reliable signal isn't available.
	MinYellowPercent: 60, // at risk.  In this range, there is a systemic problem that needs to be addressed.
	MinGreenPercent:  80, // no action required. This *should* be closer to 85%
}

var OverallInstallUpgradeColors = ColorizationCriteria{
	MinRedPercent:    0,  // failure.  In this range, there is a systemic failure so severe that a reliable signal isn't available.
	MinYellowPercent: 85, // at risk.  In this range, there is a systemic problem that needs to be addressed.
	MinGreenPercent:  90, // no action required.  TODO this should be closer to 95, but we need to ratchet there
}

func (c ColorizationCriteria) GetColor(passPercentage float64, total int) string {
	switch {
	case total == 0:
		return "table-secondary"
	case passPercentage > c.MinGreenPercent:
		return "table-success"
	case passPercentage > c.MinYellowPercent:
		return "table-warning"
	case passPercentage > c.MinRedPercent:
		return "table-danger"
	default:
		return "error"
	}
}

var collapseNameRemoveRegex = regexp.MustCompile(`[. ,:\(\)\[\]]`)

func MakeSafeForCollapseName(in string) string {
	return collapseNameRemoveRegex.ReplaceAllString(in, "")
}

func GetExpandingButtonHTML(sectionName, buttonName string) string {
	buttonHTML := `<button class="btn btn-primary btn-sm py-0" style="font-size: 0.8em" type="button" data-toggle="collapse" data-target=".{{ .sectionName }}" aria-expanded="false" aria-controls="{{ .sectionName }}">{{ .buttonName }}</button>`
	buttonHTMLTemplate := template.Must(template.New("buttonHTML").Parse(buttonHTML))

	return MustSubstitute(buttonHTMLTemplate, map[string]string{
		"sectionName": sectionName,
		"buttonName":  buttonName,
	})
}

func GetTestDetailsButtonHTML(release string, testNames ...string) string {
	testDetailsButtonHTML := `<a class="btn btn-primary btn-sm py-0" style="font-size: 0.8em" href="/testdetails?release={{ .release }}{{range .testNames}}&test={{ . }}{{end}}" target="_blank" role="button">Test Details by Variants</a>`
	buttonHTMLTemplate := template.Must(template.New("testDetailsButtonHTML").Parse(testDetailsButtonHTML))

	escapedTestNames := []string{}
	for _, curr := range testNames {
		escapedTestNames = append(escapedTestNames, url.QueryEscape(curr))
	}

	return MustSubstitute(buttonHTMLTemplate, map[string]interface{}{
		"release":   release,
		"testNames": escapedTestNames,
	})
}
