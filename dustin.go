package westspy

import (
	"net/http"

	"dustin"
)

func init() {
	http.HandleFunc("/~dustin/", dustin.ServePage)
	http.HandleFunc("/_update/github/", dustin.UpdateGithub)
}
