package dustin

import (
	"html/template"
	"net/http"
	"strings"
	"sync"

	"appengine"
)

const base = "/~dustin/"

var (
	templates  *template.Template
	updateOnce sync.Once
)

func init() {
	var err error
	templates, err = template.New("").ParseGlob("templates/dustin/*html")
	if err != nil {
		panic("Couldn't parse templates: " + err.Error())
	}
}

func ServePage(w http.ResponseWriter, req *http.Request) {
	c := appengine.NewContext(req)

	updateOnce.Do(func() {
		if getGithub() != nil {
			return
		}

		_, err := updateGithub(c)
		if err != nil {
			c.Errorf("Error doing initial github update: %v", err)
		}
	})

	page := req.URL.Path
	if !strings.HasPrefix(page, base) {
		panic(page)
	}
	page = page[len(base):]
	if page == "" {
		page = "index.html"
	}
	c.Infof("Serving %v", page)

	templates.ExecuteTemplate(w, page, struct {
		Github interface{}
	}{getGithub()})
}
