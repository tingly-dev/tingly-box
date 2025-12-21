package util

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		want string
	}{
		{
			name: "empty string",
			host: "",
			want: "localhost",
		},
		{
			name: "localhost",
			host: "localhost",
			want: "localhost",
		},
		{
			name: "IPv4 address",
			host: "192.168.1.100",
			want: "192.168.1.100",
		},
		{
			name: "IPv6 address",
			host: "2001:db8::1",
			want: "2001:db8::1",
		},
		{
			name: "zero address resolves to local IP",
			host: "0.0.0.0",
		},
		{
			name: "hostname returns original if resolution fails",
			host: "nonexistent.invalid.domain.test",
			want: "nonexistent.invalid.domain.test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveHost(tt.host)

			if tt.want != "" {
				assert.Equal(t, tt.want, got)
			} else {
				// For zero address, just verify it's not 0.0.0.0 anymore
				if tt.host == "0.0.0.0" {
					assert.NotEqual(t, "0.0.0.0", got)
					ip := net.ParseIP(got)
					assert.NotNil(t, ip)
				}
			}
		})
	}
}

func TestGetLocalIP(t *testing.T) {
	ip := getLocalIP()
	assert.NotEmpty(t, ip)

	parsedIP := net.ParseIP(ip)
	assert.NotNil(t, parsedIP)
}
