package toolruntime

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

const defaultMaxURLLength = 2000

func validateBuiltinFetchURL(targetURL string, config *builtinConfig) error {
	maxURLLength := defaultMaxURLLength
	if config != nil && config.MaxURLLength > 0 {
		maxURLLength = config.MaxURLLength
	}
	if len(targetURL) > maxURLLength {
		return fmt.Errorf("url too long (max %d characters)", maxURLLength)
	}

	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme")
	}

	host := parsedURL.Hostname()
	if host == "" {
		return fmt.Errorf("missing hostname in URL")
	}
	if isBlockedFetchHost(host) {
		return fmt.Errorf("blocked hostname: %s", host)
	}
	return nil
}

func isBlockedFetchHost(host string) bool {
	normalized := strings.Trim(strings.ToLower(host), "[]")
	if normalized == "" {
		return true
	}
	if normalized == "localhost" || strings.HasSuffix(normalized, ".localhost") || strings.HasSuffix(normalized, ".local") {
		return true
	}

	if addr, err := netip.ParseAddr(normalized); err == nil {
		return isBlockedFetchAddr(addr)
	}

	ip := net.ParseIP(normalized)
	if ip == nil {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	return isBlockedFetchAddr(addr)
}

func isBlockedFetchAddr(addr netip.Addr) bool {
	return addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified()
}
