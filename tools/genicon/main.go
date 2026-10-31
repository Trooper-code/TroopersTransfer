// Command genicon draws the app icon (assets/icon.png). It only needs to be
// re-run if the design changes: go run ./tools/genicon
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

const size = 512

func main() {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	top := color.NRGBA{0x1f, 0x7a, 0x5c, 0xff}    // deep green
	bottom := color.NRGBA{0x0f, 0x4c, 0x81, 0xff} // deep blue
	radius := 104.0

	for y := 0; y < size; y++ {
		t := float64(y) / size
		bg := lerp(top, bottom, t)
		for x := 0; x < size; x++ {
			a := roundedRectCoverage(float64(x)+0.5, float64(y)+0.5, 16, 16, size-16, size-16, radius)
			if a <= 0 {
				continue
			}
			c := bg
			// arrows: up-arrow left, down-arrow right
			if v := arrowCoverage(float64(x), float64(y), 190, true) + arrowCoverage(float64(x), float64(y), 322, false); v > 0 {
				c = lerp(c, color.NRGBA{0xff, 0xff, 0xff, 0xff}, math.Min(v, 1))
			}
			c.A = uint8(255 * a)
			img.SetNRGBA(x, y, c)
		}
	}
	f, err := os.Create("assets/icon.png")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}

// arrowCoverage returns ~1 inside a vertical arrow centred on cx.
func arrowCoverage(x, y, cx float64, up bool) float64 {
	const (
		shaftHalf = 26.0
		headHalf  = 78.0
		topY      = 110.0
		bottomY   = 402.0
		headLen   = 100.0
	)
	if !up {
		y = topY + bottomY - y // mirror vertically
	}
	dx := math.Abs(x - cx)
	// head: triangle from topY (tip) to topY+headLen
	if y >= topY && y <= topY+headLen {
		half := (y - topY) / headLen * headHalf
		return clamp(half - dx + 0.5)
	}
	if y > topY+headLen && y <= bottomY {
		return clamp(shaftHalf - dx + 0.5)
	}
	return 0
}

func roundedRectCoverage(x, y, x0, y0, x1, y1, r float64) float64 {
	cx := math.Max(x0+r, math.Min(x, x1-r))
	cy := math.Max(y0+r, math.Min(y, y1-r))
	d := math.Hypot(x-cx, y-cy)
	return clamp(r - d + 0.5)
}

func clamp(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func lerp(a, b color.NRGBA, t float64) color.NRGBA {
	m := func(p, q uint8) uint8 { return uint8(float64(p) + (float64(q)-float64(p))*t) }
	return color.NRGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), 0xff}
}
