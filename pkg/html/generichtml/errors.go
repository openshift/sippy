package generichtml

import (
	"html/template"
	"k8s.io/klog"
	"net/http"
)

var messageTemplate = template.Must(template.New("jobs").Parse(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>{{.StatusCode}} {{.StatusMessage}}</title>
</head>
<body>
  <h1>{{.StatusCode}} {{.StatusMessage}}</h1>
  <hr>
  <p>{{.UserMessage}}</p>
</body>
</html>
`))

// PrintStatusMessage is used to display a proper error page to an end user.
func PrintStatusMessage(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	w.Header().Set("Content-Type", "text/html;charset=UTF-8")

	err := messageTemplate.Execute(w, map[string]interface{}{
		"StatusCode":    code,
		"StatusMessage": http.StatusText(code),
		"UserMessage":   message,
	})
	if err != nil {
		klog.Error(err)
	}
}
