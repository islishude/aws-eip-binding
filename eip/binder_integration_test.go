package eip

import (
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// localstackEC2Client builds an EC2 client pointed at LocalStack.
// It reads AWS_ENDPOINT_URL (with fallback to AWS_ENDPOINT), and skips the
// test if neither variable is set.
func localstackEC2Client(t *testing.T) *ec2.Client {
	t.Helper()
	if os.Getenv("ENABLE_INTEGRATION_TESTS") != "true" {
		t.Skip("ENABLE_INTEGRATION_TESTS is not true – skipping integration test")
	}

	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	if endpoint == "" {
		endpoint = os.Getenv("AWS_ENDPOINT")
	}
	if endpoint == "" {
		t.Skip("AWS_ENDPOINT_URL not set – skipping integration test")
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
		awsconfig.WithBaseEndpoint(endpoint),
	)
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}
	return ec2.NewFromConfig(cfg)
}

// integrationIMDS returns an IMDSClient backed by a local httptest.Server that
// serves the given publicIP and instanceID at the IMDSv2 paths used by Binder.
func integrationIMDS(t *testing.T, publicIP, instanceID string) *IMDSClient {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/latest/api/token":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("integration-test-token")) //nolint:errcheck
		case r.Method == http.MethodGet && r.URL.Path == "/latest/meta-data/public-ipv4":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(publicIP)) //nolint:errcheck
		case r.Method == http.MethodGet && r.URL.Path == "/latest/meta-data/instance-id":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(instanceID)) //nolint:errcheck
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	return &IMDSClient{
		HTTPClient: srv.Client(),
		Endpoint:   srv.URL,
	}
}

// allocateEIP allocates a VPC-domain Elastic IP in LocalStack and registers a
// cleanup to release it at the end of the test.
func allocateEIP(t *testing.T, ec2c *ec2.Client) *ec2.AllocateAddressOutput {
	t.Helper()

	out, err := ec2c.AllocateAddress(context.Background(), &ec2.AllocateAddressInput{
		Domain: types.DomainTypeVpc,
	})
	if err != nil {
		t.Fatalf("allocate EIP: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ec2c.ReleaseAddress(context.Background(), &ec2.ReleaseAddressInput{
			AllocationId: out.AllocationId,
		})
	})
	return out
}

// createENI creates a network interface in the first available LocalStack subnet
// and registers a cleanup to delete it at the end of the test. The test is
// skipped if no subnets exist.
func createENI(t *testing.T, ec2c *ec2.Client) *types.NetworkInterface {
	t.Helper()

	subnets, err := ec2c.DescribeSubnets(context.Background(), &ec2.DescribeSubnetsInput{})
	if err != nil || len(subnets.Subnets) == 0 {
		t.Skipf("no subnets available in LocalStack (err=%v) – skipping", err)
	}

	out, err := ec2c.CreateNetworkInterface(context.Background(), &ec2.CreateNetworkInterfaceInput{
		SubnetId: subnets.Subnets[0].SubnetId,
	})
	if err != nil {
		t.Fatalf("create network interface: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ec2c.DeleteNetworkInterface(context.Background(), &ec2.DeleteNetworkInterfaceInput{
			NetworkInterfaceId: out.NetworkInterface.NetworkInterfaceId,
		})
	})
	return out.NetworkInterface
}

// integrationLogger returns a logger that writes to stderr with an [integration]
// prefix, making it easy to distinguish from other test output.
func integrationLogger() *log.Logger {
	return log.New(os.Stderr, "[integration] ", 0)
}

// TestIntegration_AlreadyAssociated verifies that Bind returns AlreadyAssociated
// when the target EIP is already the instance's public IP.
func TestIntegration_AlreadyAssociated(t *testing.T) {
	ctx := context.Background()
	ec2c := localstackEC2Client(t)

	eipOut := allocateEIP(t, ec2c)
	eni := createENI(t, ec2c)

	// Pre-associate: the EIP is already on our ENI.
	assocOut, err := ec2c.AssociateAddress(ctx, &ec2.AssociateAddressInput{
		AllocationId:       eipOut.AllocationId,
		NetworkInterfaceId: eni.NetworkInterfaceId,
	})
	if err != nil {
		t.Fatalf("pre-associate EIP: %v", err)
	}
	t.Cleanup(func() {
		if assocOut.AssociationId != nil {
			_, _ = ec2c.DisassociateAddress(context.Background(), &ec2.DisassociateAddressInput{
				AssociationId: assocOut.AssociationId,
			})
		}
	})

	// IMDS reports the same EIP as the instance's public IP.
	imds := integrationIMDS(t, *eipOut.PublicIp, "i-integration-test")
	b := NewBinder(ec2c, imds, integrationLogger())

	result, err := b.Bind(ctx, *eipOut.PublicIp)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if !result.AlreadyAssociated {
		t.Error("expected AlreadyAssociated=true, got false")
	}
	if result.InstanceID != "i-integration-test" {
		t.Errorf("InstanceID=%q, want i-integration-test", result.InstanceID)
	}
}

// TestIntegration_NewAssociation verifies that Bind associates a free EIP with
// the current instance's ENI (found via the instance's current public IP).
func TestIntegration_NewAssociation(t *testing.T) {
	ctx := context.Background()
	ec2c := localstackEC2Client(t)

	currentEIP := allocateEIP(t, ec2c) // the EIP currently on the instance
	targetEIP := allocateEIP(t, ec2c)  // the EIP we want to bind
	eni := createENI(t, ec2c)

	// Associate currentEIP with the ENI so Binder can find the ENI by
	// filtering DescribeNetworkInterfaces on the instance's public IP.
	_, err := ec2c.AssociateAddress(ctx, &ec2.AssociateAddressInput{
		AllocationId:       currentEIP.AllocationId,
		NetworkInterfaceId: eni.NetworkInterfaceId,
	})
	if err != nil {
		t.Fatalf("pre-associate currentEIP: %v", err)
	}

	imds := integrationIMDS(t, *currentEIP.PublicIp, "i-integration-test")
	b := NewBinder(ec2c, imds, integrationLogger())

	result, err := b.Bind(ctx, *targetEIP.PublicIp)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if result.AlreadyAssociated {
		t.Error("expected AlreadyAssociated=false, got true")
	}
	if result.AssociationID == "" {
		t.Error("expected non-empty AssociationID")
	}
	if result.InstanceID != "i-integration-test" {
		t.Errorf("InstanceID=%q, want i-integration-test", result.InstanceID)
	}

	// Confirm the targetEIP is now associated in LocalStack.
	descOut, err := ec2c.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{
		AllocationIds: []string{*targetEIP.AllocationId},
	})
	if err != nil {
		t.Fatalf("DescribeAddresses after Bind: %v", err)
	}
	if len(descOut.Addresses) == 0 || descOut.Addresses[0].AssociationId == nil {
		t.Error("targetEIP has no association after Bind")
	}
}

// TestIntegration_DisassociatesFirst verifies that Bind removes an existing
// association before moving the EIP to the current instance's ENI.
func TestIntegration_DisassociatesFirst(t *testing.T) {
	ctx := context.Background()
	ec2c := localstackEC2Client(t)

	currentEIP := allocateEIP(t, ec2c) // the EIP currently on the instance
	targetEIP := allocateEIP(t, ec2c)  // the EIP we want to bind (already associated elsewhere)
	eni1 := createENI(t, ec2c)         // instance's ENI (has currentEIP)
	eni2 := createENI(t, ec2c)         // some other ENI (currently holds targetEIP)

	// currentEIP → eni1: represents the instance's current public IP.
	_, err := ec2c.AssociateAddress(ctx, &ec2.AssociateAddressInput{
		AllocationId:       currentEIP.AllocationId,
		NetworkInterfaceId: eni1.NetworkInterfaceId,
	})
	if err != nil {
		t.Fatalf("pre-associate currentEIP→eni1: %v", err)
	}

	// targetEIP → eni2: the EIP is already in use and must be disassociated first.
	_, err = ec2c.AssociateAddress(ctx, &ec2.AssociateAddressInput{
		AllocationId:       targetEIP.AllocationId,
		NetworkInterfaceId: eni2.NetworkInterfaceId,
	})
	if err != nil {
		t.Fatalf("pre-associate targetEIP→eni2: %v", err)
	}

	imds := integrationIMDS(t, *currentEIP.PublicIp, "i-integration-test")
	b := NewBinder(ec2c, imds, integrationLogger())

	result, err := b.Bind(ctx, *targetEIP.PublicIp)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if result.AlreadyAssociated {
		t.Error("expected AlreadyAssociated=false, got true")
	}
	if result.AssociationID == "" {
		t.Error("expected non-empty AssociationID")
	}

	// Confirm targetEIP was moved to eni1.
	descOut, err := ec2c.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{
		AllocationIds: []string{*targetEIP.AllocationId},
	})
	if err != nil {
		t.Fatalf("DescribeAddresses after Bind: %v", err)
	}
	if len(descOut.Addresses) == 0 {
		t.Fatal("targetEIP not found after Bind")
	}
	addr := descOut.Addresses[0]
	if addr.AssociationId == nil {
		t.Error("targetEIP has no association after Bind")
	}
	if addr.NetworkInterfaceId == nil || *addr.NetworkInterfaceId != *eni1.NetworkInterfaceId {
		t.Errorf("targetEIP associated with ENI %v, want %s",
			addr.NetworkInterfaceId, *eni1.NetworkInterfaceId)
	}
}
