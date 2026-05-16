package eip

import (
	"testing"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		getenv     func(string) string
		wantIP     string
		wantFamily string
		wantErr    bool
	}{
		{
			name:    "no arguments",
			args:    []string{},
			getenv:  func(string) string { return "" },
			wantErr: true,
		},
		{
			name:       "valid IPv4",
			args:       []string{"54.162.153.80"},
			getenv:     func(string) string { return "" },
			wantIP:     "54.162.153.80",
			wantFamily: IPFamilyIPv4,
		},
		{
			name:    "invalid IP",
			args:    []string{"not-an-ip"},
			getenv:  func(string) string { return "" },
			wantErr: true,
		},
		{
			name:       "valid IPv6",
			args:       []string{"2001:db8::1234"},
			getenv:     func(string) string { return "" },
			wantIP:     "2001:db8::1234",
			wantFamily: IPFamilyIPv6,
		},
		{
			name:       "IPv4-mapped IPv6 normalized to IPv4",
			args:       []string{"::ffff:54.162.153.80"},
			getenv:     func(string) string { return "" },
			wantIP:     "54.162.153.80",
			wantFamily: IPFamilyIPv4,
		},
		{
			name: "POD_NAME mode success",
			args: []string{"POD_NAME"},
			getenv: func(key string) string {
				m := map[string]string{
					"POD_NAME":   "app-config",
					"app_config": "54.162.153.80",
				}
				return m[key]
			},
			wantIP:     "54.162.153.80",
			wantFamily: IPFamilyIPv4,
		},
		{
			name: "POD_NAME mode IPv6 success",
			args: []string{"POD_NAME"},
			getenv: func(key string) string {
				m := map[string]string{
					"POD_NAME":      "app-config-v6",
					"app_config_v6": "2001:db8::1234",
				}
				return m[key]
			},
			wantIP:     "2001:db8::1234",
			wantFamily: IPFamilyIPv6,
		},
		{
			name: "POD_NAME mode empty POD_NAME env",
			args: []string{"POD_NAME"},
			getenv: func(string) string {
				return ""
			},
			wantErr: true,
		},
		{
			name: "POD_NAME mode empty resolved env",
			args: []string{"POD_NAME"},
			getenv: func(key string) string {
				if key == "POD_NAME" {
					return "my-pod"
				}
				return ""
			},
			wantErr: true,
		},
		{
			name: "POD_NAME mode resolved value is invalid IP",
			args: []string{"POD_NAME"},
			getenv: func(key string) string {
				m := map[string]string{
					"POD_NAME": "my-pod",
					"my_pod":   "bad-ip",
				}
				return m[key]
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseConfig(tt.args, tt.getenv)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.TargetIP != tt.wantIP {
				t.Errorf("TargetIP = %q, want %q", cfg.TargetIP, tt.wantIP)
			}
			if cfg.Family != tt.wantFamily {
				t.Errorf("Family = %q, want %q", cfg.Family, tt.wantFamily)
			}
		})
	}
}
