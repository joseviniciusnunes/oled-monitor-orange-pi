package main

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"oled/ssd1306"
)

// containerInfo representa um container Docker em execução.
type containerInfo struct {
	Name    string  // nome do container
	MemMB   float64 // uso de memória em MB
	Restart int     // quantidade de restarts
}

// parseSizeBytes converte um tamanho humano do docker ("512KiB", "15.6MiB",
// "1.2GiB") em MB (float). Usa unidades binárias (MiB = 2^20 bytes).
func parseSizeBytes(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Extrai o prefixo numérico (aceita vírgula como separador decimal).
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.' || s[i] == ',') {
		i++
	}
	num, err := strconv.ParseFloat(strings.Replace(s[:i], ",", ".", 1), 64)
	if err != nil {
		return 0
	}
	switch strings.TrimSpace(s[i:]) {
	case "KiB":
		return num / 1024
	case "MiB":
		return num
	case "GiB":
		return num * 1024
	case "TiB":
		return num * 1024 * 1024
	case "kB":
		return num / 1000
	case "MB":
		return num
	case "GB":
		return num * 1000
	default:
		// Valor já em bytes (docker usa B para valores pequenos).
		return num / (1024 * 1024)
	}
}

// runningContainers retorna os containers em execução com nome, uso de memória
// (MB) e quantidade de restarts. Em caso de erro (Docker indisponível), retorna
// um slice vazio.
func runningContainers() []containerInfo {
	// 1) Nome e uso de memória de cada container em execução.
	outStats, err := exec.Command("docker", "stats", "--no-stream",
		"--format", "{{.Name}}|{{.MemUsage}}").Output()
	if err != nil {
		return nil
	}

	type statLine struct {
		name string
		mem  float64
	}
	var stats []statLine
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(outStats)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimPrefix(strings.TrimSpace(parts[0]), "/")
		// MemUsage vem como "15.6MiB / 1.95GiB": pegamos a parte usada.
		used := strings.TrimSpace(strings.SplitN(parts[1], "/", 2)[0])
		stats = append(stats, statLine{name: name, mem: parseSizeBytes(used)})
		names = append(names, name)
	}

	// 2) Quantidade de restarts via docker inspect (por nome).
	restarts := map[string]int{}
	if len(names) > 0 {
		args := append([]string{"inspect", "--format", "{{.Name}}|{{.RestartCount}}"}, names...)
		if outInsp, err := exec.Command("docker", args...).Output(); err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(outInsp)), "\n") {
				parts := strings.SplitN(line, "|", 2)
				if len(parts) != 2 {
					continue
				}
				n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
				if err != nil {
					continue
				}
				restarts[strings.TrimPrefix(strings.TrimSpace(parts[0]), "/")] = n
			}
		}
	}

	out := make([]containerInfo, 0, len(stats))
	for _, s := range stats {
		out = append(out, containerInfo{
			Name:    s.name,
			MemMB:   s.mem,
			Restart: restarts[s.name],
		})
	}

	// Ordena por nome para uma exibição estável.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Name < out[j-1].Name; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}

	return out
}

// truncateRunes trunca s para no máximo n caracteres (rune), preservando o
// início. Usado para nomes de container que não cabem na largura do display.
func truncateRunes(s string, n int) string {
	if n < 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// containerRows é quantos containers cabem na tela (128x64, fonte 7x13).
const containerRows = 4

// containerLineChars é o máximo de caracteres por linha (128px / 7px por char,
// com 1px de folga). Usado para limitar o nome sem esconder as informações.
const containerLineChars = 17

// scrollInterval define a velocidade da rolagem automática (3s por porta).
const scrollInterval = 3 * time.Second

// drawContainerList desenha a lista de containers em execução com o uso de
// memória (MB) e quantidade de restarts, exibindo `containerRows` por porta.
// `offset` controla a rolagem: se houver mais containers que cabem, a lista
// rola automaticamente a cada `scrollInterval`, reiniciando do topo.
// Retorna o offset atualizado (para a rolagem persistir entre frames).
func drawContainerList(oled *ssd1306.Display, containers []containerInfo, offset int, lastScroll time.Time, now time.Time) (int, time.Time) {
	if len(containers) > containerRows && now.Sub(lastScroll) >= scrollInterval {
		// Rola uma posição por vez; ao chegar no fim, volta ao topo.
		offset = (offset + 1) % (len(containers) - containerRows + 1)
		lastScroll = now
	}

	oled.Clear()
	for i := 0; i < containerRows; i++ {
		idx := offset + i
		if idx >= len(containers) {
			break
		}
		c := containers[idx]
		// Número sequencial (estável mesmo com rolagem): "1. ", "2. ", etc.
		seq := fmt.Sprintf("%d. ", idx+1)
		// Formata primeiro as informações (à direita) para não serem
		// escondidas por um nome longo: "47M R3".
		suffix := fmt.Sprintf(" %s R%d", formatMB(c.MemMB), c.Restart)
		// Trunca SOMENTE o nome para caber no espaço restante da linha,
		// garantindo que a memória e os restarts sempre fiquem visíveis.
		name := truncateRunes(c.Name, containerLineChars-len(seq)-len(suffix))
		// Fallback de segurança no comprimento total da linha.
		line := truncateRunes(seq+name+suffix, containerLineChars)
		oled.Text(0, i*13, line)
	}
	return offset, lastScroll
}

// formatMB formata um valor em MB de forma compacta: <10MB usa uma casa
// decimal, >=10MB inteiro. Ex.: 3.4MB -> "3M", 512 -> "512M".
func formatMB(mb float64) string {
	if mb < 10 {
		return fmt.Sprintf("%.1fM", mb)
	}
	return fmt.Sprintf("%.0fM", mb)
}

// watchContainerRestarts roda em segundo plano (time.Ticker) e verifica, a cada
// `interval`, se algum container em execução tem restart > 0 (reiniciou).
//
// Como é dirigido por ticker (e não por sleep seguido de checagem do canal),
// o canal `resumeCheck` é servido imediatamente quando um alerta é dispensado.
// O canal `done` encerra o monitor quando o programa para.
//
// Ao encontrar um problema, envia o container pelo canal `alert` UMA VEZ e
// PAUSA a verificação (fica bloqueado aguardando `resumeCheck`) — a tela fica
// ligada com o aviso até alguém dispensar manualmente. O restart count já
// notificado é lembrado (mapa `seen`): ao dispensar, o MESMO problema não
// re-dispara — apenas um NOVO restart (count maior) volta a alertar.
func watchContainerRestarts(interval time.Duration, resumeCheck <-chan struct{}, done <-chan struct{}, alert chan<- containerInfo) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// seen guarda o último restart count já notificado (ou reconhecido) por
	// container. Um alerta só dispara quando o count AUMENTA em relação ao
	// valor já visto: ao dispensar um aviso, o mesmo problema não reprovoca
	// imediatamente — somente um NOVO restart volta a alertar.
	seen := make(map[string]int)

	for {
		select {
		case <-resumeCheck:
			// Alerta dispensado: retoma a verificação periodicamente.
			log.Println("Retomando verificação de restarts")
			ticker.Reset(interval)

		case <-done:
			return

		case <-ticker.C:
			all := runningContainers()
			var alertTarget *containerInfo
			for i := range all {
				c := &all[i]
				// Só alerta em restart NOVO (count maior que o já reconhecido).
				if c.Restart > seen[c.Name] {
					seen[c.Name] = c.Restart
					log.Printf("Container %s reiniciou %d vez(es) - acionando alerta", c.Name, c.Restart)
					alertTarget = c
					break
				}
			}
			if alertTarget != nil {
				select {
				case alert <- *alertTarget:
				default:
				}
				// Pausa a verificação: a tela fica ligada para verificação
				// manual. Aguarda o usuário dispensar (resumeCheck) antes de
				// continuar o loop com um novo intervalo.
				select {
				case <-resumeCheck:
					log.Println("Alerta dispensado - retomando verificação de restarts")
				case <-done:
					return
				}
				ticker.Reset(interval)
			}
		}
	}
}

// drawProblem desenha o aviso de problema na tela: um cabeçalho "Problem"
// destacado com o nome do container que reiniciou e a data/hora do alerta.
// A tela fica ligada neste aviso até alguém apertar o botão para dispensar.
func drawProblem(oled *ssd1306.Display, c containerInfo, at time.Time) {
	oled.Clear()
	// Cabeçalho de alerta (destacado no topo).
	oled.Text(0, 0, "Problem!")
	// Nome do container que reiniciou (truncado p/ caber na largura).
	oled.Text(0, 13, truncateRunes(c.Name, containerLineChars))
	// Quantidade de restarts detectada.
	oled.Text(0, 26, fmt.Sprintf("Restarts: %d", c.Restart))
	// Data e hora em que a verificação detectou o problema.
	oled.Text(0, 39, at.Format("02/01 15:04:05"))
	// Dica de como dispensar o alerta.
	oled.Text(0, 52, "Btn p/ voltar aos servicos")
}
