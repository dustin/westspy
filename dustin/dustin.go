package dustin

import (
	"html/template"
	"net/http"
	"strings"

	"appengine"
)

const base = "/~dustin/"

var templates *template.Template

func init() {
	var err error
	templates, err = template.New("").ParseGlob("templates/dustin/*html")
	if err != nil {
		panic("Couldn't parse templates: " + err.Error())
	}
}

func ServePage(w http.ResponseWriter, req *http.Request) {
	c := appengine.NewContext(req)
	page := req.URL.Path
	if !strings.HasPrefix(page, base) {
		panic(page)
	}
	page = page[len(base):]
	if page == "" {
		page = "index.html"
	}
	c.Infof("Serving %v", page)

	templates.ExecuteTemplate(w, page, nil)
}
