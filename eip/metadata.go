package eip

import (
	"context"

	ec2imds "github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
)

// MetadataClient abstracts EC2 instance metadata retrieval.
type MetadataClient interface {
	GetMetadata(ctx context.Context, params *ec2imds.GetMetadataInput, optFns ...func(*ec2imds.Options)) (*ec2imds.GetMetadataOutput, error)
}
