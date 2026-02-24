package eip

import (
	"fmt"
	"io"
	"net/http"
)

// MetadataClient abstracts EC2 instance metadata retrieval.
type MetadataClient interface {
	// GetToken fetches an IMDSv2 session token.
	GetToken() (string, error)
	// GetMetadata retrieves metadata at the given path using the provided token.
	GetMetadata(token, path string) (string, error)
}

// IMDSClient implements MetadataClient using the EC2 Instance Metadata Service v2.
type IMDSClient struct {
	// HTTPClient is the HTTP client used to call the metadata endpoint.
	HTTPClient *http.Client
	// Endpoint is the base URL for the metadata service (default: http://169.254.169.254).
	Endpoint string
}

// NewIMDSClient creates a new IMDSClient with default settings.
func NewIMDSClient() *IMDSClient {
	return &IMDSClient{
		HTTPClient: http.DefaultClient,
		Endpoint:   "http://169.254.169.254",
	}
}

// GetToken fetches an IMDSv2 token from the EC2 metadata service.
func (c *IMDSClient) GetToken() (string, error) {
	req, err := http.NewRequest(http.MethodPut, c.Endpoint+"/latest/api/token", nil)
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
func (c *IMDSClient) GetMetadata(token, path string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.Endpoint+"/latest/"+path, nil)
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
