package main

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/warthog618/go-gpiocdev"
)

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

// watchButton monitors a push button connected to the given GPIO offset
// (offset dentro do chip) and calls onPress() whenever it is pressed.
//
// Usa a interface de dispositivo de caracteres (/dev/gpiochip0) via libgpiod,
// que SUPORTA pull-up interno por software (resolve o pino "flutuando" sem
// resistor externo). Requer kernel Linux >= 5.5 (padrão no Armbian moderno).
//
// O Orange Pi PC Plus expõe o PA7 como offset 7 do gpiochip0.
func watchButton(offset int, onPress func()) {
	const chipName = "gpiochip0"

	// Liberta o pino do sysfs antes de usar libgpiod (evita "device busy").
	tryReleaseSysfs(offset)

	line, err := gpiocdev.RequestLine(
		chipName,
		offset,
		gpiocdev.AsInput,    // entrada
		gpiocdev.WithPullUp, // pull-up interno: solto = HIGH, apertado = LOW
	)
	if err != nil {
		log.Printf("GPIO %d (%s): falha ao solicitar linha: %v "+
			"(precisa de root/container privileged)", offset, chipName, err)
		return
	}
	defer line.Close()

	log.Printf("GPIO %d (%s): botão pronto (pull-up interno habilitado)", offset, chipName)

	// Debounce por estabilidade: só registra a transição depois que o novo
	// estado permanece ESTÁVEL por estabilidadeMin (mascara ruído/rebote).
	const estabilidadeMin = 100 * time.Millisecond

	var prevLow = true
	lastChange := time.Now()
	var pending bool

	for {
		val, err := line.Value()
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		// Com pull-up, o botão (para GND) gera LOW quando pressionado.
		low := val == 0

		if low == prevLow {
			// Estado estável: se estava em transição pendente e já passou o
			// tempo, confirma agora.
			if pending && time.Since(lastChange) >= estabilidadeMin {
				if low {
					log.Printf("Botão pressionado (GPIO %d)", offset)
					onPress()
				} else {
					log.Printf("Botão solto (GPIO %d)", offset)
				}
				pending = false
			}
		} else {
			// Estado mudou: inicia/atualiza a janela de estabilização.
			lastChange = time.Now()
			prevLow = low
			pending = true
		}

		time.Sleep(10 * time.Millisecond)
	}
}
