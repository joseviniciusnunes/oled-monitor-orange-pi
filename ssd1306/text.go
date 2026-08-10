package ssd1306

import (
	"image"
	"image/color"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

type framebuffer struct {
	d *Display
}

func (fb framebuffer) ColorModel() color.Model {
	return color.GrayModel
}

func (fb framebuffer) Bounds() image.Rectangle {
	return image.Rect(0, 0, Width, Height)
}

func (fb framebuffer) At(x, y int) color.Color {
	return color.Black
}

func (fb framebuffer) Set(x, y int, c color.Color) {
	r, g, b, _ := c.RGBA()

	if r|g|b != 0 {
		fb.d.Pixel(x, y, true)
	}
}

func (d *Display) Text(x, y int, txt string) {

	img := framebuffer{d}

	dr := font.Drawer{
		Dst:  img,
		Src:  image.White,
		Face: basicfont.Face7x13,
		Dot:  fixed.P(x, y+13),
	}

	dr.DrawString(txt)
}
