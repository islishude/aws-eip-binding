package eip

import (
	"os"
	"testing"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		want    Config
		wantErr bool
	}{
		{
			name:    "no arguments",
			wantErr: true,
		},
		{
			name: "valid IPv4",
			args: []string{"54.162.153.80"},
			want: Config{TargetIP: "54.162.153.80", Family: IPFamilyIPv4},
		},
		{
			name:    "invalid IP",
			args:    []string{"not-an-ip"},
			wantErr: true,
		},
		{
			name: "valid IPv6",
			args: []string{"2001:db8::1234"},
			want: Config{TargetIP: "2001:db8::1234", Family: IPFamilyIPv6},
		},
		{
			name: "IPv4-mapped IPv6 normalized to IPv4",
			args: []string{"::ffff:54.162.153.80"},
			want: Config{TargetIP: "54.162.153.80", Family: IPFamilyIPv4},
		},
		{
			name: "POD_NAME mode resolves IPv4",
			args: []string{"POD_NAME"},
			env: map[string]string{
				"POD_NAME":   "app-config",
				"app_config": "54.162.153.80",
			},
			want: Config{TargetIP: "54.162.153.80", Family: IPFamilyIPv4},
		},
		{
			name: "POD_NAME mode resolves IPv6",
			args: []string{"POD_NAME"},
			env: map[string]string{
				"POD_NAME":      "app-config-v6",
				"app_config_v6": "2001:db8::1234",
			},
			want: Config{TargetIP: "2001:db8::1234", Family: IPFamilyIPv6},
		},
		{
			name:    "POD_NAME mode empty POD_NAME env",
			args:    []string{"POD_NAME"},
			wantErr: true,
		},
		{
			name: "POD_NAME mode empty resolved env",
			args: []string{"POD_NAME"},
			env: map[string]string{
				"POD_NAME": "my-pod",
			},
			wantErr: true,
		},
		{
			name: "POD_NAME mode resolved value is invalid IP",
			args: []string{"POD_NAME"},
			env: map[string]string{
				"POD_NAME": "my-pod",
				"my_pod":   "bad-ip",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseConfig(tt.args, getenvFromMap(tt.env))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertConfig(t, cfg, tt.want)
		})
	}
}

func TestParseConfigFromOS(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() {
		os.Args = origArgs
	})

	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		want    Config
		wantErr bool
	}{
		{
			name:    "requires one CLI argument",
			wantErr: true,
		},
		{
			name: "uses os.Args and os.Getenv",
			args: []string{"POD_NAME"},
			env: map[string]string{
				"POD_NAME": "pod-v6",
				"pod_v6":   "2001:db8::5",
			},
			want: Config{TargetIP: "2001:db8::5", Family: IPFamilyIPv6},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = append([]string{"aws-eip-binding"}, tt.args...)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			cfg, err := ParseConfigFromOS()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertConfig(t, cfg, tt.want)
		})
	}
}

func getenvFromMap(env map[string]string) func(string) string {
	return func(key string) string {
		return env[key]
	}
}

func assertConfig(t *testing.T, got *Config, want Config) {
	t.Helper()
	if got == nil {
		t.Fatal("config is nil")
	}
	if got.TargetIP != want.TargetIP {
		t.Errorf("TargetIP = %q, want %q", got.TargetIP, want.TargetIP)
	}
	if got.Family != want.Family {
		t.Errorf("Family = %q, want %q", got.Family, want.Family)
	}
}
