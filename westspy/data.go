package westspy

import (
	"sort"
	"time"
)

type Reading struct {
	Serial    string
	Reading   float64
	Timestamp time.Time
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
