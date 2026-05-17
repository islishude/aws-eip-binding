package eip

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	IMDSEndpointIPv4 = "http://169.254.169.254"
	IMDSEndpointIPv6 = "http://[fd00:ec2::254]"
	IMDSTimeout      = 2 * time.Second

	awsEC2MetadataServiceEndpointEnv = "AWS_EC2_METADATA_SERVICE_ENDPOINT"
)

type IMDSEndpointMode string

const (
	IMDSEndpointModeIPv4 IMDSEndpointMode = "IPv4"
	IMDSEndpointModeIPv6 IMDSEndpointMode = "IPv6"
)

type imdsClientOptions struct {
	httpClient   *http.Client
	endpoint     string
	endpointSet  bool
	endpointMode IMDSEndpointMode
}

// IMDSClientOption configures an IMDSClient created by NewIMDSClient.
type IMDSClientOption func(*imdsClientOptions)

// WithIMDSHTTPClient sets the HTTP client used by IMDSClient.
func WithIMDSHTTPClient(client *http.Client) IMDSClientOption {
	return func(o *imdsClientOptions) {
		o.httpClient = client
	}
}

// WithIMDSEndpoint overrides the IMDS endpoint.
func WithIMDSEndpoint(endpoint string) IMDSClientOption {
	return func(o *imdsClientOptions) {
		o.endpoint = endpoint
		o.endpointSet = endpoint != ""
	}
}

// WithIMDSEndpointMode selects the default IMDS endpoint for IPv4 or IPv6.
func WithIMDSEndpointMode(mode IMDSEndpointMode) IMDSClientOption {
	return func(o *imdsClientOptions) {
		o.endpointMode = mode
	}
}

// MetadataClient abstracts EC2 instance metadata retrieval.
type MetadataClient interface {
	// GetToken fetches an IMDSv2 session token.
	GetToken(ctx context.Context) (string, error)
	// GetMetadata retrieves metadata at the given path using the provided token.
	GetMetadata(ctx context.Context, token, path string) (string, error)
}

// IMDSClient implements MetadataClient using the EC2 Instance Metadata Service v2.
type IMDSClient struct {
	// HTTPClient is the HTTP client used to call the metadata endpoint.
	HTTPClient *http.Client
	// Endpoint is the base URL for the metadata service.
	Endpoint string
}

// NewIMDSClient creates a new IMDSClient with default settings.
func NewIMDSClient(optFns ...IMDSClientOption) *IMDSClient {
	opts := imdsClientOptions{
		httpClient:   &http.Client{Timeout: IMDSTimeout},
		endpointMode: IMDSEndpointModeIPv4,
	}
	for _, fn := range optFns {
		fn(&opts)
	}

	endpoint := defaultIMDSEndpoint(opts.endpointMode)
	if envEndpoint := os.Getenv(awsEC2MetadataServiceEndpointEnv); envEndpoint != "" {
		endpoint = envEndpoint
	}
	if opts.endpointSet {
		endpoint = opts.endpoint
	}
	if opts.httpClient == nil {
		opts.httpClient = &http.Client{Timeout: IMDSTimeout}
	}

	return &IMDSClient{
		HTTPClient: opts.httpClient,
		Endpoint:   endpoint,
	}
}

func defaultIMDSEndpoint(mode IMDSEndpointMode) string {
	if mode == IMDSEndpointModeIPv6 {
		return IMDSEndpointIPv6
	}
	return IMDSEndpointIPv4
}

// GetToken fetches an IMDSv2 token from the EC2 metadata service.
func (c *IMDSClient) GetToken(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.Endpoint+"/latest/api/token", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Add("X-aws-ec2-metadata-token-ttl-seconds", "300")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get metadata token: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata token request returned status %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read metadata token: %w", err)
	}
	return string(b), nil
}

// GetMetadata retrieves metadata from EC2 instance by providing the token and metadata path.
func (c *IMDSClient) GetMetadata(ctx context.Context, token, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Endpoint+"/latest/"+path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create metadata request: %w", err)
	}
	req.Header.Add("X-aws-ec2-metadata-token", token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get metadata %s: %w", path, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata %s request returned status %d", path, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read metadata %s: %w", path, err)
	}
	return string(b), nil
}
