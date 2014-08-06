package westspy

import (
	"bytes"
	"html/template"
	"io"
	"net/http"
	"sync"
)

var warmups []func(w http.ResponseWriter, req *http.Request)

var templates = template.Must(template.New("").ParseGlob("templates/*html"))

func init() {
	http.HandleFunc("/_ah/warmup", warmupHandler)
	http.HandleFunc("/", err404)
}

func serveError(msg string, status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, msg, status)
	})
}

func registerWarmup(f func(w http.ResponseWriter, req *http.Request)) {
	warmups = append(warmups, f)
}

func warmupHandler(w http.ResponseWriter, req *http.Request) {
	// Should probably actually check these and stuff...
	wg := &sync.WaitGroup{}
	for _, wh := range warmups {
		wg.Add(1)
		go func(wh http.HandlerFunc) {
			wg.Done()
			wh(w, req)
		}(wh)
	}
	wg.Wait()
}

func err404(w http.ResponseWriter, req *http.Request) {
	buf := &bytes.Buffer{}
	templates.ExecuteTemplate(buf, "404.html", nil)
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(404)
	io.Copy(w, buf)
}
