package html

import (
	"fmt"
)

const (
	up       = `<i class="fa fa-arrow-up" title="Increased %0.2f%%" style="font-size:28px;color:green"></i>`
	down     = `<i class="fa fa-arrow-down" title="Decreased %0.2f%%" style="font-size:28px;color:red"></i>`
	flatup   = `<i class="fa fa-arrows-h" title="Increased %0.2f%%" style="font-size:28px;color:darkgray"></i>`
	flatdown = `<i class="fa fa-arrows-h" title="Decreased %0.2f%%" style="font-size:28px;color:darkgray"></i>`
	flat     = `<i class="fa fa-arrows-h" style="font-size:28px;color:darkgray"></i>`
)

func getArrow(totalRuns int, currPassPercentage, prevPassPercentage float64) string {
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

type colorizationCriteria struct {
	minRedPercent    float64
	minYellowPercent float64
	minGreenPercent  float64
}

func (c colorizationCriteria) getColor(passPercentage float64) string {
	switch {
	case passPercentage > c.minGreenPercent:
		return "table-success"
	case passPercentage > c.minYellowPercent:
		return "table-warning"
	case passPercentage > c.minRedPercent:
		return "table-danger"
	default:
		return "error"
	}
}
