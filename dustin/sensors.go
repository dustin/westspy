package dustin

import (
	"net/http"
	"os"

	"time"

	"github.com/adafruit/io-client-go"
	humanize "github.com/dustin/go-humanize"
	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/urlfetch"
)

func CheckSensors(w http.ResponseWriter, req *http.Request) {
	c := appengine.NewContext(req)

	client := adafruitio.NewClient(os.Getenv("ADAFRUIT_IO_KEY"))
	client.Client = urlfetch.Client(c)
	feeds, _, err := client.Feed.All()

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	feed := &adafruitio.Feed{Name: "errors", Key: "errors"}
	client.SetFeed(feed)

	const timefmt = "2006-01-02T15:04:05Z"
	const maxAge = time.Hour

	careabout := map[string]bool{"oroville.workshop-temp": true, "oroville.workshop-humidity": true}

	for _, f := range feeds {
		if !careabout[f.Key] {
			continue
		}

		ts, err := time.Parse(timefmt, f.UpdatedAt)
		if err != nil {
			log.Errorf(c, "Error parsing timestamp from %#v: %v", f, err)
			continue
		}

		if time.Since(ts) > maxAge {
			log.Infof(c, "too old:  %#v: %v", f, time.Since(ts))
			if _, _, err := client.Data.Send(&adafruitio.Data{Value: f.Name + " last heard " + humanize.RelTime(ts, time.Now(), "ago", "since")}); err != nil {
				log.Errorf(c, "Error sending error message: %v", err)
			}
		}
		log.Debugf(c, "age of %#v from %v to %v is %v", f, ts, time.Now(), time.Since(ts))
	}
}
