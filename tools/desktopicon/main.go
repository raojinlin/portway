// Generate the desktop icon from simple vector geometry, without image tooling.
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

func segment(x, y, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := math.Max(0, math.Min(1, ((x-ax)*dx+(y-ay)*dy)/(dx*dx+dy*dy)))
	return math.Hypot(x-ax-t*dx, y-ay-t*dy)
}

func main() {
	if len(os.Args) != 2 {
		panic("usage: desktopicon <output.png>")
	}
	img := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			px, py := (float64(x)+0.5)/2, (float64(y)+0.5)/2
			qx, qy := math.Abs(px-256)-156, math.Abs(py-256)-156
			d := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - 64
			alpha := math.Max(0, math.Min(1, 0.5-d))
			if alpha == 0 {
				continue
			}
			c := color.NRGBA{R: 23, G: 43, B: 53, A: uint8(255 * alpha)}
			upper := math.Min(segment(px, py, 145, 190, 365, 190), math.Min(segment(px, py, 313, 138, 365, 190), segment(px, py, 313, 242, 365, 190)))
			lower := math.Min(segment(px, py, 147, 322, 367, 322), math.Min(segment(px, py, 147, 322, 199, 270), segment(px, py, 147, 322, 199, 374)))
			for _, layer := range []struct {
				distance float64
				colour   color.NRGBA
			}{{upper, color.NRGBA{96, 217, 180, 255}}, {lower, color.NRGBA{230, 238, 240, 255}}} {
				a := math.Max(0, math.Min(1, 14.5-layer.distance))
				c.R = uint8(float64(c.R)*(1-a) + float64(layer.colour.R)*a)
				c.G = uint8(float64(c.G)*(1-a) + float64(layer.colour.G)*a)
				c.B = uint8(float64(c.B)*(1-a) + float64(layer.colour.B)*a)
			}
			img.SetNRGBA(x, y, c)
		}
	}
	if err := os.MkdirAll(filepath.Dir(os.Args[1]), 0o755); err != nil {
		panic(err)
	}
	f, err := os.Create(os.Args[1])
	if err != nil {
		panic(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}
}
