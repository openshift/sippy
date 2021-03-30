package releasehtml

import (
	"html/template"
	"log"
	"net/http"
)

var jobsTemplate = template.Must(template.New("jobs").Parse(`
<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Release {{.Release}} Jobs Dashboard</title>
  <link href="/static/jobs.css" rel="stylesheet">
</head>
<body>
  <div id="app"></div>

  <script src="https://unpkg.com/react@17/umd/react.development.js" crossorigin></script>
  <script src="https://unpkg.com/react-dom@17/umd/react-dom.development.js" crossorigin></script>
  <script src="https://unpkg.com/@babel/standalone/babel.min.js"></script>

  <script>
    var release = {{.Release}};
  </script>
  <script data-plugins="transform-modules-umd" data-presets="react" data-type="module" type="text/babel" src="/static/jobsdashboard.mjs"></script>
  <script data-plugins="transform-modules-umd" data-presets="react" data-type="module" type="text/babel">
    import { JobsDashboard } from '/static/jobsdashboard.mjs';
    const domContainer = document.querySelector('#app');
    ReactDOM.render(<JobsDashboard release={release} />, domContainer);
  </script>
</body>
</html>
`))

func PrintJobsReport(w http.ResponseWriter, release string) {
	err := jobsTemplate.Execute(w, map[string]interface{}{
		"Release": release,
	})
	if err != nil {
		log.Print(err)
	}
}
