package house

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"sync"
	"time"

	_ "image/png"

	"code.google.com/p/draw2d/draw2d"
	"code.google.com/p/freetype-go/freetype"
	"code.google.com/p/freetype-go/freetype/truetype"

	"appengine"
	"appengine/memcache"
	"appengine/urlfetch"
)

var font *truetype.Font
var houseBase image.Image
var conf HouseConfig

func mustFetch(c appengine.Context, u string) io.ReadCloser {
	client := urlfetch.Client(c)
	res, err := client.Get(u)
	must(err)
	if res.StatusCode != 200 {
		panic("http error on " + u + ": " + res.Status)
	}
	return res.Body
}

var houseInitOnce sync.Once

func houseInit(c appengine.Context) {
	houseInitOnce.Do(func() {
		c.Infof("Initializing all the house things on " + appengine.InstanceID())

		base := "http://" + appengine.DefaultVersionHostname(c) + "/static/house/"

		wg := &sync.WaitGroup{}
		wg.Add(3)

		func() {
			defer wg.Done()
			fontr := mustFetch(c, base+"luximr.ttf")
			defer fontr.Close()
			fontBytes, err := ioutil.ReadAll(fontr)
			must(err)
			font, err = freetype.ParseFont(fontBytes)
			must(err)
		}()

		func() {
			defer wg.Done()
			pngr := mustFetch(c, base+"house.png")
			defer pngr.Close()
			var err error
			houseBase, _, err = image.Decode(pngr)
			must(err)
		}()

		func() {
			defer wg.Done()
			confr := mustFetch(c, base+"house.json")
			d := json.NewDecoder(confr)
			must(d.Decode(&conf))
		}()
		wg.Wait()
	})
}

func drawBox(i *image.NRGBA, room *Room) {
	draw.Draw(i, image.Rect(room.Rect.X-1, room.Rect.Y-1,
		room.Rect.X+room.Rect.W+1,
		room.Rect.Y+room.Rect.H+1),
		image.NewUniform(color.Black),
		image.Pt(room.Rect.X-1, room.Rect.Y-1),
		draw.Over)
}

func ifZero(a, b int) int {
	if a == 0 {
		return b
	}
	return a
}

func getFillColor(room *Room, reading, relevance float64) (rv color.NRGBA) {
	normal := room.Min + ((room.Max - room.Min) / 2.0)
	maxDifference := room.Max - room.Min

	difference := math.Abs(reading - normal)
	differencePercent := difference / maxDifference

	factor := 1.0 - relevance

	base := 255.0 - (255.0 * differencePercent)
	colorval := uint8(base + ((255 - base) * factor))

	rv.A = 255
	rv.R = uint8(colorval)
	rv.G = rv.R
	rv.B = rv.R
	if reading > normal {
		rv.R = 255
	} else {
		rv.B = 255
	}
	return
}

func fillGradient(img *image.NRGBA, room *Room, reading float64) {
	tx := ifZero(room.Therm.X, room.Rect.X+(room.Rect.W/2))
	ty := ifZero(room.Therm.Y, room.Rect.Y+(room.Rect.H/2))

	for i := 0; i < room.Rect.W; i++ {
		for j := 0; j < room.Rect.H; j++ {
			px, py := room.Rect.X+i, room.Rect.Y+j
			xd, yd := float64(px-tx), float64(py-ty)
			distance := math.Sqrt(xd*xd + yd*yd)
			relevance := 1.0 - (distance / conf.MaxRelevantDistance)
			if relevance < 0 {
				relevance = 0
			}

			img.Set(px, py, getFillColor(room, reading, relevance))
		}
	}
}

func fillSolid(i *image.NRGBA, room *Room, c color.Color) {
	draw.Draw(i, image.Rect(room.Rect.X, room.Rect.Y,
		room.Rect.X+room.Rect.W,
		room.Rect.Y+room.Rect.H),
		image.NewUniform(c),
		image.Pt(room.Rect.X, room.Rect.Y),
		draw.Over)
}

func fill(i *image.NRGBA, room *Room, reading float64) {
	switch {
	case reading < room.Min:
		fillSolid(i, room, color.NRGBA{0, 0, 255, 255})
	case reading > room.Max:
		fillSolid(i, room, color.NRGBA{255, 0, 0, 255})
	default:
		fillGradient(i, room, reading)
	}
}

func drawLabel(i draw.Image, room *Room, lbl string) {
	charwidth := 6
	charheight := 12

	x := ifZero(room.Reading.X, (room.Rect.X + (room.Rect.W / 2) -
		((len(lbl) * charwidth) / 2)))
	y := ifZero(room.Reading.Y, (room.Rect.Y +
		((room.Rect.H - charheight*2) / 2) - 12))

	c := freetype.NewContext()
	c.SetDPI(72)
	c.SetFont(font)
	c.SetFontSize(10)
	c.SetClip(image.Rect(x, y, x+room.Rect.W, y+charheight))
	c.SetDst(i)
	c.SetSrc(image.Black)

	pt := freetype.Pt(x, y+int(c.PointToFix32(10)>>8))
	c.DrawString(lbl, pt)
}

func drawSparklines(i *image.NRGBA, room *Room, roomReadings []*Reading) {
	// Not interested in plotting fewer than two points
	if len(roomReadings) < 2 {
		return
	}

	if len(roomReadings) > room.SparkWidth() {
		roomReadings = roomReadings[:room.SparkWidth()]
	}

	sparkh := ifZero(room.Spark.H, 20)
	sparkx := ifZero(room.Spark.X, room.Rect.X)
	sparky := ifZero(room.Spark.Y, (room.Rect.Y+room.Rect.H)-sparkh)

	high, low := math.SmallestNonzeroFloat64, math.MaxFloat64

	for _, r := range roomReadings {
		high = math.Max(high, r.Reading)
		low = math.Min(low, r.Reading)
	}

	if (high - low) < float64(sparkh) {
		avg := high - ((high - low) / 2.0)
		low = avg + (float64(sparkh) / 2.0)
		high = avg - (float64(sparkh) / 2.0)
	}

	for pos, r := range roomReadings {
		x := len(roomReadings) - pos + sparkx - 1
		heightPercent := (r.Reading - low) / (high - low)
		y := int((float64(sparky) + float64(sparkh)) - (float64(sparkh) * heightPercent))
		if y > sparky+sparkh {
			y = sparky + sparkh
		}
		if y < sparky {
			y = sparky
		}

		i.Set(x, y, color.Gray{127})
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func getReadings(c appengine.Context) map[string][]*Reading {
	rv := map[string][]*Reading{}

	current := map[string]float64{}
	_, err := memcache.JSON.Get(c, "current", &current)
	if err != nil {
		c.Warningf("Couldn't get current values from cache: %v", err)
		return rv
	}
	keys := []string{}

	for k := range current {
		keys = append(keys, "r-"+k)
	}
	cached, err := memcache.GetMulti(c, keys)
	if err != nil {
		c.Warningf("Couldn't get latest readings from cache: %v", err)
		for k, v := range current {
			rv[k] = []*Reading{&Reading{Reading: v}}
		}
		return rv
	}

	for k, vitem := range cached {
		v := []*Reading{}
		must(json.Unmarshal(vitem.Value, &v))
		rv[k[2:]] = v
	}
	return rv
}

func drawHouse(c appengine.Context) image.Image {
	i := image.NewNRGBA(houseBase.Bounds())

	alldata := getReadings(c)

	draw.Draw(i, houseBase.Bounds(), houseBase, image.Pt(0, 0), draw.Over)

	for _, roomName := range conf.Colorize {
		room := conf.Rooms[roomName]
		drawBox(i, room)
		roomReadings, ok := alldata[room.SN]
		if ok {
			reading := roomReadings[0].Reading
			fill(i, room, reading)
			drawLabel(i, room, fmt.Sprintf("%.2f", reading))
			drawSparklines(i, room, roomReadings)
		} else {
			fillSolid(i, room, color.White)
			drawLabel(i, room, "??.??")
		}
	}

	return i
}

const (
	houseImgKey = "houseimg"
	houseExpKey = "houseexp"
)

func Server(w http.ResponseWriter, req *http.Request) {
	c := appengine.NewContext(req)

	caches, err := memcache.GetMulti(c, []string{houseImgKey, houseExpKey})
	if err != nil {
		c.Warningf("Error getting stuff from memcache: %v", err)
		caches = map[string]*memcache.Item{}
	}

	if len(caches) == 2 {
		data := caches[houseImgKey].Value

		// Serve from cache
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "max-age=300")
		w.Header().Set("Expires", string(caches[houseExpKey].Value))
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))

		w.WriteHeader(200)
		w.Write(caches[houseImgKey].Value)
		return
	}

	houseInit(c)

	exptime := time.Now().Add(5 * time.Minute).UTC().Format(http.TimeFormat)

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=300")
	w.Header().Set("Expires", exptime)

	err = processInput(c)
	if err != nil {
		c.Warningf("Error processing batched data: %v.  Might be stale", err)
	}

	i := drawHouse(c)

	start := time.Now()
	buf := &bytes.Buffer{}
	png.Encode(buf, i)
	c.Debugf("Rebuild house image in %v", time.Since(start))
	data := buf.Bytes()

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()

		err := memcache.SetMulti(c, []*memcache.Item{
			&memcache.Item{
				Key:        houseExpKey,
				Value:      []byte(exptime),
				Expiration: time.Minute * 5,
			},
			&memcache.Item{
				Key:        houseImgKey,
				Value:      data,
				Expiration: time.Minute * 5,
			},
		})

		if err != nil {
			c.Warningf("Error setting image to cache: %v", err)
		}
	}()

	w.Header().Set("Content-Length", fmt.Sprint(len(data)))

	w.WriteHeader(200)
	w.Write(data)

	wg.Wait()
}

func drawTemp(i draw.Image, r Reading) {
	x1, y1 := float64(66), float64(65)

	// Translate the angle because we're a little crooked
	trans := -90.0

	// Calculate the angle based on the temperature
	angle := (r.Reading * 1.8) + trans
	// Calculate the angle in radians
	rad := ((angle / 360.0) * 2.0 * 3.14159265358979)
	// Find the extra points
	x2 := math.Sin(rad) * 39.0
	y2 := math.Cos(rad) * 39.0
	// Negate the y, we're upside-down
	y2 = -y2
	// Move over to the starting points.
	x2 += x1
	y2 += y1

	// Draw the line...
	gc := draw2d.NewGraphicContext(i)
	gc.MoveTo(x1, y1)
	gc.LineTo(x2, y2)
	gc.Stroke()

	// And label it
	c := freetype.NewContext()
	c.SetDPI(72)
	c.SetFont(font)
	c.SetFontSize(10)
	c.SetClip(i.Bounds())
	c.SetDst(i)
	c.SetSrc(image.Black)

	pt := freetype.Pt(52, 72+int(c.PointToFix32(10)>>8))
	c.DrawString(fmt.Sprintf("%.2f", r.Reading), pt)

}

func Warmup(w http.ResponseWriter, req *http.Request) {
	houseInit(appengine.NewContext(req))
	w.WriteHeader(204)
}
