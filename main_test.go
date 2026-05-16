package main

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	ec2imds "github.com/aws/aws-sdk-go-v2/feature/ec2/imds"

	"github.com/islishude/aws-eip-binding/eip"
)

func TestAWSLoadOptionsForConfig(t *testing.T) {
	t.Run("IPv4 uses default SDK behavior", func(t *testing.T) {
		opts := awsLoadOptionsForConfig(&eip.Config{Family: eip.IPFamilyIPv4})
		if len(opts) != 0 {
			t.Fatalf("expected no load options, got %d", len(opts))
		}
	})

	t.Run("IPv6 enables IMDS IPv6 and dual-stack service endpoints", func(t *testing.T) {
		var loadOptions config.LoadOptions
		for _, opt := range awsLoadOptionsForConfig(&eip.Config{Family: eip.IPFamilyIPv6}) {
			if err := opt(&loadOptions); err != nil {
				t.Fatalf("apply load option: %v", err)
			}
		}

		if loadOptions.EC2IMDSEndpointMode != ec2imds.EndpointModeStateIPv6 {
			t.Errorf("EC2IMDSEndpointMode = %v, want %v", loadOptions.EC2IMDSEndpointMode, ec2imds.EndpointModeStateIPv6)
		}
		if loadOptions.UseDualStackEndpoint != aws.DualStackEndpointStateEnabled {
			t.Errorf("UseDualStackEndpoint = %v, want %v", loadOptions.UseDualStackEndpoint, aws.DualStackEndpointStateEnabled)
		}
	})
}

func TestIMDSClientForConfig(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_SERVICE_ENDPOINT", "")

	ipv4Client := imdsClientForConfig(&eip.Config{Family: eip.IPFamilyIPv4})
	if ipv4Client.Endpoint != eip.IMDSEndpointIPv4 {
		t.Errorf("IPv4 Endpoint = %q, want %q", ipv4Client.Endpoint, eip.IMDSEndpointIPv4)
	}

	ipv6Client := imdsClientForConfig(&eip.Config{Family: eip.IPFamilyIPv6})
	if ipv6Client.Endpoint != eip.IMDSEndpointIPv6 {
		t.Errorf("IPv6 Endpoint = %q, want %q", ipv6Client.Endpoint, eip.IMDSEndpointIPv6)
	}
}
