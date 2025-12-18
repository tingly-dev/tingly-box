package util

import (
	"net"
)

// getLocalIP returns the first non-loopback local IP address
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1" // fallback to localhost
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}

	return "127.0.0.1" // fallback to localhost
}

// ResolveHost resolves the host to an IP address or returns it as-is if it's already an IP or localhost
func ResolveHost(host string) string {
	if host == "" || host == "localhost" {
		return "localhost"
	}

	// Handle 0.0.0.0 specially by resolving to actual local IP
	if host == "0.0.0.0" {
		return getLocalIP()
	}

	// Try to parse as IP address first
	if ip := net.ParseIP(host); ip != nil {
		return host
	}

	// Try to resolve hostname
	ips, err := net.LookupHost(host)
	if err != nil {
		// If resolution fails, return the original host
		return host
	}

	if len(ips) > 0 {
		// Return the first resolved IP
		return ips[0]
	}

	return host
}
