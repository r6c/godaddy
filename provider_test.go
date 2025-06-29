package godaddy

import (
	"net/netip"
	"testing"
	"time"

	"github.com/libdns/libdns"
)

func TestProviderConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		provider    Provider
		expectedURL string
	}{
		{
			name: "Production Environment (default)",
			provider: Provider{
				APIToken: "test:secret",
			},
			expectedURL: "https://api.godaddy.com",
		},
		{
			name: "OTE Environment",
			provider: Provider{
				APIToken: "test:secret",
				UseOTE:   true,
			},
			expectedURL: "https://api.ote-godaddy.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := tt.provider.getApiHost()
			if url != tt.expectedURL {
				t.Errorf("getApiHost() = %s; expected %s", url, tt.expectedURL)
			}
		})
	}
}

func TestHTTPClientConfiguration(t *testing.T) {
	tests := []struct {
		name            string
		provider        Provider
		expectedTimeout time.Duration
	}{
		{
			name: "Default timeout",
			provider: Provider{
				APIToken: "test:secret",
			},
			expectedTimeout: 30 * time.Second,
		},
		{
			name: "Custom timeout",
			provider: Provider{
				APIToken:    "test:secret",
				HTTPTimeout: 60 * time.Second,
			},
			expectedTimeout: 60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.provider.getHTTPClient()
			if client.Timeout != tt.expectedTimeout {
				t.Errorf("HTTP client timeout = %v; expected %v", client.Timeout, tt.expectedTimeout)
			}
		})
	}
}

func TestConvertToLibdnsRecord(t *testing.T) {
	tests := []struct {
		name     string
		input    godaddyRecord
		expected libdns.Record
	}{
		{
			name: "A Record",
			input: godaddyRecord{
				Type: "A",
				Name: "www",
				Data: "192.168.1.1",
				TTL:  3600,
			},
			expected: libdns.Address{
				Name: "www",
				TTL:  time.Hour,
				IP:   netip.MustParseAddr("192.168.1.1"),
			},
		},
		{
			name: "TXT Record",
			input: godaddyRecord{
				Type: "TXT",
				Name: "_acme-challenge",
				Data: "test-challenge-token",
				TTL:  300,
			},
			expected: libdns.TXT{
				Name: "_acme-challenge",
				TTL:  5 * time.Minute,
				Text: "test-challenge-token",
			},
		},
		{
			name: "CNAME Record",
			input: godaddyRecord{
				Type: "CNAME",
				Name: "blog",
				Data: "example.com",
				TTL:  3600,
			},
			expected: libdns.CNAME{
				Name:   "blog",
				TTL:    time.Hour,
				Target: "example.com",
			},
		},
		{
			name: "MX Record",
			input: godaddyRecord{
				Type: "MX",
				Name: "@",
				Data: "10 mail.example.com",
				TTL:  3600,
			},
			expected: libdns.MX{
				Name:       "@",
				TTL:        time.Hour,
				Preference: 10,
				Target:     "mail.example.com",
			},
		},
		{
			name: "Invalid MX Record - fallback to RR",
			input: godaddyRecord{
				Type: "MX",
				Name: "@",
				Data: "invalid-mx-format",
				TTL:  3600,
			},
			expected: libdns.RR{
				Name: "@",
				TTL:  time.Hour,
				Type: "MX",
				Data: "invalid-mx-format",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToLibdnsRecord(tt.input)

			// Compare RR representations for consistency
			expectedRR := tt.expected.RR()
			resultRR := result.RR()

			if expectedRR.Name != resultRR.Name {
				t.Errorf("Name mismatch: expected %s, got %s", expectedRR.Name, resultRR.Name)
			}
			if expectedRR.Type != resultRR.Type {
				t.Errorf("Type mismatch: expected %s, got %s", expectedRR.Type, resultRR.Type)
			}
			if expectedRR.TTL != resultRR.TTL {
				t.Errorf("TTL mismatch: expected %v, got %v", expectedRR.TTL, resultRR.TTL)
			}
			if expectedRR.Data != resultRR.Data {
				t.Errorf("Data mismatch: expected %s, got %s", expectedRR.Data, resultRR.Data)
			}
		})
	}
}

func TestConvertFromLibdnsRecord(t *testing.T) {
	tests := []struct {
		name     string
		input    libdns.Record
		zone     string
		expected godaddyRecord
	}{
		{
			name: "Address Record",
			input: libdns.Address{
				Name: "www.example.com.",
				TTL:  time.Hour,
				IP:   netip.MustParseAddr("192.168.1.1"),
			},
			zone: "example.com.",
			expected: godaddyRecord{
				Type: "A",
				Name: "www",
				Data: "192.168.1.1",
				TTL:  3600,
			},
		},
		{
			name: "TXT Record with minimum TTL enforcement",
			input: libdns.TXT{
				Name: "_acme-challenge.example.com.",
				TTL:  5 * time.Minute, // Less than 600 seconds
				Text: "test-challenge-token",
			},
			zone: "example.com.",
			expected: godaddyRecord{
				Type: "TXT",
				Name: "_acme-challenge",
				Data: "test-challenge-token",
				TTL:  600, // Minimum TTL enforced
			},
		},
		{
			name: "TXT Record with sufficient TTL",
			input: libdns.TXT{
				Name: "_acme-challenge.example.com.",
				TTL:  15 * time.Minute, // More than 600 seconds
				Text: "test-challenge-token",
			},
			zone: "example.com.",
			expected: godaddyRecord{
				Type: "TXT",
				Name: "_acme-challenge",
				Data: "test-challenge-token",
				TTL:  900, // Original TTL preserved
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertFromLibdnsRecord(tt.input, tt.zone)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.Type != tt.expected.Type {
				t.Errorf("Type mismatch: expected %s, got %s", tt.expected.Type, result.Type)
			}
			if result.Name != tt.expected.Name {
				t.Errorf("Name mismatch: expected %s, got %s", tt.expected.Name, result.Name)
			}
			if result.Data != tt.expected.Data {
				t.Errorf("Data mismatch: expected %s, got %s", tt.expected.Data, result.Data)
			}
			if result.TTL != tt.expected.TTL {
				t.Errorf("TTL mismatch: expected %d, got %d", tt.expected.TTL, result.TTL)
			}
		})
	}
}

func TestGetRecordName(t *testing.T) {
	tests := []struct {
		zone     string
		name     string
		expected string
	}{
		{"example.com.", "@", "@"},
		{"example.com.", "www.example.com.", "www"},
		{"example.com.", "sub.example.com.", "sub"},
		{"example.com.", "test", "test"},
		{"example.com.", "_acme-challenge.sub.example.com.", "_acme-challenge.sub"},
	}

	for _, tt := range tests {
		result := getRecordName(tt.zone, tt.name)
		if result != tt.expected {
			t.Errorf("getRecordName(%s, %s) = %s; expected %s", tt.zone, tt.name, result, tt.expected)
		}
	}
}
