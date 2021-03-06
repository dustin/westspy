package house

import (
	"encoding/json"
	"math"
	"os"
)

type Rect struct {
	X, Y, W, H int
}

type Point struct {
	X, Y int
}

// Room definition.
type Room struct {
	SN      string
	Max     float64
	Min     float64
	Rect    Rect
	Therm   Point
	Spark   Rect
	Reading Point

	Latest float64

	Name string
}

// SparkWidth returns the width of a sparkline defined for this room.
func (r *Room) SparkWidth() int {
	if r.Spark.W != 0 {
		return r.Spark.W
	}
	return r.Rect.W
}

// HouseConfig holds the configuration for all the house things.
type HouseConfig struct {
	Dims struct {
		H int
		W int
	}
	MaxRelevantDistance float64
	Rooms               map[string]*Room
	bySerial            map[string]*Room
	Colorize            []string
}

// NameOf returns the name for this Serial Number
func (hc *HouseConfig) NameOf(sn string) string {
	r := hc.BySerial(sn)
	if r == nil {
		return sn
	}
	return r.Name
}

// BySerial gets a room by serial number.
func (hc *HouseConfig) BySerial(sn string) *Room {
	return hc.bySerial[sn]
}

// LoadConfig loads the from a JSON file.
func LoadConfig(path string) (conf HouseConfig, err error) {
	f, err := os.Open(path)
	if err != nil {
		return conf, err
	}
	defer f.Close()

	err = json.NewDecoder(f).Decode(&conf)
	if err != nil {
		return
	}

	conf.bySerial = make(map[string]*Room)
	for k, r := range conf.Rooms {
		sn := r.SN
		if sn == "" {
			sn = k
		}
		conf.bySerial[sn] = r
		r.Latest = math.NaN()
		r.Name = k
	}
	return
}
