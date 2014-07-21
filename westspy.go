package westspy

import (
	"net/http"
	"sync"
)

var warmups []func(w http.ResponseWriter, req *http.Request)

func init() {
	http.HandleFunc("/_ah/warmup", warmupHandler)
}

func registerWarmup(f func(w http.ResponseWriter, req *http.Request)) {
	warmups = append(warmups, f)
}

func warmupHandler(w http.ResponseWriter, req *http.Request) {
	// Should probably actually check these and stuff...
	wg := &sync.WaitGroup{}
	for _, wh := range warmups {
		wg.Add(1)
		func(wh http.HandlerFunc) {
			wg.Done()
			wh(w, req)
		}(wh)
	}
	wg.Wait()
}
