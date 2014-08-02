package westspy

import (
	"net/http"

	"dustin"
)

func init() {
	http.HandleFunc("/~dustin/", dustin.ServePage)
}
