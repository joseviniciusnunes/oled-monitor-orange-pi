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
