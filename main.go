package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
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

	// Captura sinais de parada (docker stop envia SIGTERM)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)

	done := make(chan struct{})

	go func() {
		<-sig
		// Escreve mensagem de parada na tela antes de sair
		oled.Clear()
		oled.Text(0, 13, "OLED PARADO")
		oled.Text(0, 39, time.Now().Format("02/01 15:04:05"))
		oled.Show()
		log.Println("OLED parado - mensagem exibida na tela")
		close(done)
	}()

	prev, _ := readCPU()

	for {

		select {
		case <-done:
			return
		default:
		}

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
