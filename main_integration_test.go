package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestIntegration_CLIIPv4BindLocalStack(t *testing.T) {
	ctx := context.Background()
	endpoint := localstackEndpointForCLIIntegration(t)
	ec2c := localstackEC2ClientForCLIIntegration(t, endpoint)

	currentEIP := allocateCLIIntegrationEIP(t, ec2c)
	targetEIP := allocateCLIIntegrationEIP(t, ec2c)
	if currentEIP.PublicIp == nil {
		t.Fatal("current EIP has no PublicIp")
	}
	if targetEIP.PublicIp == nil {
		t.Fatal("target EIP has no PublicIp")
	}

	eni := createCLIIntegrationENI(t, ec2c)
	if eni.NetworkInterfaceId == nil {
		t.Fatal("created ENI has no NetworkInterfaceId")
	}

	assocOut, err := ec2c.AssociateAddress(ctx, &ec2.AssociateAddressInput{
		AllocationId:       currentEIP.AllocationId,
		NetworkInterfaceId: eni.NetworkInterfaceId,
	})
	if err != nil {
		t.Fatalf("pre-associate current EIP: %v", err)
	}
	t.Cleanup(func() {
		if assocOut.AssociationId != nil {
			_, _ = ec2c.DisassociateAddress(context.Background(), &ec2.DisassociateAddressInput{
				AssociationId: assocOut.AssociationId,
			})
		}
	})

	imds := cliIntegrationIMDSServer(t, *currentEIP.PublicIp, "i-cli-integration")
	exe := filepath.Join(t.TempDir(), "aws-eip-binding")
	build := exec.Command("go", "build", "-o", exe, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}

	cmd := exec.Command(exe, *targetEIP.PublicIp)
	cmd.Env = cliIntegrationEnv(os.Environ(), endpoint, imds.URL)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run CLI: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Done") {
		t.Fatalf("CLI output did not report completion:\n%s", out)
	}

	descOut, err := ec2c.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{
		AllocationIds: []string{*targetEIP.AllocationId},
	})
	if err != nil {
		t.Fatalf("DescribeAddresses after CLI bind: %v", err)
	}
	if len(descOut.Addresses) != 1 {
		t.Fatalf("target EIP descriptions = %d, want 1", len(descOut.Addresses))
	}
	addr := descOut.Addresses[0]
	if addr.AssociationId == nil {
		t.Fatal("target EIP has no association after CLI bind")
	}
	t.Cleanup(func() {
		_, _ = ec2c.DisassociateAddress(context.Background(), &ec2.DisassociateAddressInput{
			AssociationId: addr.AssociationId,
		})
	})
	if addr.NetworkInterfaceId == nil || *addr.NetworkInterfaceId != *eni.NetworkInterfaceId {
		t.Fatalf("target EIP associated with ENI %v, want %s", addr.NetworkInterfaceId, *eni.NetworkInterfaceId)
	}
}

func localstackEndpointForCLIIntegration(t *testing.T) string {
	t.Helper()
	if os.Getenv("ENABLE_INTEGRATION_TESTS") != "true" {
		t.Skip("ENABLE_INTEGRATION_TESTS is not true - skipping integration test")
	}

	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	if endpoint == "" {
		endpoint = os.Getenv("AWS_ENDPOINT")
	}
	if endpoint == "" {
		t.Skip("AWS_ENDPOINT_URL not set - skipping integration test")
	}
	return endpoint
}

func localstackEC2ClientForCLIIntegration(t *testing.T, endpoint string) *ec2.Client {
	t.Helper()

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

func allocateCLIIntegrationEIP(t *testing.T, ec2c *ec2.Client) *ec2.AllocateAddressOutput {
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

func createCLIIntegrationENI(t *testing.T, ec2c *ec2.Client) *types.NetworkInterface {
	t.Helper()

	subnets, err := ec2c.DescribeSubnets(context.Background(), &ec2.DescribeSubnetsInput{})
	if err != nil || len(subnets.Subnets) == 0 {
		t.Skipf("no subnets available in LocalStack (err=%v) - skipping", err)
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

func cliIntegrationIMDSServer(t *testing.T, publicIP, instanceID string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/latest/api/token":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("cli-integration-token")) //nolint:errcheck
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
	return srv
}

func cliIntegrationEnv(base []string, endpoint, metadataEndpoint string) []string {
	overrides := map[string]string{
		"AWS_ACCESS_KEY_ID":                   "test",
		"AWS_SECRET_ACCESS_KEY":               "test",
		"AWS_SESSION_TOKEN":                   "",
		"AWS_REGION":                          "us-east-1",
		"AWS_DEFAULT_REGION":                  "us-east-1",
		"AWS_ENDPOINT":                        endpoint,
		"AWS_ENDPOINT_URL":                    endpoint,
		"AWS_ENDPOINT_URL_EC2":                endpoint,
		"AWS_EC2_METADATA_DISABLED":           "false",
		"AWS_EC2_METADATA_SERVICE_ENDPOINT":   metadataEndpoint,
		"AWS_IGNORE_CONFIGURED_ENDPOINT_URLS": "false",
		"AWS_SDK_LOAD_CONFIG":                 "0",
	}
	drop := map[string]struct{}{
		"AWS_CONFIG_FILE":             {},
		"AWS_DEFAULT_PROFILE":         {},
		"AWS_PROFILE":                 {},
		"AWS_SHARED_CREDENTIALS_FILE": {},
	}

	env := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, shouldDrop := drop[key]; shouldDrop {
			continue
		}
		if _, isOverride := overrides[key]; isOverride {
			continue
		}
		env = append(env, entry)
	}

	for _, key := range []string{
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
		"AWS_REGION",
		"AWS_DEFAULT_REGION",
		"AWS_ENDPOINT",
		"AWS_ENDPOINT_URL",
		"AWS_ENDPOINT_URL_EC2",
		"AWS_EC2_METADATA_DISABLED",
		"AWS_EC2_METADATA_SERVICE_ENDPOINT",
		"AWS_IGNORE_CONFIGURED_ENDPOINT_URLS",
		"AWS_SDK_LOAD_CONFIG",
	} {
		env = append(env, key+"="+overrides[key])
	}
	return env
}
