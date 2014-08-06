package westspy

import (
	"net/http"

	"dustin"
)

func init() {
	http.Handle("/~dustin/m2repo/", serveError("Go away", 410))
	http.Handle("/diggwatch/", serveError("Gone missing", 410))
	http.Handle("/rss/", serveError("Gone missing", 410))
	http.Handle("/ispy/", serveError("Gone missing", 410))

	http.HandleFunc("/~dustin/", dustin.ServePage)
	http.HandleFunc("/_update/github/", dustin.UpdateGithub)
}
