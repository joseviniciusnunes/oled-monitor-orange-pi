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

// mostra a mensagem inicial na tela
func splash(oled *ssd1306.Display) {
	oled.Clear()
	oled.Text(0, 13, "MONITOR INICIADO")
	oled.Text(0, 39, time.Now().Format("02/01 15:04:05"))
	oled.Show()
}

// desliga o painel (economia de energia)
func showOff(oled *ssd1306.Display) {
	oled.Clear()
	oled.Show()
	oled.Off()
}

// desenha a linha extra (y=48) que rotaciona a cada tick
func drawBottom(bottomIdx int) string {
	switch bottomIdx % 3 {
	case 0:
		return "Containers: " + containerCount()
	case 1:
		return "Up Time: " + uptime()
	default:
		return "Temp: " + cpuTemp()
	}
}

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
		oled.Text(0, 13, "MONITOR PARADO")
		oled.Text(0, 39, time.Now().Format("02/01 15:04:05"))
		oled.On()
		oled.Show()
		log.Println("OLED parado - mensagem exibida na tela")
		close(done)
	}()

	go splash(oled)

	// ---- Controle de energia via botão físico ----
	// O botão liga o LCD por 1 minuto; depois desliga novamente.
	//
	// GPIO recomendado: PA7 -> pino físico 29 do header.
	// Offset no gpiochip0 = (letra do banco - 1)*32 + pin = (1-1)*32 + 7 = 7.
	const gpioBotao = 7
	// Canal BUFFERED (cap 1): o aperto pode acontecer durante o time.Sleep do
	// loop principal; sem buffer ele seria descartado e a tela não ligaria.
	button := make(chan struct{}, 1)
	go watchButton(gpioBotao, func() {
		select {
		case button <- struct{}{}:
		default:
		}
	})

	// Estados de energia
	displayOn := true        // começa ligado
	screenOnAt := time.Now() // referência para marcar o início da sessão

	// Índice da informação exibida na última linha
	bottomIdx := 0
	// Controle para trocar a cada 3 segundos
	lastSwap := time.Now()

	prev, _ := readCPU()

	for {
		select {
		case <-button:
			// Botão pressionado -> liga (ou reinicia o timer de 1 minuto)
			displayOn = true
			screenOnAt = time.Now()
			oled.On()
			log.Println("Display ligado pelo botão (1 minuto)")
		case <-done:
			return
		default:
		}

		// Desliga automaticamente 1 minuto depois do último toque
		if displayOn && time.Since(screenOnAt) >= 1*time.Minute {
			displayOn = false
			showOff(oled)
			log.Println("Display desligado automaticamente (1 minuto sem pressionar)")
		}

		// Se o botão for pressionado durante o sleep, sai sem renderizar
		select {
		case <-done:
			return
		default:
		}

		// Só atualiza as métricas com a tela ligada
		if displayOn {
			now, _ := readCPU()
			cpu := cpuUsage(prev, now)
			prev = now

			ram, _ := readRAM()
			disk, _ := readDisk()
			ip := getIP()

			if time.Since(lastSwap) >= 3*time.Second {
				bottomIdx++
				lastSwap = time.Now()
			}

			oled.Clear()
			oled.Text(0, 0, fmt.Sprintf("CPU: %.0f%%", cpu))
			oled.Text(72, 0, fmt.Sprintf("RAM: %.0f%%", ram))
			oled.Text(0, 12, fmt.Sprintf("%.1f/%.1fG %.0f%%", disk.UsedGB, disk.TotalGB, disk.UsedGB*100/disk.TotalGB))
			oled.Text(0, 24, "IP: "+ip)
			oled.Text(0, 36, time.Now().Format("02/01 15:04:05"))

			// Linha extra (y=48) rotaciona a cada 3 segundos
			oled.Text(0, 48, drawBottom(bottomIdx))

			if err := oled.Show(); err != nil {
				log.Fatal(err)
			}
		}

		time.Sleep(1 * time.Second)
	}
}
