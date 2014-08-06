package dustin

import (
	"encoding/json"
	"encoding/xml"
	"html/template"
	"net/http"
	"sync"

	"appengine"
	"appengine/urlfetch"

	"github.com/dustin/httputil"
	"kylelemons.net/go/atom"
)

var (
	githubFeed *atom.Feed
	fmu        sync.Mutex
)

func getGithub() []template.HTML {
	fmu.Lock()
	defer fmu.Unlock()

	if githubFeed == nil {
		return nil
	}

	var rv []template.HTML

	for i, e := range githubFeed.Entries {
		rv = append(rv, template.HTML(e.Content.Data))
		if i > 10 {
			break
		}
	}

	return rv
}

func updateGithub(c appengine.Context) (*atom.Feed, error) {
	client := urlfetch.Client(c)

	res, err := client.Get("https://github.com/dustin.atom")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, httputil.HTTPError(res)
	}

	feed := &atom.Feed{}
	err = xml.NewDecoder(res.Body).Decode(feed)
	if err != nil {
		return nil, err
	}

	c.Infof("Feed read: %v", feed)

	fmu.Lock()
	defer fmu.Unlock()
	githubFeed = feed

	return feed, nil
}

func UpdateGithub(w http.ResponseWriter, req *http.Request) {
	c := appengine.NewContext(req)
	feed, err := updateGithub(c)
	if err != nil {
		c.Errorf("Error fetching from github: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}

	json.NewEncoder(w).Encode(feed)
}
