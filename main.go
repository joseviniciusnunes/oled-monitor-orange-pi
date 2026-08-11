package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	logger "github.com/d2r2/go-logger"

	"oled/ssd1306"
)

// desenha um retângulo com borda (contorno) no display
func drawRect(oled *ssd1306.Display, x0, y0, x1, y1 int) {
	oled.Line(x0, y0, x1, y0, true) // topo
	oled.Line(x0, y1, x1, y1, true) // base
	oled.Line(x0, y0, x0, y1, true) // esquerda
	oled.Line(x1, y0, x1, y1, true) // direita
}

// desenha um círculo (contorno) com centro (cx, cy) e raio r
// usando o algoritmo do ponto médio (midpoint circle).
func drawCircle(oled *ssd1306.Display, cx, cy, r int) {
	x := r
	y := 0
	err := 1 - r

	for x >= y {
		oled.Pixel(cx+x, cy+y, true)
		oled.Pixel(cx+y, cy+x, true)
		oled.Pixel(cx-y, cy+x, true)
		oled.Pixel(cx-x, cy+y, true)
		oled.Pixel(cx-x, cy-y, true)
		oled.Pixel(cx-y, cy-x, true)
		oled.Pixel(cx+y, cy-x, true)
		oled.Pixel(cx+x, cy-y, true)

		y++
		if err < 0 {
			err += 2*y + 1
		} else {
			x--
			err += 2*(y-x) + 1
		}
	}
}

// desenha um círculo PREENCHIDO com centro (cx, cy) e raio r.
// Para cada linha horizontal dentro do círculo, liga todos os pixels
// entre as bordas esquerda e direita.
func fillCircle(oled *ssd1306.Display, cx, cy, r int) {
	for y := -r; y <= r; y++ {
		// Meia-largura da linha na altura y (teorema de Pitágoras)
		dx := int(float64(r) * math.Sqrt(1-float64(y*y)/float64(r*r)))
		for x := -dx; x <= dx; x++ {
			oled.Pixel(cx+x, cy+y, true)
		}
	}
}

// desenha um triângulo PREENCHIDO a partir de 3 vértices.
// Percorre cada linha horizontal entre o topo e a base e liga os pixels
// entre as bordas esquerda e direita (interpolação linear).
func fillTriangle(oled *ssd1306.Display, x0, y0, x1, y1, x2, y2 int) {
	// Ordena os vértices por y (topo, meio, base)
	if y0 > y1 {
		x0, y0, x1, y1 = x1, y1, x0, y0
	}
	if y0 > y2 {
		x0, y0, x2, y2 = x2, y2, x0, y0
	}
	if y1 > y2 {
		x1, y1, x2, y2 = x2, y2, x1, y1
	}

	// Interpola a borda esquerda e direita para cada linha y
	for y := y0; y <= y2; y++ {
		var xa, xb int
		if y < y1 {
			// Trecho superior: entre (x0,y0)-(x1,y1) e (x0,y0)-(x2,y2)
			xa = x0 + (x1-x0)*(y-y0)/(y1-y0+1)
			xb = x0 + (x2-x0)*(y-y0)/(y2-y0+1)
		} else {
			// Trecho inferior: entre (x1,y1)-(x2,y2) e (x0,y0)-(x2,y2)
			xa = x1 + (x2-x1)*(y-y1)/(y2-y1+1)
			xb = x0 + (x2-x0)*(y-y0)/(y2-y0+1)
		}
		if xa > xb {
			xa, xb = xb, xa
		}
		for x := xa; x <= xb; x++ {
			oled.Pixel(x, y, true)
		}
	}
}

// desenha o logo da laranja (fruta) preenchido, ocupando toda a tela
func drawLogo(oled *ssd1306.Display) {
	// Laranja preenchida, centralizada (um pouco menor para sobrar espaço
	// para o talo e a folha no topo)
	fillCircle(oled, 64, 38, 26)
	// Talo no topo da laranja
	oled.Line(64, 12, 64, 4, true)
	// Folha preenchida à direita do talo (bem visível)
	fillTriangle(oled, 64, 4, 64, 12, 88, 8)
}

// mostra a mensagem inicial na tela (logo da laranja)
func splash(oled *ssd1306.Display) {
	oled.Clear()
	drawLogo(oled)
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

// rebootSystem exibe a mensagem de reboot na tela e reinicia o sistema.
// Deve ser chamado a partir do loop principal (dono do display) para evitar
// corrida no acesso ao I2C.
//
// O container roda com privileged:true e pid:host, então o nsenter -t 1
// executa o reboot no namespace de PID do HOST (PID 1 = init do host), e não
// no namespace do container. Sem isso, o `reboot` reiniciaria apenas o
// container.
func rebootSystem(oled *ssd1306.Display, rebooting *atomic.Bool) {
	// Marca que um reboot está em andamento para que o signal handler
	// (SIGTERM ao desligar o host) não sobrescreva a mensagem "REBOOT".
	rebooting.Store(true)

	oled.Clear()
	oled.Text(0, 13, "Rebooting...")
	oled.Text(0, 39, time.Now().Format("02/01 15:04:05"))
	oled.On()
	oled.Show()
	log.Println("Reboot solicitado - mensagem exibida na tela")

	// Pequena pausa para a mensagem aparecer antes de reiniciar.
	time.Sleep(2 * time.Second)

	// Entra nos namespaces do host (PID 1) e executa o reboot do sistema.
	cmd := exec.Command("nsenter", "-t", "1", "-m", "-u", "-i", "-n", "-p", "reboot")
	if err := cmd.Run(); err != nil {
		log.Printf("Falha ao executar reboot: %v", err)
	}
}

func main() {
	logger.ChangePackageLogLevel("i2c", logger.FatalLevel)

	log.Println("Tela OLED SSD1306 iniciada!")

	oled, err := ssd1306.Open(0, 0x3C)
	if err != nil {
		log.Fatal(err)
	}
	defer oled.Close()

	// Captura sinais de parada (docker stop envia SIGTERM)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)

	done := make(chan struct{})

	// Flag atômico: quando true, um reboot está em andamento e a tela não deve
	// ser sobrescrita pela mensagem de parada (MONITOR PARADO).
	var rebooting atomic.Bool

	go func() {
		<-sig
		// Se um reboot está em andamento, não sobrescreve a mensagem "REBOOT".
		if rebooting.Load() {
			log.Println("OLED parado durante reboot - mantendo mensagem REBOOT na tela")
			close(done)
			return
		}
		// Escreve mensagem de parada na tela antes de sair
		oled.Clear()
		oled.Text(0, 13, "No Data")
		oled.Text(0, 39, time.Now().Format("02/01 15:04:05"))
		oled.On()
		oled.Show()
		log.Println("OLED parado - mensagem exibida na tela")
		close(done)
	}()

	// Exibe o logo do Orange Pi por alguns segundos antes de iniciar o loop
	// de métricas (síncrono, para não ser sobrescrito imediatamente).
	splash(oled)
	time.Sleep(3 * time.Second)

	// ---- Controle de energia via botão físico ----
	// O botão liga o LCD por 1 minuto; depois desliga novamente.
	//
	// GPIO recomendado: PA7 -> pino físico 29 do header.
	// Offset no gpiochip0 = (letra do banco - 1)*32 + pin = (1-1)*32 + 7 = 7.
	const gpioBotao = 7
	// Canal BUFFERED (cap 1): o aperto pode acontecer durante o time.Sleep do
	// loop principal; sem buffer ele seria descartado e a tela não ligaria.
	button := make(chan struct{}, 1)
	// Canal para sinalizar reboot (toque longo). O reboot é executado no loop
	// principal, que é o dono do display, para evitar corrida no acesso ao I2C.
	reboot := make(chan struct{}, 1)
	go watchButton(gpioBotao, func() {
		select {
		case button <- struct{}{}:
		default:
		}
	}, func() {
		// Toque longo (4s) -> reinicia o sistema.
		select {
		case reboot <- struct{}{}:
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
		case <-reboot:
			// Toque longo (4s) -> exibe a mensagem e reinicia o sistema.
			// Executado aqui (loop principal) para garantir que a mensagem
			// apareça na tela antes do reboot.
			rebootSystem(oled, &rebooting)
			return
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
