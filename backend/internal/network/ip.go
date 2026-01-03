package network

import (
	"net"
	"os"
)

var localhostIP = net.IPv4(127, 0, 0, 1)

func HostIP() net.IP {
	hostname, err := os.Hostname()
	if err != nil {
		return localhostIP
	}
	addrs, err := net.LookupIP(hostname)
	if err != nil {
		return localhostIP
	}

	for _, ip := range addrs {
		if ip.IsLoopback() {
			continue
		}
		if ip4 := ip.To4(); ip4 != nil {
			return ip4
		}
	}

	return localhostIP
}
