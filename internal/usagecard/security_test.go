package usagecard

import (
	"net"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		// blocked
		{"127.0.0.1", true},
		{"127.0.0.100", true},
		{"::1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		{"169.254.1.1", true},
		{"0.0.0.0", true},
		{"::", true},
		{"fe80::1", true}, // link-local IPv6

		// allowed
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"172.15.0.1", false},  // not in 172.16/12 range
		{"172.32.0.1", false},
		{"192.169.1.1", false},
		{"104.16.0.1", false},
		{"2606:4700::1", false}, // public IPv6
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP %q", tt.ip)
			}
			got := isBlockedIP(ip)
			if got != tt.want {
				t.Errorf("isBlockedIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestIsBlockedHostname(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"LOCALHOST", true},
		{"my.service.local", true},
		{"my.service.internal", true},
		{"test.lvh.me", true},
		{"api.github.com", false},
		{"api.openai.com", false},
		{"example.com", false},
		{"sub.domain.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := isBlockedHostname(tt.host)
			if got != tt.want {
				t.Errorf("isBlockedHostname(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestValidateEndpoint(t *testing.T) {
	tests := []struct {
		endpoint string
		wantErr  bool
	}{
		{"https://api.github.com/rate_limit", false},
		{"http://example.com/api", false},
		{"", true},                          // empty
		{"ftp://example.com", true},         // non-http
		{"api.github.com/rate_limit", true}, // no protocol
		{"https://localhost/api", true},     // localhost
		{"https://my.service.local/api", true}, // .local
	}

	for _, tt := range tests {
		t.Run(tt.endpoint, func(t *testing.T) {
			err := validateEndpoint(tt.endpoint)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEndpoint(%q) error = %v, wantErr %v", tt.endpoint, err, tt.wantErr)
			}
		})
	}
}
