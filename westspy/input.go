package westspy

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"appengine"
	"appengine/memcache"
	"appengine/taskqueue"
)

const (
	readingQueue = "readings"
	maxItems     = 250
	maxTasks     = 1000
)

func init() {
	http.HandleFunc("/input/", handleInput)
	http.HandleFunc("/cron/consume/", consumeInput)
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
	if err != nil {
		panic(err)
	}

	return &taskqueue.Task{
		Payload: data,
		Method:  "PULL",
	}, nil
}

func handleInput(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	r.ParseForm()

	sns := r.Form["sn"]
	tss := r.Form["ts"]
	rs := r.Form["r"]

	if len(sns) != len(tss) || len(sns) != len(rs) {
		http.Error(w, "Incorrect parameters", 400)
		return
	}

	tasks := []*taskqueue.Task{}
	for i := range sns {
		t, err := prepareOne(c, sns[i], tss[i], rs[i])
		if err != nil {
			http.Error(w, "Error preparing: "+err.Error(), 400)
			return
		}
		tasks = append(tasks, t)
	}

	_, err := taskqueue.AddMulti(c, tasks, readingQueue)
	if err != nil {
		http.Error(w, "Error queueing things: "+err.Error(), 500)
		return
	}

	c.Debugf("Enqueued %v items", len(sns))

	w.WriteHeader(202)
}

func consumeInput(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	tasks, err := taskqueue.Lease(c, maxTasks, readingQueue, 60)
	if err != nil {
		panic(err)
	}

	c.Debugf("Found %v tasks in the queue", len(tasks))

	m := map[string]Readings{}

	for _, task := range tasks {
		r := Reading{}
		err := json.Unmarshal(task.Payload, &r)
		if err != nil {
			panic(err)
		}
		m[r.Serial] = append(m[r.Serial], r)
	}

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
		Key:    "current",
		Object: current,
	}}
	for k, v := range m {
		v.Sort()
		if len(v) > maxItems {
			v = v[:maxItems]
		}
		current[k] = v[0].Reading
		items = append(items, &memcache.Item{
			Key:    "r-" + k,
			Object: v,
		})
	}
	err = memcache.JSON.SetMulti(c, items)
	if err != nil {
		panic(err)
	}

	err = taskqueue.DeleteMulti(c, tasks, readingQueue)
	if err != nil {
		panic(err)
	}

	w.WriteHeader(204)
}
