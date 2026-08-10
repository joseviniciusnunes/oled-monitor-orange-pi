package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

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

	if total == 0 {
		return 0, fmt.Errorf("MemTotal não encontrado")
	}

	used := float64(total-available) * 100 / float64(total)
	return used, nil
}
