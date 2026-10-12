//go:build ignore

// Command appicon renders frontend/public/apple-touch-icon.png (180x180) — the
// home-screen icon iOS asks for and will not take as an SVG. It reproduces the
// favicon.svg / Logo.tsx artwork full-bleed (iOS applies its own rounded mask).
//
// The file is excluded from normal builds with `//go:build ignore`; regenerate
// the committed PNG from the repo root with:
//
//	go run frontend/tools/appicon/main.go
//
// Keep the artwork in sync with frontend/public/favicon.svg and
// frontend/src/Logo.tsx by hand if the mark changes.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

const size = 180

// The mark is authored against a 32x32 viewBox; scale it up to the canvas.
const scale = size / 32.0

func main() {
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Diagonal violet gradient, matching the SVG's linearGradient.
	topLeft := color.RGBA{0xa7, 0x8b, 0xfa, 0xff}
	bottomRight := color.RGBA{0x7c, 0x3a, 0xed, 0xff}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			t := float64(x+y) / (2.0 * (size - 1))
			img.SetRGBA(x, y, lerp(topLeft, bottomRight, t))
		}
	}

	// Three queue rows, each with the SVG's white opacity, then the amber dot.
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}
	drawRow(img, 7.5, 7.6, 17.0, 0.95, white)
	drawRow(img, 7.5, 14.0, 17.0, 0.72, white)
	drawRow(img, 7.5, 20.4, 10.0, 0.5, white)

	fillCircle(img, 24.0, 22.1, 2.9, color.RGBA{0xfb, 0xbf, 0x24, 0xff})

	f, err := os.Create("frontend/public/apple-touch-icon.png")
	if err != nil {
		fmt.Fprintln(os.Stderr, "appicon:", err)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fmt.Fprintln(os.Stderr, "appicon:", err)
		os.Exit(1)
	}
}

// drawRow fills a rounded row starting at (x, y) in viewBox units, width and a
// fixed height of 3.4, composited over the gradient at alpha.
func drawRow(img *image.RGBA, x, y, width, alpha float64, c color.RGBA) {
	c.A = uint8(alpha * 255)
	fillRoundRect(img, x, y, x+width, y+3.4, 1.7, c)
}

// fillRoundRect composites a rounded rectangle over img. Coordinates are in
// viewBox units and scaled to the canvas.
func fillRoundRect(img *image.RGBA, x0, y0, x1, y1, r float64, c color.RGBA) {
	x0, y0, x1, y1, r = x0*scale, y0*scale, x1*scale, y1*scale, r*scale
	for y := int(y0); y <= int(y1)+1; y++ {
		for x := int(x0); x <= int(x1)+1; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			dx := max(x0+r-px, px-(x1-r), 0)
			dy := max(y0+r-py, py-(y1-r), 0)
			if dx*dx+dy*dy <= r*r {
				blend(img, x, y, c)
			}
		}
	}
}

// fillCircle composites a filled circle over img (viewBox units).
func fillCircle(img *image.RGBA, cx, cy, r float64, c color.RGBA) {
	cx, cy, r = cx*scale, cy*scale, r*scale
	for y := int(cy - r); y <= int(cy+r)+1; y++ {
		for x := int(cx - r); x <= int(cx+r)+1; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			if (px-cx)*(px-cx)+(py-cy)*(py-cy) <= r*r {
				blend(img, x, y, c)
			}
		}
	}
}

// blend composites c (using its alpha) over the existing pixel.
func blend(img *image.RGBA, x, y int, c color.RGBA) {
	if !image.Pt(x, y).In(img.Bounds()) {
		return
	}
	a := float64(c.A) / 255
	old := img.RGBAAt(x, y)
	mix := func(src, dst uint8) uint8 {
		return uint8(float64(src)*a + float64(dst)*(1-a) + 0.5)
	}
	img.SetRGBA(x, y, color.RGBA{mix(c.R, old.R), mix(c.G, old.G), mix(c.B, old.B), 0xff})
}

func lerp(a, b color.RGBA, t float64) color.RGBA {
	mix := func(x, y uint8) uint8 {
		return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5)
	}
	return color.RGBA{mix(a.R, b.R), mix(a.G, b.G), mix(a.B, b.B), 0xff}
}
