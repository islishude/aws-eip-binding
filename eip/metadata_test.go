package eip

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIMDSClient_GetToken(t *testing.T) {
	tests := []struct {
		name      string
		handler   http.HandlerFunc
		wantToken string
		wantErr   bool
	}{
		{
			name: "success",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut {
					t.Errorf("expected PUT, got %s", r.Method)
				}
				if r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds") != "300" {
					t.Error("missing TTL header")
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("test-token-abc")) //nolint:errcheck
			},
			wantToken: "test-token-abc",
		},
		{
			name: "server error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			c := &IMDSClient{
				HTTPClient: srv.Client(),
				Endpoint:   srv.URL,
			}

			token, err := c.GetToken()
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

func TestIMDSClient_GetMetadata(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		path    string
		handler http.HandlerFunc
		want    string
		wantErr bool
	}{
		{
			name:  "public-ipv4",
			token: "tok",
			path:  "meta-data/public-ipv4",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}
				if r.Header.Get("X-aws-ec2-metadata-token") != "tok" {
					t.Error("missing token header")
				}
				if r.URL.Path != "/latest/meta-data/public-ipv4" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("1.2.3.4")) //nolint:errcheck
			},
			want: "1.2.3.4",
		},
		{
			name:  "instance-id",
			token: "tok",
			path:  "meta-data/instance-id",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("i-abc123")) //nolint:errcheck
			},
			want: "i-abc123",
		},
		{
			name:  "not found",
			token: "tok",
			path:  "meta-data/missing",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			c := &IMDSClient{
				HTTPClient: srv.Client(),
				Endpoint:   srv.URL,
			}

			got, err := c.GetMetadata(tt.token, tt.path)
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
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
