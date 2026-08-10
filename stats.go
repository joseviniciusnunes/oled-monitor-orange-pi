package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// DiskInfo contém o total e o espaço usado (em GB)
type DiskInfo struct {
	TotalGB float64
	UsedGB  float64
}

type CPUStats struct {
	Idle  uint64
	Total uint64
}

func readCPU() (CPUStats, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return CPUStats{}, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return CPUStats{}, fmt.Errorf("erro lendo /proc/stat")
	}

	fields := strings.Fields(sc.Text())

	var values []uint64
	for _, s := range fields[1:] {
		v, _ := strconv.ParseUint(s, 10, 64)
		values = append(values, v)
	}

	var total uint64
	for _, v := range values {
		total += v
	}

	idle := values[3]
	if len(values) > 4 {
		idle += values[4]
	}

	return CPUStats{
		Idle:  idle,
		Total: total,
	}, nil
}

func cpuUsage(prev, now CPUStats) float64 {
	total := now.Total - prev.Total
	idle := now.Idle - prev.Idle

	if total == 0 {
		return 0
	}

	return float64(total-idle) * 100 / float64(total)
}

func readRAM() (float64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var total uint64
	var available uint64

	sc := bufio.NewScanner(f)

	for sc.Scan() {
		line := sc.Text()

		if strings.HasPrefix(line, "MemTotal:") {
			fmt.Sscanf(line, "MemTotal: %d kB", &total)
		}

		if strings.HasPrefix(line, "MemAvailable:") {
			fmt.Sscanf(line, "MemAvailable: %d kB", &available)
		}
	}

	if err := sc.Err(); err != nil {
		return 0, err
	}

	if total == 0 {
		return 0, fmt.Errorf("MemTotal não encontrado")
	}

	used := float64(total-available) * 100 / float64(total)
	return used, nil
}

// readDisk retorna o espaço total e livre (em GB) do disco
// Usa /hostroot (raiz do host montada no container) se existir,
// caso contrário usa "/" (fallback).
func readDisk() (DiskInfo, error) {
	path := "/"
	if _, err := os.Stat("/hostroot"); err == nil {
		path = "/hostroot"
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return DiskInfo{}, err
	}

	// Blocos em bytes: Bsize (tamanho do bloco) * blocos
	total := stat.Blocks * uint64(stat.Bsize)
	used := (stat.Blocks - stat.Bfree) * uint64(stat.Bsize)

	return DiskInfo{
		TotalGB: float64(total) / (1024 * 1024 * 1024),
		UsedGB:  float64(used) / (1024 * 1024 * 1024),
	}, nil
}

// containerCount retorna quantos containers Docker estão em execução.
// Retorna "-" se o Docker não estiver acessível.
func containerCount() string {
	out, err := exec.Command("docker", "ps", "-q").Output()
	if err != nil {
		return "-"
	}
	lines := strings.Fields(string(out))
	return strconv.Itoa(len(lines))
}

// uptime retorna o tempo de atividade do sistema no formato "XdHhMm".
func uptime() string {
	f, err := os.Open("/proc/uptime")
	if err != nil {
		return "-"
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return "-"
	}

	fields := strings.Fields(sc.Text())
	if len(fields) == 0 {
		return "-"
	}

	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "-"
	}

	days := int(secs) / 86400
	hours := (int(secs) % 86400) / 3600
	mins := (int(secs) % 3600) / 60

	if days > 0 {
		return fmt.Sprintf("%dd%dh%dm", days, hours, mins)
	}
	return fmt.Sprintf("%dh%dm", hours, mins)
}

// cpuTemp retorna a temperatura da CPU em °C.
func cpuTemp() string {
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return "-"
	}

	s := strings.TrimSpace(string(data))
	var v float64
	// O arquivo pode conter vírgula ou ponto
	s = strings.Replace(s, ",", ".", 1)
	v, err = strconv.ParseFloat(s, 64)
	if err != nil {
		return "-"
	}

	// A fonte do display é ASCII: não usa o glifo "°" (renderiza como "?").
	// Exibe "C" no lugar para indicar Celsius.
	return fmt.Sprintf("%.0fC", v/1000)
}
