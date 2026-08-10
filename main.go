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

	// Índice da informação exibida na última linha
	bottomIdx := 0
	// Controle para trocar a cada 3 segundos
	lastSwap := time.Now()

	for {

		select {
		case <-done:
			return
		default:
		}

		time.Sleep(1 * time.Second)

		// Se o sinal de parada chegou durante o sleep, sai sem renderizar
		select {
		case <-done:
			return
		default:
		}

		if time.Since(lastSwap) >= 3*time.Second {
			bottomIdx++
			lastSwap = time.Now()
		}

		now, _ := readCPU()
		cpu := cpuUsage(prev, now)
		prev = now

		ram, _ := readRAM()

		disk, _ := readDisk()

		ip := getIP()

		oled.Clear()

		oled.Text(0, 0, fmt.Sprintf("CPU: %.0f%%", cpu))
		oled.Text(72, 0, fmt.Sprintf("RAM: %.0f%%", ram))
		oled.Text(0, 12, fmt.Sprintf("%.1f/%.1fG %.0f%%", disk.UsedGB, disk.TotalGB, disk.UsedGB*100/disk.TotalGB))
		oled.Text(0, 24, "IP: "+ip)
		oled.Text(0, 36, time.Now().Format("02/01 15:04:05"))

		// Linha extra (y=48) rotaciona a cada 3 segundos
		switch bottomIdx % 3 {
		case 0:
			oled.Text(0, 48, "Containers: "+containerCount())
		case 1:
			oled.Text(0, 48, "Up Time: "+uptime())
		case 2:
			oled.Text(0, 48, "Temp: "+cpuTemp())
		}

		if err := oled.Show(); err != nil {
			log.Fatal(err)
		}
	}
}
