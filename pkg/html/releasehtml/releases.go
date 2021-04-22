package releasehtml

import (
	"fmt"
	"strings"
)

// releasesList expects an array of report/release names like "4.8"
func releasesList(reportNames []string) string {
	if len(reportNames) == 0 {
		return ""
	}
	releaseLinks := make([]string, len(reportNames))
	for i := range reportNames {
		releaseLinks[i] = fmt.Sprintf(`[<a href="?release=%s">release-%[1]s</a>] `, reportNames[i])
	}
	return fmt.Sprintf("<p class=text-center>%s</p>", strings.Join(releaseLinks, "\n"))
}
