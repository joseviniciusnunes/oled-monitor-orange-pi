package main

import (
	"net"
	"os/exec"
	"strings"
)

func getIP() string {
	ifaces, _ := net.Interfaces()

	for _, iface := range ifaces {

		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, _ := iface.Addrs()

		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipnet.IP.To4()
			if ip == nil {
				continue
			}

			// Pula IPs do Docker (172.16.0.0/12)
			if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
				continue
			}

			// Prioriza a rede local 192.168.x.x
			if ip[0] == 192 && ip[1] == 168 {
				return ip.String()
			}
		}
	}

	// Se nenhum IP 192.168 foi encontrado, retorna o primeiro IP privado válido
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if strings.HasPrefix(iface.Name, "docker") {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip != nil {
				return ip.String()
			}
		}
	}

	return "-"
}

func getSSID() string {

	out, err := exec.Command("iwgetid", "-r").Output()
	if err != nil {
		return "-"
	}

	return strings.TrimSpace(string(out))
}
