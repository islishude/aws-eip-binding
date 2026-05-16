package eip

import (
	"context"
	"log"
	"net/netip"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

const enableAWSE2ETestsEnv = "ENABLE_AWS_E2E_TESTS"

func TestAWSE2E_ReadOnlyCurrentInstance(t *testing.T) {
	ctx := context.Background()
	ec2c := awsE2EEC2Client(t)
	imds := NewIMDSClient()

	token, err := imds.GetToken()
	if err != nil {
		t.Fatalf("get real IMDSv2 token: %v", err)
	}
	instanceID, err := imds.GetMetadata(token, "meta-data/instance-id")
	if err != nil {
		t.Fatalf("get real instance-id from IMDS: %v", err)
	}

	binder := NewBinder(ec2c, imds, awsE2ELogger())
	primaryENI, err := binder.findPrimaryNetworkInterface(ctx, instanceID)
	if err != nil {
		t.Fatalf("find primary ENI with DescribeNetworkInterfaces attachment filters: %v", err)
	}
	if primaryENI.NetworkInterfaceId == nil {
		t.Fatal("primary ENI has no NetworkInterfaceId")
	}

	publicIP, err := imds.GetMetadata(token, "meta-data/public-ipv4")
	if err != nil {
		t.Logf("public-ipv4 metadata unavailable; skipped public IPv4 ENI filter check: %v", err)
		return
	}

	publicIPFilter := "addresses.association.public-ip"
	eniOut, err := ec2c.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
		Filters: []types.Filter{
			{
				Name:   &publicIPFilter,
				Values: []string{publicIP},
			},
		},
	})
	if err != nil {
		t.Fatalf("describe network interfaces with %s=%s: %v", publicIPFilter, publicIP, err)
	}
	if len(eniOut.NetworkInterfaces) == 0 {
		t.Fatalf("DescribeNetworkInterfaces filter %s=%s returned no ENIs", publicIPFilter, publicIP)
	}
}

func TestAWSE2E_BindTargetIPv4(t *testing.T) {
	targetIP := strings.TrimSpace(os.Getenv("AWS_E2E_TARGET_IPV4"))
	if targetIP == "" {
		t.Skip("AWS_E2E_TARGET_IPV4 not set - skipping mutating IPv4 bind")
	}
	targetAddr, err := netip.ParseAddr(targetIP)
	if err != nil {
		t.Fatalf("AWS_E2E_TARGET_IPV4 is not a valid IP: %v", err)
	}
	targetAddr = targetAddr.Unmap()
	if !targetAddr.Is4() {
		t.Fatalf("AWS_E2E_TARGET_IPV4 = %q, want IPv4 address", targetIP)
	}

	ctx := context.Background()
	ec2c := awsE2EEC2Client(t)
	binder := NewBinder(ec2c, NewIMDSClient(), awsE2ELogger())

	result, err := binder.Bind(ctx, targetIP)
	if err != nil {
		t.Fatalf("bind real IPv4 target %s: %v", targetIP, err)
	}
	if result.Family != IPFamilyIPv4 {
		t.Fatalf("Family = %q, want %q", result.Family, IPFamilyIPv4)
	}
	if result.TargetIP != targetAddr.String() {
		t.Fatalf("TargetIP = %q, want %q", result.TargetIP, targetAddr.String())
	}
}

func TestAWSE2E_BindTargetIPv6(t *testing.T) {
	targetIP := strings.TrimSpace(os.Getenv("AWS_E2E_TARGET_IPV6"))
	if targetIP == "" {
		t.Skip("AWS_E2E_TARGET_IPV6 not set - skipping mutating IPv6 bind")
	}
	targetAddr, err := netip.ParseAddr(targetIP)
	if err != nil {
		t.Fatalf("AWS_E2E_TARGET_IPV6 is not a valid IP: %v", err)
	}
	if !targetAddr.Is6() || targetAddr.Is4In6() {
		t.Fatalf("AWS_E2E_TARGET_IPV6 = %q, want IPv6 address", targetIP)
	}
	targetAddr = targetAddr.Unmap()

	ctx := context.Background()
	ec2c := awsE2EEC2Client(t, awsconfig.WithUseDualStackEndpoint(aws.DualStackEndpointStateEnabled))
	imds := NewIMDSClient(WithIMDSEndpointMode(IMDSEndpointModeIPv6))

	token, err := imds.GetToken()
	if err != nil {
		t.Fatalf("get real IMDSv2 token from IPv6 endpoint: %v", err)
	}
	instanceID, err := imds.GetMetadata(token, "meta-data/instance-id")
	if err != nil {
		t.Fatalf("get real instance-id from IPv6 IMDS endpoint: %v", err)
	}

	binder := NewBinder(ec2c, imds, awsE2ELogger())
	primaryENI, err := binder.findPrimaryNetworkInterface(ctx, instanceID)
	if err != nil {
		t.Fatalf("find primary ENI before IPv6 bind: %v", err)
	}
	if primaryENI.SubnetId == nil {
		t.Fatalf("primary ENI %v has no subnet ID", primaryENI.NetworkInterfaceId)
	}

	if !awsE2ESubnetContainsIPv6Target(t, ctx, ec2c, *primaryENI.SubnetId, targetAddr) {
		t.Fatalf("AWS_E2E_TARGET_IPV6 %s is outside subnet %s IPv6 CIDR blocks", targetAddr, *primaryENI.SubnetId)
	}

	result, err := binder.Bind(ctx, targetAddr.String())
	if err != nil {
		t.Fatalf("bind real IPv6 target %s: %v", targetAddr, err)
	}
	if result.Family != IPFamilyIPv6 {
		t.Fatalf("Family = %q, want %q", result.Family, IPFamilyIPv6)
	}
	if result.NetworkInterfaceID == "" {
		t.Fatal("NetworkInterfaceID is empty after IPv6 bind")
	}
}

func awsE2EEC2Client(t *testing.T, optFns ...func(*awsconfig.LoadOptions) error) *ec2.Client {
	t.Helper()
	if os.Getenv(enableAWSE2ETestsEnv) != "true" {
		t.Skip(enableAWSE2ETestsEnv + " is not true - skipping AWS E2E test")
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), optFns...)
	if err != nil {
		t.Fatalf("load real AWS config: %v", err)
	}
	if cfg.Region == "" {
		t.Fatal("AWS region is empty; set AWS_REGION or run from an EC2 environment that provides a region")
	}
	return ec2.NewFromConfig(cfg)
}

func awsE2ELogger() *log.Logger {
	return log.New(os.Stderr, "[aws-e2e] ", 0)
}

func awsE2ESubnetContainsIPv6Target(t *testing.T, ctx context.Context, ec2c *ec2.Client, subnetID string, targetAddr netip.Addr) bool {
	t.Helper()

	subnetsOut, err := ec2c.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		SubnetIds: []string{subnetID},
	})
	if err != nil {
		t.Fatalf("describe subnet %s before IPv6 bind: %v", subnetID, err)
	}
	if len(subnetsOut.Subnets) == 0 {
		t.Fatalf("subnet %s not found before IPv6 bind", subnetID)
	}

	hasIPv6CIDR := false
	for _, assoc := range subnetsOut.Subnets[0].Ipv6CidrBlockAssociationSet {
		if assoc.Ipv6CidrBlock == nil {
			continue
		}
		hasIPv6CIDR = true
		prefix, err := netip.ParsePrefix(*assoc.Ipv6CidrBlock)
		if err != nil {
			t.Fatalf("parse subnet IPv6 CIDR %s: %v", *assoc.Ipv6CidrBlock, err)
		}
		if prefix.Contains(targetAddr) {
			return true
		}
	}
	if !hasIPv6CIDR {
		t.Skipf("subnet %s has no IPv6 CIDR blocks - skipping IPv6 AWS E2E bind", subnetID)
	}
	return false
}
