package ssd1306

import (
	"github.com/d2r2/go-i2c"
)

const (
	Width  = 128
	Height = 64
)

type Display struct {
	dev *i2c.I2C
	buffer [Width * Height / 8]byte
}

func Open(bus int, addr uint8) (*Display, error) {
	dev, err := i2c.NewI2C(addr, bus)
	if err != nil {
		return nil, err
	}

	d := &Display{dev: dev}
	return d, d.Init()
}

func (d *Display) Close() {
	d.dev.Close()
}

func (d *Display) cmd(cmds ...byte) error {
	buf := append([]byte{0x00}, cmds...)
	_, err := d.dev.WriteBytes(buf)
	return err
}

func (d *Display) data(data []byte) error {
	buf := append([]byte{0x40}, data...)
	_, err := d.dev.WriteBytes(buf)
	return err
}

func (d *Display) Init() error {
	return d.cmd(
		0xAE,
		0x20, 0x00,
		0xA1,
		0xC8,
		0xA6,
		0xA4,
		0xAF,
	)
}

func (d *Display) Clear() {
	for i := range d.buffer {
		d.buffer[i] = 0
	}
}

func (d *Display) Pixel(x, y int, on bool) {
	if x < 0 || x >= Width || y < 0 || y >= Height {
		return
	}

	i := x + (y/8)*Width
	b := byte(1 << (y % 8))

	if on {
		d.buffer[i] |= b
	} else {
		d.buffer[i] &^= b
	}
}

func (d *Display) Show() error {
	for page := 0; page < 8; page++ {

		if err := d.cmd(byte(0xB0+page), 0x00, 0x10); err != nil {
			return err
		}

		for i := 0; i < Width; i += 16 {
			if err := d.data(d.buffer[page*Width+i : page*Width+i+16]); err != nil {
				return err
			}
		}
	}

	return nil
}
