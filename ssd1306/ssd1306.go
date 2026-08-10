package ssd1306

import (
	"github.com/d2r2/go-i2c"
)

const (
	Width  = 128
	Height = 64
)

type Display struct {
	dev    *i2c.I2C
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
	// Sequência de inicialização completa do SSD1306.
	// O comando de charge pump (0x8D, 0x14) é essencial para alimentar o
	// painel OLED — sem ele a tela pode não acender após queda de energia.
	return d.cmd(
		0xAE,       // Display OFF
		0xD5, 0x80, // Clock div (recomendado)
		0xA8, 0x3F, // Multiplex 1/64
		0xD3, 0x00, // Display offset 0
		0x40,       // Start line 0
		0x8D, 0x14, // Charge pump ON (alimentação do painel)
		0x20, 0x00, // Memory addressing mode: horizontal
		0xA1,       // Segment remap (segmento 127 p/ 0)
		0xC8,       // COM scan direction remapped
		0xDA, 0x12, // COM pins hardware config
		0x81, 0xCF, // Contrast
		0xD9, 0xF1, // Pre-charge period
		0xDB, 0x40, // VCOMH deselect level
		0xA4, // Output follows RAM (display on resume)
		0xA6, // Normal display (não invertido)
		0xAF, // Display ON
	)
}

func (d *Display) Clear() {
	for i := range d.buffer {
		d.buffer[i] = 0
	}
}

// On liga o painel OLED (Display ON, 0xAF). O conteúdo do framebuffer é mantido.
func (d *Display) On() error {
	return d.cmd(0xAF)
}

// Off desliga o painel OLED (Display OFF, 0xAE) para economizar energia.
func (d *Display) Off() error {
	return d.cmd(0xAE)
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
