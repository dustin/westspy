package house

import (
	"sort"
	"time"
)

const keyFormat = "20060102150405"

type Reading struct {
	Serial    string
	Reading   float64
	Timestamp time.Time
}

func (r Reading) Key() string {
	return r.Timestamp.Format(keyFormat) + "_" + r.Serial
}

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

func (r Readings) Sort() {
	sort.Sort(r)
}
