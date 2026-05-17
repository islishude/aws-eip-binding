package main

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	ec2imds "github.com/aws/aws-sdk-go-v2/feature/ec2/imds"

	"github.com/islishude/aws-eip-binding/eip"
)

func TestAWSLoadOptionsForConfig(t *testing.T) {
	tests := []struct {
		name          string
		cfg           eip.Config
		wantOptionLen int
		wantIMDSMode  ec2imds.EndpointModeState
		wantDualStack aws.DualStackEndpointState
	}{
		{
			name: "IPv4 uses default SDK behavior",
			cfg:  eip.Config{Family: eip.IPFamilyIPv4},
		},
		{
			name:          "IPv6 enables IMDS IPv6 and dual-stack service endpoints",
			cfg:           eip.Config{Family: eip.IPFamilyIPv6},
			wantOptionLen: 2,
			wantIMDSMode:  ec2imds.EndpointModeStateIPv6,
			wantDualStack: aws.DualStackEndpointStateEnabled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := awsLoadOptionsForConfig(&tt.cfg)
			if len(opts) != tt.wantOptionLen {
				t.Fatalf("load option count = %d, want %d", len(opts), tt.wantOptionLen)
			}
			if tt.wantOptionLen == 0 {
				return
			}

			loadOptions := applyLoadOptions(t, opts)
			if loadOptions.EC2IMDSEndpointMode != tt.wantIMDSMode {
				t.Errorf("EC2IMDSEndpointMode = %v, want %v", loadOptions.EC2IMDSEndpointMode, tt.wantIMDSMode)
			}
			if loadOptions.UseDualStackEndpoint != tt.wantDualStack {
				t.Errorf("UseDualStackEndpoint = %v, want %v", loadOptions.UseDualStackEndpoint, tt.wantDualStack)
			}
		})
	}
}

func TestIMDSClientOptionsForConfig(t *testing.T) {
	tests := []struct {
		name             string
		cfg              eip.Config
		wantOptionLen    int
		wantEndpointMode ec2imds.EndpointModeState
	}{
		{
			name:          "IPv4 disables IMDSv1 fallback",
			cfg:           eip.Config{Family: eip.IPFamilyIPv4},
			wantOptionLen: 1,
		},
		{
			name:             "IPv6 selects IPv6 endpoint mode and disables IMDSv1 fallback",
			cfg:              eip.Config{Family: eip.IPFamilyIPv6},
			wantOptionLen:    2,
			wantEndpointMode: ec2imds.EndpointModeStateIPv6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := imdsClientOptionsForConfig(&tt.cfg)
			if len(opts) != tt.wantOptionLen {
				t.Fatalf("IMDS option count = %d, want %d", len(opts), tt.wantOptionLen)
			}

			imdsOptions := applyIMDSClientOptions(opts)
			if imdsOptions.EnableFallback != aws.BoolTernary(false) {
				t.Errorf("EnableFallback = %v, want %v", imdsOptions.EnableFallback, aws.BoolTernary(false))
			}
			if imdsOptions.EndpointMode != tt.wantEndpointMode {
				t.Errorf("EndpointMode = %v, want %v", imdsOptions.EndpointMode, tt.wantEndpointMode)
			}
		})
	}
}

func applyLoadOptions(t *testing.T, opts []func(*config.LoadOptions) error) config.LoadOptions {
	t.Helper()

	var loadOptions config.LoadOptions
	for _, opt := range opts {
		if err := opt(&loadOptions); err != nil {
			t.Fatalf("apply load option: %v", err)
		}
	}
	return loadOptions
}

func applyIMDSClientOptions(opts []func(*ec2imds.Options)) ec2imds.Options {
	var imdsOptions ec2imds.Options
	for _, opt := range opts {
		opt(&imdsOptions)
	}
	return imdsOptions
}
