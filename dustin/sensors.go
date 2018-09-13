package dustin

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"encoding/json"
	humanize "github.com/dustin/go-humanize"
	"github.com/dustin/httputil"
	"golang.org/x/sync/errgroup"
	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/urlfetch"
)

func CheckSensors(w http.ResponseWriter, req *http.Request) {
	c := appengine.NewContext(req)

	hc := urlfetch.Client(c)

	careabout := map[string]bool{"oroville.workshop-temp": true,
		"oroville.workshop-humidity": true,
		"sj.attic-temp":              true,
		"sj.plant-temp":              true,
		"sj.plant-soil":              true,
	}

	const timefmt = "2006-01-02T15:04:05Z"
	const maxAge = time.Hour
	const maxErrAge = 2 * time.Hour

	g := errgroup.Group{}
	for k := range careabout {
		k := k
		g.Go(func() error {
			durl := "https://io.adafruit.com/api/v2/dlsspy/feeds/" + k + "/data/last"
			req, err := http.NewRequest("GET", durl, nil)
			if err != nil {
				return err
			}
			req.Header.Set("X-AIO-Key", os.Getenv("ADAFRUIT_IO_KEY"))
			res, err := hc.Do(req)
			if err != nil {
				return err
			}
			defer res.Body.Close()

			thing := struct {
				Value string
				TS    string `json:"created_at"`
			}{}
			if err := json.NewDecoder(res.Body).Decode(&thing); err != nil {
				return err
			}
			log.Debugf(c, "parsed response for %v as %#v", k, thing)

			ts, err := time.Parse(timefmt, thing.TS)
			if err != nil {
				return err
			}

			if time.Since(ts) > maxErrAge {
				log.Debugf(c, "too old to even report: %#v: %v", thing, time.Since(ts))
			} else if time.Since(ts) > maxAge {
				log.Infof(c, "too old:  %#v: %v", thing, time.Since(ts))

				msg := k + " last heard " + humanize.RelTime(ts, time.Now(), "ago", "since")
				form := url.Values{"value": {msg}}

				// POST /{username}/feeds/{feed_key}/data
				durl := "https://io.adafruit.com/api/v2/dlsspy/feeds/errors/data"
				req, err := http.NewRequest("POST", durl, strings.NewReader(form.Encode()))
				if err != nil {
					return err
				}
				req.Header.Set("X-AIO-Key", os.Getenv("ADAFRUIT_IO_KEY"))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if res, err := hc.Do(req); err != nil || res.StatusCode != 200 {
					log.Errorf(c, "error posting %s to %v -- %v:\n%v",
						form, durl, err, httputil.HTTPError(res))
				}
			}
			log.Debugf(c, "age of %#v from %v to %v is %v", thing, ts, time.Now(), time.Since(ts))

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		http.Error(w, err.Error(), 500)
	}
	w.WriteHeader(204)
}
