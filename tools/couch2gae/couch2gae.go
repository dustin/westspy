package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-couch"
)

var largest int64

var baseURL string

var httpClient http.Client

var readTimeout = time.Second * 2

var batchSize = flag.Int("batchSize", 100,
	"Maximum batch size to transmit.")
var batchWait = flag.Duration("batchWait", time.Minute,
	"How long to wait for batches to build.")
var reportInterval = flag.Duration("reportInterval", time.Minute*15,
	"Sequence reporting interval.")
var reportKey = flag.String("reportKey", "_local/gae",
	"Key to store reported sequence in.")

func maybefatal(err error, msg string, args ...interface{}) {
	if err != nil {
		log.Fatalf(msg, args...)
	}
}

type reading struct {
	Reading   json.Number
	SensorOld *string `json:"sn"`
	SensorNew *string `json:"sensor"`
	Timestamp string  `json:"ts"`
}

type change struct {
	Seq int64
	Id  string
	Doc reading
}

func (r reading) SN() string {
	switch {
	case r.SensorOld != nil:
		return *r.SensorOld
	case r.SensorNew != nil:
		return *r.SensorNew
	}
	return ""
}

func reportSeq(s int64) {
	log.Printf("Recording sequence %v", s)
	db, err := couch.Connect(flag.Arg(0))
	if err != nil {
		log.Printf("Error connecting to couchdb: %v", err)
		return
	}

	m := map[string]interface{}{}
	err = db.Retrieve(*reportKey, &m)
	if err != nil {
		log.Printf("Error pulling report doc: %v", err)
		// Continue with partial data.
	}
	m["_id"] = *reportKey
	m["max_seq"] = s
	m["as_of"] = time.Now()

	if err == nil {
		_, err = db.Edit(m)
	} else {
		_, _, err = db.Insert(m)
	}
	if err != nil {
		log.Printf("Error storing doc:  %v", err)
	}
}

func feedBody(r io.Reader, results chan<- change) int64 {

	d := json.NewDecoder(r)
	d.UseNumber()

	nextReport := time.Now().Add(*reportInterval)

	for {
		thing := change{}
		err := d.Decode(&thing)
		if err != nil {
			if err.Error() == "unexpected EOF" {
				return largest
			} else {
				log.Printf("Error decoding stuff: %#v", err)
				return -1
			}
		}
		results <- thing
		largest = thing.Seq

		now := time.Now()
		if now.After(nextReport) {
			nextReport = now.Add(*reportInterval)
			go reportSeq(thing.Seq)
		}
	}
}

func parseTime(in string) (time.Time, error) {
	parts := strings.FieldsFunc(in, func(r rune) bool {
		return r == '/' || r == '-' || r == ':' || r == 'T' ||
			r == ' ' || r == '.'
	})

	if len(parts) < 6 {
		return time.Time{},
			fmt.Errorf("Incorrect number of fields: %#v", parts)
	}

	np := []int{}
	for _, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil {
			return time.Time{}, errors.New("Unparsable time")
		}
		np = append(np, x)
	}

	nsec := 0
	if len(np) > 6 {
		nsec = np[6] * 1000
	}

	return time.Date(np[0], time.Month(np[1]), np[2],
		np[3], np[4], np[5], nsec, time.Local), nil
}

func storeItems(cs []change) error {
	sns := []string{}
	tss := []string{}
	rs := []string{}

	for _, c := range cs {
		t, err := parseTime(c.Doc.Timestamp)
		if err != nil {
			log.Fatalf("Error parsing timestamp %#v: %v",
				c.Doc.Timestamp, err)
		}
		sns = append(sns, c.Doc.SN())
		tss = append(tss, t.UTC().Format(time.RFC3339Nano))
		rs = append(rs, string(c.Doc.Reading))
	}

	params := url.Values{
		"r":  rs,
		"sn": sns,
		"ts": tss,
	}

	start := time.Now()
	wd := time.AfterFunc(readTimeout, func() {
		log.Printf("Taking longer than %v to send data",
			readTimeout)
	})

	resp, err := httpClient.PostForm(baseURL, params)
	if err != nil {
		wd.Stop()
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		wd.Stop()
		return err
	}

	if !wd.Stop() {
		log.Printf("Finished long request in %v",
			time.Since(start))
	}
	return nil
}

func sendData(ch <-chan change) {
	t := time.NewTicker(*batchWait)
	for {
		items := []change{}
		timedout := false
		var tch <-chan time.Time
		for len(items) < *batchSize && !timedout {
			select {
			case c := <-ch:
				sn := c.Doc.SN()
				if sn != "" {
					items = append(items, c)
					tch = t.C
				}
			case <-tch:
				timedout = true
			}
		}

		log.Printf("Transmitting %v items", len(items))

		done := false
		retries := 5
		for !done {
			err := storeItems(items)
			if err == nil {
				done = true
			} else {
				retries--
				if retries > 0 {
					log.Printf("Failed to store item, retrying: %v",
						err)
					time.Sleep(time.Second)
				} else {
					log.Fatalf("Too much failure")
				}
			}
		}
	}
}

func main() {
	since := flag.Int64("since", 0, "Starting seq id")

	flag.Parse()
	db, err := couch.Connect(flag.Arg(0))
	maybefatal(err, "Error connecting: %v", err)

	baseURL = flag.Arg(1)

	var t http.RoundTripper = &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConnsPerHost: 50,
		Dial: func(n, addr string) (c net.Conn, err error) {
			c, derr := net.Dial(n, addr)
			return &timeoutConn{c, readTimeout}, derr
		},
	}

	httpClient.Transport = t

	info, err := db.GetInfo()
	maybefatal(err, "Error getting info: %v", err)
	log.Printf("Info %#v", info)

	ch := make(chan change)

	go sendData(ch)

	if *since < 0 {
		*since = info.UpdateSeq
		log.Printf("Setting since to %v", *since)
	}

	for {
		db.Changes(func(r io.Reader) int64 {
			return feedBody(r, ch)
		},
			map[string]interface{}{
				"since":        *since,
				"feed":         "continuous",
				"include_docs": true,
				"timeout":      10000,
				"heartbeat":    5000,
			})
		log.Printf("Error changesing: %v", err)
		log.Printf("Largest seen was %v", largest)
		*since = largest
		time.Sleep(time.Second)
	}
}
