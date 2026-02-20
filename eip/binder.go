package eip

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// Binder performs EIP association with the current EC2 instance.
type Binder struct {
	EC2    EC2API
	IMDS   MetadataClient
	Logger *log.Logger
}

// NewBinder creates a Binder with the given dependencies.
func NewBinder(ec2Client EC2API, imds MetadataClient, logger *log.Logger) *Binder {
	if logger == nil {
		logger = log.Default()
	}
	return &Binder{
		EC2:    ec2Client,
		IMDS:   imds,
		Logger: logger,
	}
}

// BindResult describes the outcome of a Bind operation.
type BindResult struct {
	// AlreadyAssociated is true when the EIP was already on this instance.
	AlreadyAssociated bool
	// AssociationID is the new association ID (empty when AlreadyAssociated).
	AssociationID string
	// InstanceID is the current instance's ID.
	InstanceID string
}

// Bind associates the given Elastic IP with the current EC2 instance.
//
// It will:
//  1. Look up the EIP allocation.
//  2. Fetch the instance's public IP and instance ID via IMDS.
//  3. If the EIP is already on this instance, return early.
//  4. If the EIP is associated elsewhere, disassociate it first.
//  5. Find the network interface of this instance and associate the EIP.
func (b *Binder) Bind(ctx context.Context, targetIP string) (*BindResult, error) {
	// 1. Describe the EIP allocation.
	descOut, err := b.EC2.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{
		PublicIps: []string{targetIP},
	})
	if err != nil {
		return nil, fmt.Errorf("describe addresses for %s: %w", targetIP, err)
	}
	if len(descOut.Addresses) == 0 {
		return nil, fmt.Errorf("no addresses found for %s", targetIP)
	}
	address := descOut.Addresses[0]

	// 2. Get instance metadata.
	mdToken, err := b.IMDS.GetToken()
	if err != nil {
		return nil, fmt.Errorf("get metadata token: %w", err)
	}

	instancePublicIP, err := b.IMDS.GetMetadata(mdToken, "meta-data/public-ipv4")
	if err != nil {
		return nil, fmt.Errorf("get public-ipv4: %w", err)
	}

	instanceID, err := b.IMDS.GetMetadata(mdToken, "meta-data/instance-id")
	if err != nil {
		return nil, fmt.Errorf("get instance-id: %w", err)
	}

	// 3. Already associated – nothing to do.
	if targetIP == instancePublicIP {
		b.Logger.Printf("EIP %s is already associated with instance %s", targetIP, instanceID)
		return &BindResult{
			AlreadyAssociated: true,
			InstanceID:        instanceID,
		}, nil
	}

	// 4. Disassociate from previous instance if needed.
	if address.AssociationId != nil {
		b.Logger.Printf("Disassociating EIP from previous association %s", *address.AssociationId)
		_, err = b.EC2.DisassociateAddress(ctx, &ec2.DisassociateAddressInput{
			AssociationId: address.AssociationId,
		})
		if err != nil {
			return nil, fmt.Errorf("disassociate EIP %s: %w", targetIP, err)
		}
	}

	// 5. Find the network interface and associate.
	eniOut, err := b.EC2.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
		Filters: []types.Filter{
			{
				Name:   new("addresses.association.public-ip"),
				Values: []string{instancePublicIP},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("describe network interfaces for %s: %w", instancePublicIP, err)
	}
	if len(eniOut.NetworkInterfaces) == 0 {
		return nil, fmt.Errorf("no network interface found for public IP %s", instancePublicIP)
	}

	networkInterfaceID := eniOut.NetworkInterfaces[0].NetworkInterfaceId
	b.Logger.Printf("Associating EIP %s (allocation=%s) to ENI %s on instance %s",
		targetIP, *address.AllocationId, *networkInterfaceID, instanceID)

	assocOut, err := b.EC2.AssociateAddress(ctx, &ec2.AssociateAddressInput{
		AllocationId:       address.AllocationId,
		NetworkInterfaceId: networkInterfaceID,
	})
	if err != nil {
		return nil, fmt.Errorf("associate EIP %s with instance %s: %w", targetIP, instanceID, err)
	}

	assocID := ""
	if assocOut.AssociationId != nil {
		assocID = *assocOut.AssociationId
	}

	b.Logger.Printf("Successfully associated EIP %s with instance %s (association=%s)", targetIP, instanceID, assocID)
	return &BindResult{
		AlreadyAssociated: false,
		AssociationID:     assocID,
		InstanceID:        instanceID,
	}, nil
}
