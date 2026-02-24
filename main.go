package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
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
	logger.Printf("Target EIP: %s", cfg.TargetIP)

	// Load AWS configuration.
	ctx := context.Background()
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		logger.Fatalf("loading AWS config: %v", err)
	}
	logger.Printf("AWS configuration loaded (region=%s)", awsCfg.Region)

	// Create dependencies and bind.
	ec2Client := ec2.NewFromConfig(awsCfg)
	imds := eip.NewIMDSClient()
	binder := eip.NewBinder(ec2Client, imds, logger)

	result, err := binder.Bind(ctx, cfg.TargetIP)
	if err != nil {
		logger.Fatalf("bind: %v", err)
	}

	if result.AlreadyAssociated {
		logger.Printf("No changes needed – EIP already on instance %s", result.InstanceID)
	} else {
		logger.Printf("Done – association %s on instance %s", result.AssociationID, result.InstanceID)
	}
}
