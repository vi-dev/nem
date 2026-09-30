package clidoc

import (
	"bytes"
	"embed"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var templates = template.Must(template.New("").Funcs(template.FuncMap{
	"list":    codeList,
	"applies": applies,
	"dashes":  func(s string) string { return strings.Repeat("-", len(s)+2) },
}).ParseFS(templateFS, "templates/*.tmpl"))

func render(name string, data any) []byte {
	var b bytes.Buffer
	if err := templates.ExecuteTemplate(&b, name, data); err != nil {
		panic(err)
	}
	return append(bytes.TrimRight(b.Bytes(), "\n"), '\n')
}

func applies(names []string) string {
	if len(names) == 1 {
		return "applies"
	}
	return "apply"
}

func codeList(items []string) string {
	quoted := make([]string, len(items))
	for i, it := range items {
		quoted[i] = "`" + it + "`"
	}
	switch len(quoted) {
	case 1:
		return quoted[0]
	case 2:
		return quoted[0] + " and " + quoted[1]
	default:
		return strings.Join(quoted[:len(quoted)-1], ", ") + ", and " + quoted[len(quoted)-1]
	}
}
