package generichtml

import (
	"bytes"
	"text/template"
)

func MustSubstitute(tmpl *template.Template, data interface{}) string {
	buf := &bytes.Buffer{}
	err := tmpl.Execute(buf, data)
	if err != nil {
		panic(err)
	}
	return buf.String()
}
