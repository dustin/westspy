package house

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/memcache"
	"appengine/taskqueue"
)

const (
	readingQueue   = "readings"
	maxTasksPerAdd = 100
	maxItems       = 250
	maxPullTasks   = 1000
	maxPersistSize = 100
	persistEnabled = false
	currentExpiry  = time.Minute * 15
	sensorExpiry   = time.Hour * 24
	pConsume       = 0.01 // Probability of consuming after input
)

var whitelist = map[string]bool{"162.230.117.10": true}

func init() {
	rand.Seed(int64(time.Now().Nanosecond()))
}

func showError(c appengine.Context, w http.ResponseWriter, e string, code int) {
	http.Error(w, e, code)
	c.Errorf("Error response: %v (%v)", e, code)
}

func prepareOne(c appengine.Context, sn, ts, r string) (*taskqueue.Task, error) {
	f, err := strconv.ParseFloat(r, 64)
	if err != nil {
		return nil, err
	}

	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return nil, err
	}

	reading := Reading{Reading: f, Serial: sn, Timestamp: t}

	data, err := json.Marshal(&reading)
	must(err)

	return &taskqueue.Task{
		Payload: data,
		Method:  "PULL",
	}, nil
}

func mightConsume() bool {
	return rand.Float64() < pConsume
}

func HandleInput(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	if !whitelist[r.RemoteAddr] {
		showError(c, w, "Invalid address", 403)
		return
	}

	r.ParseForm()

	sns := r.Form["sn"]
	tss := r.Form["ts"]
	rs := r.Form["r"]

	if len(sns) != len(tss) || len(sns) != len(rs) {
		showError(c, w, "Incorrect parameters", 400)
		return
	}

	shouldConsume := false

	tasks := []*taskqueue.Task{}
	for i := range sns {
		t, err := prepareOne(c, sns[i], tss[i], rs[i])
		if err != nil {
			showError(c, w, "Error preparing: "+err.Error(), 400)
			return
		}
		tasks = append(tasks, t)
		if len(tasks) >= maxTasksPerAdd {
			_, err := taskqueue.AddMulti(c, tasks, readingQueue)
			if err != nil {
				showError(c, w, "Error queueing things: "+err.Error(), 500)
				return
			}
			tasks = nil
		}
		shouldConsume = shouldConsume || mightConsume()
	}

	if len(tasks) > 0 {
		_, err := taskqueue.AddMulti(c, tasks, readingQueue)
		if err != nil {
			showError(c, w, "Error queueing things: "+err.Error(), 500)
			return
		}
	}

	c.Debugf("Enqueued %v items", len(sns))
	if shouldConsume {
		c.Infof("Consuming input.")
		taskqueue.Add(c, taskqueue.NewPOSTTask("/cron/house/consume/", nil), "")
	}

	w.WriteHeader(202)
}

func maybePersist(c appengine.Context, keys []*datastore.Key, obs interface{}) (err error) {
	if persistEnabled {
		_, err = datastore.PutMulti(c, keys, obs)
	}
	return
}

func persistReadings(c appengine.Context, ch <-chan *Reading, ech chan<- error) {
	keys := []*datastore.Key{}
	obs := []*Reading{}

	var err error

	for r := range ch {
		keys = append(keys, datastore.NewKey(c, "Reading", r.Key(), 0, nil))
		obs = append(obs, r)

		if len(keys) >= maxPersistSize {
			err = maybePersist(c, keys, obs)
			if err != nil {
				break
			}
			keys = nil
			obs = nil
		}
	}

	if err == nil && len(keys) > 0 {
		err = maybePersist(c, keys, obs)
	}

	// Consume anything that might be left over in the error case
	for _ = range ch {
	}

	ech <- err
}

func processBatch(c appengine.Context) (int, error) {
	tasks, err := taskqueue.Lease(c, maxPullTasks, readingQueue, 60)
	if err != nil {
		return 0, err
	}
	if len(tasks) == 0 {
		return 0, nil
	}

	m := map[string]Readings{}

	rch := make(chan *Reading, maxPersistSize)
	ech := make(chan error, 2)

	go persistReadings(c, rch, ech)

	for _, task := range tasks {
		r := Reading{}
		must(json.Unmarshal(task.Payload, &r))
		m[r.Serial] = append(m[r.Serial], r)
		rch <- &r
	}
	close(rch)

	keys := []string{"current"}
	for k := range m {
		keys = append(keys, "r-"+k)
	}

	cached, err := memcache.GetMulti(c, keys)
	if err != nil {
		c.Warningf("memcache multiget failure: %v", err)
	}

	currentItem := cached["current"]
	delete(cached, "current")
	current := map[string]float64{}
	if currentItem != nil {
		json.Unmarshal(currentItem.Value, &current)
	}

	for k, cv := range cached {
		sn := k[2:]
		var v Readings
		err := json.Unmarshal(cv.Value, &v)
		if err == nil {
			m[sn] = append(m[sn], v...)
		}
	}

	items := []*memcache.Item{&memcache.Item{
		Key:        "current",
		Expiration: currentExpiry,
		Object:     current,
	}}
	for k, v := range m {
		v.Sort()
		if len(v) > maxItems {
			v = v[:maxItems]
		}
		current[k] = v[0].Reading
		items = append(items, &memcache.Item{
			Key:        "r-" + k,
			Expiration: sensorExpiry,
			Object:     v,
		})
	}

	go func() {
		ech <- memcache.JSON.SetMulti(c, items)
	}()

	err = consumeErrors(ech, 2)
	if err != nil {
		return 0, nil
	}

	must(taskqueue.DeleteMulti(c, tasks, readingQueue))

	return len(tasks), nil
}

func consumeErrors(ch <-chan error, n int) (err error) {
	for i := 0; i < n; i++ {
		if e := <-ch; e != nil {
			err = e
		}
	}
	return
}

func processInput(c appengine.Context) error {
	for {
		n, err := processBatch(c)
		if err != nil {
			return err
		}
		c.Infof("Processed %v items", n)
		if n < maxPullTasks {
			return nil
		}
	}
}

func ConsumeInput(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	if err := processInput(c); err != nil {
		showError(c, w, "Error processing batch: "+err.Error(), 500)
		return
	}

	w.WriteHeader(204)
}
