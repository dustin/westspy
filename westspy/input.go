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
	readingQueue   = "readings"
	maxTasksPerAdd = 100
	maxItems       = 250
	maxPullTasks   = 1000
)

func init() {
	http.HandleFunc("/input/", handleInput)
	http.HandleFunc("/cron/consume/", consumeInput)
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
		showError(c, w, "Incorrect parameters", 400)
		return
	}

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
	}

	if len(tasks) > 0 {
		_, err := taskqueue.AddMulti(c, tasks, readingQueue)
		if err != nil {
			showError(c, w, "Error queueing things: "+err.Error(), 500)
			return
		}
	}

	c.Debugf("Enqueued %v items", len(sns))

	w.WriteHeader(202)
}

func processBatch(c appengine.Context) (int, error) {
	tasks, err := taskqueue.Lease(c, maxPullTasks, readingQueue, 60)
	if err != nil {
		return 0, err
	}
	if len(tasks) == 0 {
		return 0, nil
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

	return len(tasks), nil
}

func consumeInput(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	for {
		n, err := processBatch(c)
		if err != nil {
			showError(c, w, "Error processing batch: "+err.Error(), 500)
			return
		}
		c.Infof("Processed %v items", n)
		if n == 0 {
			break
		}
	}

	w.WriteHeader(204)
}
