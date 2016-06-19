package dustin

import (
	"encoding/xml"
	"html/template"
	"net/http"
	"sync"

	"golang.org/x/net/context"

	"google.golang.org/appengine"
	"google.golang.org/appengine/urlfetch"

	"github.com/dustin/httputil"
	"kylelemons.net/go/atom"
)

var (
	githubFeed, blogFeed *atom.Feed
	fmu                  sync.Mutex
)

func getGithub() []template.HTML {
	fmu.Lock()
	defer fmu.Unlock()

	if githubFeed == nil {
		return nil
	}

	var rv []template.HTML

	for _, e := range githubFeed.Entries {
		rv = append(rv, template.HTML(e.Content.Data))
	}

	return rv
}

func fetchFeed(c context.Context, url string) (*atom.Feed, error) {
	client := urlfetch.Client(c)

	res, err := client.Get(url)
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

	return feed, nil
}

func updateGithub(c context.Context) (*atom.Feed, error) {
	feed, err := fetchFeed(c, "https://github.com/dustin.atom")
	if err != nil {
		return nil, err
	}

	fmu.Lock()
	defer fmu.Unlock()
	githubFeed = feed

	return feed, nil
}

func getBlog() *atom.Feed {
	fmu.Lock()
	defer fmu.Unlock()

	return blogFeed
}

func updateBlog(c context.Context) (*atom.Feed, error) {
	feed, err := fetchFeed(c, "http://dustin.sallings.org/atom.xml")
	if err != nil {
		return nil, err
	}

	fmu.Lock()
	defer fmu.Unlock()
	blogFeed = feed

	return feed, nil
}

// UpdateFeeds updates all monitored feeds immediately.
func UpdateFeeds(w http.ResponseWriter, req *http.Request) {
	err := updateFeeds(appengine.NewContext(req))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

func bgAtomFetch(c context.Context, f func(c context.Context) (*atom.Feed, error),
	ch chan<- error) {
	_, err := f(c)
	ch <- err
}

func updateFeeds(c context.Context) error {
	ch := make(chan error)
	updates := []func(c context.Context) (*atom.Feed, error){
		updateGithub,
		updateBlog,
	}
	for _, f := range updates {
		f := f
		go func() {
			_, err := f(c)
			ch <- err
		}()
	}
	var err error
	for _ = range updates {
		e := <-ch
		if e != nil {
			err = e
		}
	}
	return err
}
