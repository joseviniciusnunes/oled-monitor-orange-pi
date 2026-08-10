package main

import (
	"fmt"
	"log"
	"time"

	logger "github.com/d2r2/go-logger"

	"oled/ssd1306"
)

func main() {
	logger.ChangePackageLogLevel("i2c", logger.FatalLevel)

	log.Println("Tela OLED SSD1306 iniciada!")

	oled, err := ssd1306.Open(1, 0x3C)
	if err != nil {
		log.Fatal(err)
	}
	defer oled.Close()

	prev, _ := readCPU()

	for {

		time.Sleep(1 * time.Second)

		now, _ := readCPU()
		cpu := cpuUsage(prev, now)
		prev = now

		ram, _ := readRAM()

		ip := getIP()

		oled.Clear()

		oled.Text(0, 0, fmt.Sprintf("CPU: %.0f%%", cpu))
		oled.Text(0, 13, fmt.Sprintf("RAM: %.0f%%", ram))
		oled.Text(0, 26, "IP: "+ip)
		oled.Text(0, 39, time.Now().Format("02/01 15:04:05"))

		if err := oled.Show(); err != nil {
			log.Fatal(err)
		}
	}
}
