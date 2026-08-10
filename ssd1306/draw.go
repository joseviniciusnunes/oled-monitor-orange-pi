package ssd1306

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (d *Display) Line(x0, y0, x1, y1 int, on bool) {
	dx := abs(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}

	dy := -abs(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}

	err := dx + dy

	for {
		d.Pixel(x0, y0, on)

		if x0 == x1 && y0 == y1 {
			break
		}

		e2 := err * 2

		if e2 >= dy {
			err += dy
			x0 += sx
		}

		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}
