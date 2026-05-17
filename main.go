package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	ec2imds "github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/islishude/aws-eip-binding/eip"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)

	// Parse CLI arguments and environment variables.
	cfg, err := eip.ParseConfigFromOS()
	if err != nil {
		logger.Fatalf("config: %v", err)
	}
	logger.Printf("Target IP: %s", cfg.TargetIP)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Load AWS configuration.
	awsCfg, err := config.LoadDefaultConfig(ctx, awsLoadOptionsForConfig(cfg)...)
	if err != nil {
		logger.Fatalf("loading AWS config: %v", err)
	}
	logger.Printf("AWS configuration loaded (region=%s)", awsCfg.Region)

	// Create dependencies and bind.
	ec2Client := ec2.NewFromConfig(awsCfg)
	imds := imdsClientForConfig(cfg)
	binder := eip.NewBinder(ec2Client, imds, logger)

	result, err := binder.Bind(ctx, cfg.TargetIP)
	if err != nil {
		logger.Fatalf("bind: %v", err)
	}

	if result.AlreadyAssociated {
		logger.Printf("No changes needed – %s %s already on instance %s", result.Family, result.TargetIP, result.InstanceID)
	} else if result.Family == eip.IPFamilyIPv6 {
		logger.Printf("Done – IPv6 %s on ENI %s for instance %s", result.TargetIP, result.NetworkInterfaceID, result.InstanceID)
	} else {
		logger.Printf("Done – association %s on instance %s", result.AssociationID, result.InstanceID)
	}
}

func awsLoadOptionsForConfig(cfg *eip.Config) []func(*config.LoadOptions) error {
	if cfg.Family != eip.IPFamilyIPv6 {
		return nil
	}
	return []func(*config.LoadOptions) error{
		config.WithEC2IMDSEndpointMode(ec2imds.EndpointModeStateIPv6),
		config.WithUseDualStackEndpoint(aws.DualStackEndpointStateEnabled),
	}
}

func imdsClientForConfig(cfg *eip.Config) *eip.IMDSClient {
	if cfg.Family == eip.IPFamilyIPv6 {
		return eip.NewIMDSClient(eip.WithIMDSEndpointMode(eip.IMDSEndpointModeIPv6))
	}
	return eip.NewIMDSClient()
}
