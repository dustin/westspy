package house

import (
	"sort"
	"time"
)

const keyFormat = "20060102150405"

// A Reading represents a temperature reading at a point in time.
type Reading struct {
	Serial    string
	Reading   float64
	Timestamp time.Time
}

// Key is a unique key for this reading.
func (r Reading) Key() string {
	return r.Timestamp.Format(keyFormat) + "_" + r.Serial
}

// Readings is a sortable slice of Reading.
type Readings []Reading

func (r Readings) Len() int {
	return len(r)
}

func (r Readings) Less(i, j int) bool {
	return r[i].Timestamp.After(r[j].Timestamp)
}

func (r Readings) Swap(i, j int) {
	r[j], r[i] = r[i], r[j]
}

// Sort the readings.
func (r Readings) Sort() {
	sort.Sort(r)
}
