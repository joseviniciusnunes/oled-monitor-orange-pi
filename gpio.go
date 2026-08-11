package main

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/warthog618/go-gpiocdev"
)

// Estado do botão compartilhado entre o handler de eventos (goroutine da lib)
// e o timer do toque longo (goroutine própria do time.AfterFunc).
type buttonState struct {
	mu          sync.Mutex
	line        *gpiocdev.Line
	onPress     func()
	onLongPress func()

	pressed   bool // botão atualmente pressionado (debounced)
	longFired bool
	longTimer *time.Timer

	lastEvent  time.Time // momento do ÚLTIMO evento de qualquer borda (debounce)
	pressStart time.Time // início da pressão atual (p/ duração mínima do toque curto)
}

// buttonDebounce é o PERÍODO DE SILÊNCIO exigido entre eventos de qualquer
// borda para aceitar o próximo. O bounce físico/elétrico do botão gera RAJADAS
// de bordas (ms); ao esperar a linha ficar em silêncio por este tempo, a rajada
// inteira é colapsada em UMA transição única, evitando múltiplos onPress.
const buttonDebounce = 60 * time.Millisecond

// minPressDuration é a duração mínima de uma pressão para contar como "toque
// curto". Pressões mais curtas que isso (bounce/percusão) são descartadas,
// evitando toggles indesejados e cliques "enfileirados".
const minPressDuration = 30 * time.Millisecond

const gpioBase = "/sys/class/gpio"

// tryReleaseSysfs libera o GPIO do sysfs (se estiver preso lá) para que o
// libgpiod (/dev/gpiochip0) possa requisitar a linha. O sysfs "segura" o pino
// e causa "device or resource busy" no RequestLine. Não é erro se não existir.
func tryReleaseSysfs(offset int) {
	exported := filepath.Join(gpioBase, "gpio"+strconv.Itoa(offset))
	if _, err := os.Stat(exported); err != nil {
		return // não estava exportado no sysfs
	}
	// Desexporta (libera) o pino via sysfs.
	if err := os.WriteFile(filepath.Join(gpioBase, "unexport"), []byte(strconv.Itoa(offset)), 0); err != nil {
		log.Printf("GPIO %d: aviso ao liberar sysfs: %v", offset, err)
	} else {
		log.Printf("GPIO %d: liberado do sysfs (unexport) para uso via libgpiod", offset)
		time.Sleep(200 * time.Millisecond) // pequena pausa para o kernel soltar
	}
}

// watchButton monitora um botão físico conectado ao GPIO dado (offset dentro
// do chip).
//
//   - onPress(): chamado em um toque curto (pressionado e solto em menos de
//     longPressDuration).
//   - onLongPress(): chamado quando o botão é segurado por longPressDuration
//     ou mais (usado para reboot).
//
// Usa a interface de dispositivo de caracteres (/dev/gpiochip0) via libgpiod
// com DETECÇÃO POR BORDA (eventos), em vez de polling a cada 10ms. Assim, o
// kernel avisa o programa apenas quando o pino muda de estado, e o processo
// fica BLOCKED (dormindo) esperando — desperdiçando ~0 de CPU quando o botão
// está parado (ideal para um monitor).
//
// Requer kernel Linux >= 5.5 (padrão no Armbian moderno). O debounce é feito
// no software, por rejeição de eventos muito próximos, para não depender do
// suporte de hardware (WithDebounce exige kernel >= 5.10).
//
// O Orange Pi PC Plus expõe o PA7 como offset 7 do gpiochip0.
func watchButton(offset int, onPress func(), onLongPress func()) {
	const chipName = "gpiochip0"

	// Liberta o pino do sysfs antes de usar libgpiod (evita "device busy").
	tryReleaseSysfs(offset)

	state := &buttonState{
		onPress:     onPress,
		onLongPress: onLongPress,
	}

	// WithBothEdges: recebe eventos nas duas bordas (LOW = pressionado,
	// HIGH = solto). O handler roda numa goroutine própria da lib.
	line, err := gpiocdev.RequestLine(
		chipName,
		offset,
		gpiocdev.AsInput,       // entrada
		gpiocdev.WithPullUp,    // pull-up interno: solto = HIGH,
		gpiocdev.WithBothEdges, // eventos nas duas bordas
		gpiocdev.WithEventHandler(state.handleEvent), // sem looping de CPU
	)
	if err != nil {
		log.Printf("GPIO %d (%s): falha ao solicitar linha: %v "+
			"(precisa de root/container privileged)", offset, chipName, err)
		return
	}
	state.line = line
	defer line.Close()

	log.Printf("GPIO %d (%s): botão pronto (monitor por eventos, ~0%% CPU)",
		offset, chipName)

	// Mantém a goroutine viva até o fim do programa. Não há loop ativo:
	// o handler é chamado pelo kernel (via epoll) apenas quando o pino muda.
	select {}
}

// handleEvent processa cada borda do GPIO. Com pull-up, o botão (para GND)
// gera LOW quando pressionado (borda de descida) e HIGH quando solto (borda
// de subida).
//
// O toque longo (4s) NÃO depende de novas bordas: ao pressionar, agendamos um
// time.AfterFunc que dispara o reboot se o botão continuar pressionado. Se o
// botão for solto antes, o timer é cancelado e o toque vira um toque curto.
func (s *buttonState) handleEvent(ev gpiocdev.LineEvent) {
	// O AfterFunc (toque longo) roda em goroutine própria e pode disparar
	// concorrente ao release, então protegemos o estado com um mutex.
	s.mu.Lock()
	defer s.mu.Unlock()

	// Debounce por PERÍODO DE SILÊNCIO: qualquer borda que chegue antes de
	// buttonDebounce desde o último evento é parte do bounce (rajada). Ignora
	// sem logar (evita inundar o log com "bounce ignorado" em rajadas).
	if !s.lastEvent.IsZero() && time.Since(s.lastEvent) < buttonDebounce {
		return
	}
	s.lastEvent = time.Now()

	if ev.Type == gpiocdev.LineEventFallingEdge {
		// Borda de descida (pressionado). Ignora se já marcado como pressionado.
		if s.pressed {
			return
		}
		s.pressed = true
		s.longFired = false
		s.pressStart = time.Now()

		// Agenda o toque longo (reboot): se o botão continuar pressionado por
		// longPressDuration, o callback dispara; o release abaixo o cancela.
		s.longTimer = time.AfterFunc(longPressDuration, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			// Só dispara o reboot se o botão REALMENTE ainda estiver
			// pressionado. Re-lê o estado físico do pino: se a borda de
			// release se perdeu (bounce/atraso), cancelamos o reboot.
			v, err := s.line.Value()
			if err != nil || v == 1 { // 1 = HIGH = solto
				s.pressed = false
				s.longTimer = nil
				log.Printf("Botão: release perdido - reboot cancelado (GPIO %d)", s.line.Offset())
				return
			}
			if s.pressed {
				s.longFired = true
				s.onLongPress()
			}
			s.longTimer = nil
		})
		log.Printf("Botão pressionado (GPIO %d)", s.line.Offset())
		return
	}

	// Borda de subida (solto). Só importa se havia um pressionamento ativo.
	if !s.pressed {
		return
	}
	pressDur := time.Since(s.pressStart)
	s.pressed = false

	// Cancela o toque longo pendente: se o botão for solto antes de 4s,
	// o AfterFunc é desfeito (o callback até pode já estar agendado, mas
	// verificará `pressed` de novo e não fará nada).
	if s.longTimer != nil {
		s.longTimer.Stop()
		s.longTimer = nil
	}

	// Toque curto: solto antes de completar o toque longo. Exige duração
	// mínima (filtra bounce/percusão rápida).
	if !s.longFired && pressDur >= minPressDuration {
		log.Printf("Botão solto (toque curto) (GPIO %d)", s.line.Offset())
		s.onPress()
	}
}

// longPressDuration é o tempo de pressionamento para considerar "toque longo"
// (reboot). Mantido como constante de pacote para o handler consultar.
const longPressDuration = 4 * time.Second
