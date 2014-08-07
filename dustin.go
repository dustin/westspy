package westspy

import (
	"net/http"

	"dustin"
)

func init() {
	http.HandleFunc("/~dustin/m2repo/", err410)
	http.HandleFunc("/diggwatch/", err410)
	http.HandleFunc("/rss/", err410)
	http.HandleFunc("/ispy/", err410)

	http.HandleFunc("/~dustin/", dustin.ServePage)
	http.HandleFunc("/_update/github/", dustin.UpdateFeeds)
}
