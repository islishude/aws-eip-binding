package eip

import (
	"context"
	"fmt"
	"log"
	"net/netip"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

const (
	IPFamilyIPv4 = "ipv4"
	IPFamilyIPv6 = "ipv6"
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
	// AlreadyAssociated is true when the target IP was already on this instance.
	AlreadyAssociated bool
	// AssociationID is the new IPv4 EIP association ID (empty for IPv6 or when AlreadyAssociated).
	AssociationID string
	// InstanceID is the current instance's ID.
	InstanceID string
	// Family is the address family: "ipv4" or "ipv6".
	Family string
	// TargetIP is the normalized target IP address.
	TargetIP string
	// NetworkInterfaceID is the ENI that holds the target IP after binding.
	NetworkInterfaceID string
}

// Bind associates the given IPv4 Elastic IP or IPv6 address with the current EC2 instance.
//
// IPv4 uses Elastic IP allocation APIs. IPv6 uses ENI IPv6 assignment APIs.
func (b *Binder) Bind(ctx context.Context, targetIP string) (*BindResult, error) {
	targetAddr, err := parseTargetAddr(targetIP)
	if err != nil {
		return nil, err
	}
	targetIP = targetAddr.String()
	if targetAddr.Is4() {
		return b.bindIPv4(ctx, targetIP)
	}
	return b.bindIPv6(ctx, targetAddr)
}

func parseTargetAddr(targetIP string) (netip.Addr, error) {
	addr, err := netip.ParseAddr(targetIP)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid IP address: %s", targetIP)
	}
	return addr.Unmap(), nil
}

func (b *Binder) getMetadataTokenAndInstanceID(ctx context.Context) (string, string, error) {
	mdToken, err := b.IMDS.GetToken(ctx)
	if err != nil {
		return "", "", fmt.Errorf("get metadata token: %w", err)
	}

	instanceID, err := b.IMDS.GetMetadata(ctx, mdToken, "meta-data/instance-id")
	if err != nil {
		return "", "", fmt.Errorf("get instance-id: %w", err)
	}

	return mdToken, instanceID, nil
}

func (b *Binder) bindIPv4(ctx context.Context, targetIP string) (*BindResult, error) {
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

	// 2. Get instance metadata and current primary ENI.
	_, instanceID, err := b.getMetadataTokenAndInstanceID(ctx)
	if err != nil {
		return nil, err
	}
	primaryENI, err := b.findPrimaryNetworkInterface(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	networkInterfaceID := primaryENI.NetworkInterfaceId
	if networkInterfaceID == nil {
		return nil, fmt.Errorf("primary network interface for instance %s has no ID", instanceID)
	}

	// 3. Already associated - nothing to do.
	if address.NetworkInterfaceId != nil && *address.NetworkInterfaceId == *networkInterfaceID {
		b.Logger.Printf("EIP %s is already associated with instance %s", targetIP, instanceID)
		return &BindResult{
			AlreadyAssociated:  true,
			InstanceID:         instanceID,
			Family:             IPFamilyIPv4,
			TargetIP:           targetIP,
			NetworkInterfaceID: *networkInterfaceID,
		}, nil
	}
	if address.AllocationId == nil {
		return nil, fmt.Errorf("address %s has no allocation ID", targetIP)
	}

	b.Logger.Printf("Associating EIP %s (allocation=%s) to ENI %s on instance %s",
		targetIP, *address.AllocationId, *networkInterfaceID, instanceID)

	assocOut, err := b.EC2.AssociateAddress(ctx, &ec2.AssociateAddressInput{
		AllocationId:       address.AllocationId,
		AllowReassociation: new(true),
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
		AlreadyAssociated:  false,
		AssociationID:      assocID,
		InstanceID:         instanceID,
		Family:             IPFamilyIPv4,
		TargetIP:           targetIP,
		NetworkInterfaceID: *networkInterfaceID,
	}, nil
}

func (b *Binder) bindIPv6(ctx context.Context, targetAddr netip.Addr) (*BindResult, error) {
	targetIP := targetAddr.String()

	_, instanceID, err := b.getMetadataTokenAndInstanceID(ctx)
	if err != nil {
		return nil, err
	}

	primaryENI, err := b.findPrimaryNetworkInterface(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	networkInterfaceID := primaryENI.NetworkInterfaceId
	if networkInterfaceID == nil {
		return nil, fmt.Errorf("primary network interface for instance %s has no ID", instanceID)
	}
	if primaryENI.SubnetId == nil {
		return nil, fmt.Errorf("primary network interface %s has no subnet ID", *networkInterfaceID)
	}

	if hasIPv6(primaryENI, targetIP) {
		b.Logger.Printf("IPv6 %s is already assigned to ENI %s on instance %s", targetIP, *networkInterfaceID, instanceID)
		return &BindResult{
			AlreadyAssociated:  true,
			InstanceID:         instanceID,
			Family:             IPFamilyIPv6,
			TargetIP:           targetIP,
			NetworkInterfaceID: *networkInterfaceID,
		}, nil
	}

	if err := b.ensureIPv6InSubnet(ctx, targetAddr, *primaryENI.SubnetId, *networkInterfaceID); err != nil {
		return nil, err
	}

	currentENI, err := b.findNetworkInterfaceByIPv6(ctx, targetIP)
	if err != nil {
		return nil, err
	}
	if currentENI != nil && currentENI.NetworkInterfaceId == nil {
		return nil, fmt.Errorf("network interface for IPv6 %s has no ID", targetIP)
	}
	if currentENI != nil && *currentENI.NetworkInterfaceId == *networkInterfaceID {
		b.Logger.Printf("IPv6 %s is already assigned to ENI %s on instance %s", targetIP, *networkInterfaceID, instanceID)
		return &BindResult{
			AlreadyAssociated:  true,
			InstanceID:         instanceID,
			Family:             IPFamilyIPv6,
			TargetIP:           targetIP,
			NetworkInterfaceID: *networkInterfaceID,
		}, nil
	}
	if currentENI != nil {
		b.Logger.Printf("Unassigning IPv6 %s from ENI %s", targetIP, *currentENI.NetworkInterfaceId)
		_, err = b.EC2.UnassignIpv6Addresses(ctx, &ec2.UnassignIpv6AddressesInput{
			NetworkInterfaceId: currentENI.NetworkInterfaceId,
			Ipv6Addresses:      []string{targetIP},
		})
		if err != nil {
			return nil, fmt.Errorf("unassign IPv6 %s from ENI %s: %w", targetIP, *currentENI.NetworkInterfaceId, err)
		}
	}

	b.Logger.Printf("Assigning IPv6 %s to ENI %s on instance %s", targetIP, *networkInterfaceID, instanceID)
	assignOut, err := b.EC2.AssignIpv6Addresses(ctx, &ec2.AssignIpv6AddressesInput{
		NetworkInterfaceId: networkInterfaceID,
		Ipv6Addresses:      []string{targetIP},
	})
	if err != nil {
		return nil, fmt.Errorf("assign IPv6 %s to ENI %s: %w", targetIP, *networkInterfaceID, err)
	}

	if len(assignOut.AssignedIpv6Addresses) > 0 {
		targetIP = assignOut.AssignedIpv6Addresses[0]
	}

	b.Logger.Printf("Successfully assigned IPv6 %s to ENI %s on instance %s", targetIP, *networkInterfaceID, instanceID)
	return &BindResult{
		AlreadyAssociated:  false,
		InstanceID:         instanceID,
		Family:             IPFamilyIPv6,
		TargetIP:           targetIP,
		NetworkInterfaceID: *networkInterfaceID,
	}, nil
}

func (b *Binder) findPrimaryNetworkInterface(ctx context.Context, instanceID string) (*types.NetworkInterface, error) {
	eniOut, err := b.EC2.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
		Filters: []types.Filter{
			{
				Name:   new("attachment.instance-id"),
				Values: []string{instanceID},
			},
			{
				Name:   new("attachment.device-index"),
				Values: []string{"0"},
			},
			{
				Name:   new("attachment.status"),
				Values: []string{"attached"},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("describe primary network interface for instance %s: %w", instanceID, err)
	}
	if len(eniOut.NetworkInterfaces) == 0 {
		return nil, fmt.Errorf("no primary network interface found for instance %s", instanceID)
	}
	return &eniOut.NetworkInterfaces[0], nil
}

func (b *Binder) findNetworkInterfaceByIPv6(ctx context.Context, targetIP string) (*types.NetworkInterface, error) {
	eniOut, err := b.EC2.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
		Filters: []types.Filter{
			{
				Name:   new("ipv6-addresses.ipv6-address"),
				Values: []string{targetIP},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("describe network interface for IPv6 %s: %w", targetIP, err)
	}
	if len(eniOut.NetworkInterfaces) == 0 {
		return nil, nil
	}
	return &eniOut.NetworkInterfaces[0], nil
}

func (b *Binder) ensureIPv6InSubnet(ctx context.Context, targetAddr netip.Addr, subnetID, networkInterfaceID string) error {
	subnetsOut, err := b.EC2.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		SubnetIds: []string{subnetID},
	})
	if err != nil {
		return fmt.Errorf("describe subnet %s for ENI %s: %w", subnetID, networkInterfaceID, err)
	}
	if len(subnetsOut.Subnets) == 0 {
		return fmt.Errorf("subnet %s for ENI %s not found", subnetID, networkInterfaceID)
	}

	for _, assoc := range subnetsOut.Subnets[0].Ipv6CidrBlockAssociationSet {
		if assoc.Ipv6CidrBlock == nil {
			continue
		}
		prefix, err := netip.ParsePrefix(*assoc.Ipv6CidrBlock)
		if err != nil {
			return fmt.Errorf("parse IPv6 CIDR %s for subnet %s: %w", *assoc.Ipv6CidrBlock, subnetID, err)
		}
		if prefix.Contains(targetAddr) {
			return nil
		}
	}

	return fmt.Errorf("IPv6 %s is not in subnet %s IPv6 CIDR blocks", targetAddr.String(), subnetID)
}

func hasIPv6(eni *types.NetworkInterface, targetIP string) bool {
	for _, ipv6 := range eni.Ipv6Addresses {
		if ipv6.Ipv6Address != nil && *ipv6.Ipv6Address == targetIP {
			return true
		}
	}
	return false
}
