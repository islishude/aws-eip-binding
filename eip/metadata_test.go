package eip

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIMDSClientGetToken(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantToken string
		wantErr   bool
	}{
		{
			name:      "success",
			status:    http.StatusOK,
			body:      "test-token-abc",
			wantToken: "test-token-abc",
		},
		{
			name:    "server error",
			status:  http.StatusInternalServerError,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newIMDSTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut {
					t.Errorf("method = %s, want %s", r.Method, http.MethodPut)
				}
				if r.URL.Path != "/latest/api/token" {
					t.Errorf("path = %s, want /latest/api/token", r.URL.Path)
				}
				if r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds") != "300" {
					t.Error("missing metadata token TTL header")
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))

			token, err := client.GetToken(context.Background())
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if token != tt.wantToken {
				t.Errorf("token = %q, want %q", token, tt.wantToken)
			}
		})
	}
}

func TestIMDSClientGetMetadata(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		path     string
		status   int
		body     string
		wantPath string
		want     string
		wantErr  bool
	}{
		{
			name:     "public IPv4",
			token:    "tok",
			path:     "meta-data/public-ipv4",
			status:   http.StatusOK,
			body:     "1.2.3.4",
			wantPath: "/latest/meta-data/public-ipv4",
			want:     "1.2.3.4",
		},
		{
			name:     "instance ID",
			token:    "tok",
			path:     "meta-data/instance-id",
			status:   http.StatusOK,
			body:     "i-abc123",
			wantPath: "/latest/meta-data/instance-id",
			want:     "i-abc123",
		},
		{
			name:     "not found",
			token:    "tok",
			path:     "meta-data/missing",
			status:   http.StatusNotFound,
			wantPath: "/latest/meta-data/missing",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newIMDSTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("method = %s, want %s", r.Method, http.MethodGet)
				}
				if r.URL.Path != tt.wantPath {
					t.Errorf("path = %s, want %s", r.URL.Path, tt.wantPath)
				}
				if r.Header.Get("X-aws-ec2-metadata-token") != tt.token {
					t.Errorf("metadata token header = %q, want %q", r.Header.Get("X-aws-ec2-metadata-token"), tt.token)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))

			got, err := client.GetMetadata(context.Background(), tt.token, tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("metadata = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewIMDSClientEndpointSelection(t *testing.T) {
	customHTTPClient := &http.Client{}

	tests := []struct {
		name           string
		options        []IMDSClientOption
		envEndpoint    string
		wantEndpoint   string
		wantHTTPClient *http.Client
	}{
		{
			name:         "default IPv4",
			wantEndpoint: IMDSEndpointIPv4,
		},
		{
			name:         "IPv6 endpoint mode",
			options:      []IMDSClientOption{WithIMDSEndpointMode(IMDSEndpointModeIPv6)},
			wantEndpoint: IMDSEndpointIPv6,
		},
		{
			name:         "environment endpoint override",
			options:      []IMDSClientOption{WithIMDSEndpointMode(IMDSEndpointModeIPv6)},
			envEndpoint:  "http://metadata.local",
			wantEndpoint: "http://metadata.local",
		},
		{
			name:         "explicit endpoint override",
			options:      []IMDSClientOption{WithIMDSEndpointMode(IMDSEndpointModeIPv6), WithIMDSEndpoint("http://explicit.local")},
			envEndpoint:  "http://metadata.local",
			wantEndpoint: "http://explicit.local",
		},
		{
			name:           "custom HTTP client",
			options:        []IMDSClientOption{WithIMDSHTTPClient(customHTTPClient)},
			wantEndpoint:   IMDSEndpointIPv4,
			wantHTTPClient: customHTTPClient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(awsEC2MetadataServiceEndpointEnv, tt.envEndpoint)

			client := NewIMDSClient(tt.options...)
			if client.Endpoint != tt.wantEndpoint {
				t.Errorf("Endpoint = %q, want %q", client.Endpoint, tt.wantEndpoint)
			}
			if tt.wantHTTPClient != nil {
				if client.HTTPClient != tt.wantHTTPClient {
					t.Fatal("HTTPClient was not set to custom client")
				}
				return
			}
			if client.HTTPClient == nil {
				t.Fatal("HTTPClient is nil")
			}
			if client.HTTPClient.Timeout != IMDSTimeout {
				t.Errorf("HTTPClient.Timeout = %s, want %s", client.HTTPClient.Timeout, IMDSTimeout)
			}
		})
	}
}

func newIMDSTestClient(t *testing.T, handler http.Handler) *IMDSClient {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return &IMDSClient{
		HTTPClient: server.Client(),
		Endpoint:   server.URL,
	}
}
