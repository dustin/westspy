package westspy

import (
	"io"
	"io/ioutil"
	"net/http"

	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
)

func init() {
	http.HandleFunc("/gitmirror/", handleGitmirror)
}

func handleGitmirror(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	n, err := io.Copy(ioutil.Discard, r.Body)
	log.Infof(c, "Read %v bytes with err=%v", n, err)
	w.WriteHeader(201)
}
