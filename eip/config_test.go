package eip

import (
	"testing"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		getenv  func(string) string
		wantIP  string
		wantErr bool
	}{
		{
			name:    "no arguments",
			args:    []string{},
			getenv:  func(string) string { return "" },
			wantErr: true,
		},
		{
			name:   "valid IPv4",
			args:   []string{"54.162.153.80"},
			getenv: func(string) string { return "" },
			wantIP: "54.162.153.80",
		},
		{
			name:    "invalid IP",
			args:    []string{"not-an-ip"},
			getenv:  func(string) string { return "" },
			wantErr: true,
		},
		{
			name:    "IPv6 rejected",
			args:    []string{"::1"},
			getenv:  func(string) string { return "" },
			wantErr: true,
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
			wantIP: "54.162.153.80",
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
		})
	}
}
