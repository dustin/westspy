package westspy

import (
	"io"
	"io/ioutil"
	"net/http"

	"appengine"
)

func init() {
	http.HandleFunc("/gitmirror/", handleGitmirror)
}

func handleGitmirror(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	n, err := io.Copy(ioutil.Discard, r.Body)
	c.Infof("Read %v bytes with err=%v", n, err)
	w.WriteHeader(201)
}
