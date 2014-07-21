package westspy

import (
	"net/http"

	"house"
)

func init() {
	http.HandleFunc("/house/", house.Server)
	http.HandleFunc("/house/input/", house.HandleInput)
	http.HandleFunc("/cron/house/consume/", house.ConsumeInput)

	registerWarmup(house.Warmup)
}
