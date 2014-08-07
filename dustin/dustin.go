package dustin

import (
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"appengine"
)

const (
	tmplBase = "templates/dustin/"
	base     = "/~dustin/"
)

var (
	templates  = template.Must(loadTemplates())
	updateOnce sync.Once
)

func loadTemplates() (*template.Template, error) {
	rv := template.New("").Funcs(template.FuncMap{
		"limit": func(limit int, s interface{}) interface{} {
			v := reflect.ValueOf(s)
			if v.Len() < limit {
				return s
			}
			return v.Slice(0, limit).Interface()
		},
	})

	err := filepath.Walk(tmplBase, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		if !strings.HasPrefix(path, tmplBase) {
			panic(path)
		}
		short := path[len(tmplBase):]
		content, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = rv.New(short).Parse(string(content))
		return err
	})
	return rv, err
}

func ServePage(w http.ResponseWriter, req *http.Request) {
	c := appengine.NewContext(req)

	updateOnce.Do(func() {
		if !(getGithub() == nil && getBlog() == nil) {
			return
		}

		if err := updateFeeds(c); err != nil {
			c.Infof("Error updating feeds: %v", err)
		}
	})

	page := req.URL.Path
	if !strings.HasPrefix(page, base) {
		panic(page)
	}
	page = page[len(base):]
	if page == "" || strings.HasSuffix(page, "/") {
		page += "index.html"
	}
	c.Infof("Serving %v", page)

	err := templates.ExecuteTemplate(w, page, struct {
		Github interface{}
		Blog   interface{}
	}{getGithub(), getBlog()})

	if err != nil {
		c.Errorf("Error serving page %q: %v", page, err)
	}
}
