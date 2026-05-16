package eip

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// --- Mock EC2 ---

type mockEC2 struct {
	describeAddressesFn         func(ctx context.Context, in *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error)
	disassociateAddressFn       func(ctx context.Context, in *ec2.DisassociateAddressInput) (*ec2.DisassociateAddressOutput, error)
	describeNetworkInterfacesFn func(ctx context.Context, in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error)
	associateAddressFn          func(ctx context.Context, in *ec2.AssociateAddressInput) (*ec2.AssociateAddressOutput, error)
	assignIpv6AddressesFn       func(ctx context.Context, in *ec2.AssignIpv6AddressesInput) (*ec2.AssignIpv6AddressesOutput, error)
	unassignIpv6AddressesFn     func(ctx context.Context, in *ec2.UnassignIpv6AddressesInput) (*ec2.UnassignIpv6AddressesOutput, error)
	describeSubnetsFn           func(ctx context.Context, in *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error)
}

func (m *mockEC2) DescribeAddresses(ctx context.Context, in *ec2.DescribeAddressesInput, _ ...func(*ec2.Options)) (*ec2.DescribeAddressesOutput, error) {
	return m.describeAddressesFn(ctx, in)
}

func (m *mockEC2) DisassociateAddress(ctx context.Context, in *ec2.DisassociateAddressInput, _ ...func(*ec2.Options)) (*ec2.DisassociateAddressOutput, error) {
	return m.disassociateAddressFn(ctx, in)
}

func (m *mockEC2) DescribeNetworkInterfaces(ctx context.Context, in *ec2.DescribeNetworkInterfacesInput, _ ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
	return m.describeNetworkInterfacesFn(ctx, in)
}

func (m *mockEC2) AssociateAddress(ctx context.Context, in *ec2.AssociateAddressInput, _ ...func(*ec2.Options)) (*ec2.AssociateAddressOutput, error) {
	return m.associateAddressFn(ctx, in)
}

func (m *mockEC2) AssignIpv6Addresses(ctx context.Context, in *ec2.AssignIpv6AddressesInput, _ ...func(*ec2.Options)) (*ec2.AssignIpv6AddressesOutput, error) {
	return m.assignIpv6AddressesFn(ctx, in)
}

func (m *mockEC2) UnassignIpv6Addresses(ctx context.Context, in *ec2.UnassignIpv6AddressesInput, _ ...func(*ec2.Options)) (*ec2.UnassignIpv6AddressesOutput, error) {
	return m.unassignIpv6AddressesFn(ctx, in)
}

func (m *mockEC2) DescribeSubnets(ctx context.Context, in *ec2.DescribeSubnetsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	return m.describeSubnetsFn(ctx, in)
}

// --- Mock IMDS ---

type mockIMDS struct {
	token    string
	metadata map[string]string
	tokenErr error
	mdErr    map[string]error
}

func (m *mockIMDS) GetToken() (string, error) {
	return m.token, m.tokenErr
}

func (m *mockIMDS) GetMetadata(_, path string) (string, error) {
	if m.mdErr != nil {
		if err, ok := m.mdErr[path]; ok {
			return "", err
		}
	}
	v, ok := m.metadata[path]
	if !ok {
		return "", errors.New("metadata not found: " + path)
	}
	return v, nil
}

// --- Helpers ---

func silentLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

func primaryENI(ipv6s ...string) types.NetworkInterface {
	addresses := make([]types.NetworkInterfaceIpv6Address, 0, len(ipv6s))
	for _, ipv6 := range ipv6s {
		addresses = append(addresses, types.NetworkInterfaceIpv6Address{
			Ipv6Address: new(ipv6),
		})
	}

	return types.NetworkInterface{
		NetworkInterfaceId: new("eni-primary"),
		SubnetId:           new("subnet-1"),
		Ipv6Addresses:      addresses,
	}
}

func subnetWithIPv6CIDR(cidr string) types.Subnet {
	return types.Subnet{
		SubnetId: new("subnet-1"),
		Ipv6CidrBlockAssociationSet: []types.SubnetIpv6CidrBlockAssociation{
			{Ipv6CidrBlock: new(cidr)},
		},
	}
}

// --- Tests ---

func TestBind_AlreadyAssociated(t *testing.T) {
	targetIP := "54.162.153.80"

	ec2Mock := &mockEC2{
		describeAddressesFn: func(_ context.Context, in *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
			return &ec2.DescribeAddressesOutput{
				Addresses: []types.Address{
					{
						PublicIp:     new(targetIP),
						AllocationId: new("eipalloc-111"),
					},
				},
			}, nil
		},
	}

	imdsMock := &mockIMDS{
		token: "tok",
		metadata: map[string]string{
			"meta-data/public-ipv4": targetIP,
			"meta-data/instance-id": "i-abc123",
		},
	}

	b := NewBinder(ec2Mock, imdsMock, silentLogger())
	result, err := b.Bind(context.Background(), targetIP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AlreadyAssociated {
		t.Error("expected AlreadyAssociated = true")
	}
	if result.InstanceID != "i-abc123" {
		t.Errorf("InstanceID = %q, want %q", result.InstanceID, "i-abc123")
	}
}

func TestBind_NewAssociation(t *testing.T) {
	targetIP := "54.162.153.80"

	ec2Mock := &mockEC2{
		describeAddressesFn: func(_ context.Context, _ *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
			return &ec2.DescribeAddressesOutput{
				Addresses: []types.Address{
					{
						PublicIp:     new(targetIP),
						AllocationId: new("eipalloc-111"),
					},
				},
			}, nil
		},
		describeNetworkInterfacesFn: func(_ context.Context, _ *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
			return &ec2.DescribeNetworkInterfacesOutput{
				NetworkInterfaces: []types.NetworkInterface{
					{NetworkInterfaceId: new("eni-aaa")},
				},
			}, nil
		},
		associateAddressFn: func(_ context.Context, in *ec2.AssociateAddressInput) (*ec2.AssociateAddressOutput, error) {
			if *in.AllocationId != "eipalloc-111" {
				t.Errorf("AllocationId = %q", *in.AllocationId)
			}
			if *in.NetworkInterfaceId != "eni-aaa" {
				t.Errorf("NetworkInterfaceId = %q", *in.NetworkInterfaceId)
			}
			return &ec2.AssociateAddressOutput{
				AssociationId: new("eipassoc-new"),
			}, nil
		},
	}

	imdsMock := &mockIMDS{
		token: "tok",
		metadata: map[string]string{
			"meta-data/public-ipv4": "10.0.0.1",
			"meta-data/instance-id": "i-myinst",
		},
	}

	b := NewBinder(ec2Mock, imdsMock, silentLogger())
	result, err := b.Bind(context.Background(), targetIP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AlreadyAssociated {
		t.Error("expected AlreadyAssociated = false")
	}
	if result.AssociationID != "eipassoc-new" {
		t.Errorf("AssociationID = %q, want %q", result.AssociationID, "eipassoc-new")
	}
	if result.InstanceID != "i-myinst" {
		t.Errorf("InstanceID = %q, want %q", result.InstanceID, "i-myinst")
	}
}

func TestBind_DisassociatesFirst(t *testing.T) {
	targetIP := "54.162.153.80"
	disassociated := false

	ec2Mock := &mockEC2{
		describeAddressesFn: func(_ context.Context, _ *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
			return &ec2.DescribeAddressesOutput{
				Addresses: []types.Address{
					{
						PublicIp:      new(targetIP),
						AllocationId:  new("eipalloc-111"),
						AssociationId: new("eipassoc-old"),
					},
				},
			}, nil
		},
		disassociateAddressFn: func(_ context.Context, in *ec2.DisassociateAddressInput) (*ec2.DisassociateAddressOutput, error) {
			if *in.AssociationId != "eipassoc-old" {
				t.Errorf("disassociate got AssociationId = %q", *in.AssociationId)
			}
			disassociated = true
			return &ec2.DisassociateAddressOutput{}, nil
		},
		describeNetworkInterfacesFn: func(_ context.Context, _ *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
			return &ec2.DescribeNetworkInterfacesOutput{
				NetworkInterfaces: []types.NetworkInterface{
					{NetworkInterfaceId: new("eni-bbb")},
				},
			}, nil
		},
		associateAddressFn: func(_ context.Context, _ *ec2.AssociateAddressInput) (*ec2.AssociateAddressOutput, error) {
			if !disassociated {
				t.Error("associate called before disassociate")
			}
			return &ec2.AssociateAddressOutput{
				AssociationId: new("eipassoc-new"),
			}, nil
		},
	}

	imdsMock := &mockIMDS{
		token: "tok",
		metadata: map[string]string{
			"meta-data/public-ipv4": "10.0.0.1",
			"meta-data/instance-id": "i-myinst",
		},
	}

	b := NewBinder(ec2Mock, imdsMock, silentLogger())
	result, err := b.Bind(context.Background(), targetIP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !disassociated {
		t.Error("expected disassociate to be called")
	}
	if result.AlreadyAssociated {
		t.Error("expected AlreadyAssociated = false")
	}
}

func TestBind_DescribeAddressesError(t *testing.T) {
	ec2Mock := &mockEC2{
		describeAddressesFn: func(_ context.Context, _ *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
			return nil, errors.New("aws error")
		},
	}
	imdsMock := &mockIMDS{token: "tok"}

	b := NewBinder(ec2Mock, imdsMock, silentLogger())
	_, err := b.Bind(context.Background(), "1.2.3.4")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBind_NoAddressesFound(t *testing.T) {
	ec2Mock := &mockEC2{
		describeAddressesFn: func(_ context.Context, _ *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
			return &ec2.DescribeAddressesOutput{Addresses: []types.Address{}}, nil
		},
	}
	imdsMock := &mockIMDS{token: "tok"}

	b := NewBinder(ec2Mock, imdsMock, silentLogger())
	_, err := b.Bind(context.Background(), "1.2.3.4")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBind_MetadataTokenError(t *testing.T) {
	ec2Mock := &mockEC2{
		describeAddressesFn: func(_ context.Context, _ *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
			return &ec2.DescribeAddressesOutput{
				Addresses: []types.Address{{PublicIp: new("1.2.3.4"), AllocationId: new("a")}},
			}, nil
		},
	}
	imdsMock := &mockIMDS{tokenErr: errors.New("imds down")}

	b := NewBinder(ec2Mock, imdsMock, silentLogger())
	_, err := b.Bind(context.Background(), "1.2.3.4")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBind_NoNetworkInterface(t *testing.T) {
	ec2Mock := &mockEC2{
		describeAddressesFn: func(_ context.Context, _ *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
			return &ec2.DescribeAddressesOutput{
				Addresses: []types.Address{{PublicIp: new("1.2.3.4"), AllocationId: new("a")}},
			}, nil
		},
		describeNetworkInterfacesFn: func(_ context.Context, _ *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
			return &ec2.DescribeNetworkInterfacesOutput{NetworkInterfaces: []types.NetworkInterface{}}, nil
		},
	}
	imdsMock := &mockIMDS{
		token: "tok",
		metadata: map[string]string{
			"meta-data/public-ipv4": "10.0.0.1",
			"meta-data/instance-id": "i-test",
		},
	}

	b := NewBinder(ec2Mock, imdsMock, silentLogger())
	_, err := b.Bind(context.Background(), "1.2.3.4")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBind_AssociateError(t *testing.T) {
	ec2Mock := &mockEC2{
		describeAddressesFn: func(_ context.Context, _ *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
			return &ec2.DescribeAddressesOutput{
				Addresses: []types.Address{{PublicIp: new("1.2.3.4"), AllocationId: new("a")}},
			}, nil
		},
		describeNetworkInterfacesFn: func(_ context.Context, _ *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
			return &ec2.DescribeNetworkInterfacesOutput{
				NetworkInterfaces: []types.NetworkInterface{{NetworkInterfaceId: new("eni-x")}},
			}, nil
		},
		associateAddressFn: func(_ context.Context, _ *ec2.AssociateAddressInput) (*ec2.AssociateAddressOutput, error) {
			return nil, errors.New("permission denied")
		},
	}
	imdsMock := &mockIMDS{
		token: "tok",
		metadata: map[string]string{
			"meta-data/public-ipv4": "10.0.0.1",
			"meta-data/instance-id": "i-test",
		},
	}

	b := NewBinder(ec2Mock, imdsMock, silentLogger())
	_, err := b.Bind(context.Background(), "1.2.3.4")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBind_DisassociateError(t *testing.T) {
	ec2Mock := &mockEC2{
		describeAddressesFn: func(_ context.Context, _ *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
			return &ec2.DescribeAddressesOutput{
				Addresses: []types.Address{
					{
						PublicIp:      new("1.2.3.4"),
						AllocationId:  new("a"),
						AssociationId: new("old-assoc"),
					},
				},
			}, nil
		},
		disassociateAddressFn: func(_ context.Context, _ *ec2.DisassociateAddressInput) (*ec2.DisassociateAddressOutput, error) {
			return nil, errors.New("cannot disassociate")
		},
	}
	imdsMock := &mockIMDS{
		token: "tok",
		metadata: map[string]string{
			"meta-data/public-ipv4": "10.0.0.1",
			"meta-data/instance-id": "i-test",
		},
	}

	b := NewBinder(ec2Mock, imdsMock, silentLogger())
	_, err := b.Bind(context.Background(), "1.2.3.4")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBindIPv6_AlreadyAssigned(t *testing.T) {
	targetIP := "2001:db8::10"

	ec2Mock := &mockEC2{
		describeNetworkInterfacesFn: func(_ context.Context, _ *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
			return &ec2.DescribeNetworkInterfacesOutput{
				NetworkInterfaces: []types.NetworkInterface{primaryENI(targetIP)},
			}, nil
		},
		describeSubnetsFn: func(_ context.Context, _ *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
			t.Fatal("DescribeSubnets should not be called when IPv6 is already assigned")
			return nil, nil
		},
		unassignIpv6AddressesFn: func(_ context.Context, _ *ec2.UnassignIpv6AddressesInput) (*ec2.UnassignIpv6AddressesOutput, error) {
			t.Fatal("UnassignIpv6Addresses should not be called when IPv6 is already assigned")
			return nil, nil
		},
		assignIpv6AddressesFn: func(_ context.Context, _ *ec2.AssignIpv6AddressesInput) (*ec2.AssignIpv6AddressesOutput, error) {
			t.Fatal("AssignIpv6Addresses should not be called when IPv6 is already assigned")
			return nil, nil
		},
	}
	imdsMock := &mockIMDS{
		token: "tok",
		metadata: map[string]string{
			"meta-data/instance-id": "i-v6",
		},
	}

	b := NewBinder(ec2Mock, imdsMock, silentLogger())
	result, err := b.Bind(context.Background(), targetIP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AlreadyAssociated {
		t.Error("expected AlreadyAssociated = true")
	}
	if result.Family != IPFamilyIPv6 {
		t.Errorf("Family = %q, want %q", result.Family, IPFamilyIPv6)
	}
	if result.TargetIP != targetIP {
		t.Errorf("TargetIP = %q, want %q", result.TargetIP, targetIP)
	}
	if result.NetworkInterfaceID != "eni-primary" {
		t.Errorf("NetworkInterfaceID = %q, want eni-primary", result.NetworkInterfaceID)
	}
	if result.AssociationID != "" {
		t.Errorf("AssociationID = %q, want empty", result.AssociationID)
	}
}

func TestBindIPv6_FreeAssignment(t *testing.T) {
	targetIP := "2001:db8::20"
	describeENICalls := 0

	ec2Mock := &mockEC2{
		describeNetworkInterfacesFn: func(_ context.Context, _ *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
			describeENICalls++
			switch describeENICalls {
			case 1:
				return &ec2.DescribeNetworkInterfacesOutput{
					NetworkInterfaces: []types.NetworkInterface{primaryENI()},
				}, nil
			case 2:
				return &ec2.DescribeNetworkInterfacesOutput{}, nil
			default:
				t.Fatalf("unexpected DescribeNetworkInterfaces call %d", describeENICalls)
				return nil, nil
			}
		},
		describeSubnetsFn: func(_ context.Context, in *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
			if len(in.SubnetIds) != 1 || in.SubnetIds[0] != "subnet-1" {
				t.Errorf("SubnetIds = %v, want [subnet-1]", in.SubnetIds)
			}
			return &ec2.DescribeSubnetsOutput{
				Subnets: []types.Subnet{subnetWithIPv6CIDR("2001:db8::/64")},
			}, nil
		},
		assignIpv6AddressesFn: func(_ context.Context, in *ec2.AssignIpv6AddressesInput) (*ec2.AssignIpv6AddressesOutput, error) {
			if *in.NetworkInterfaceId != "eni-primary" {
				t.Errorf("NetworkInterfaceId = %q, want eni-primary", *in.NetworkInterfaceId)
			}
			if len(in.Ipv6Addresses) != 1 || in.Ipv6Addresses[0] != targetIP {
				t.Errorf("Ipv6Addresses = %v, want [%s]", in.Ipv6Addresses, targetIP)
			}
			return &ec2.AssignIpv6AddressesOutput{
				AssignedIpv6Addresses: []string{targetIP},
			}, nil
		},
	}
	imdsMock := &mockIMDS{
		token: "tok",
		metadata: map[string]string{
			"meta-data/instance-id": "i-v6",
		},
	}

	b := NewBinder(ec2Mock, imdsMock, silentLogger())
	result, err := b.Bind(context.Background(), targetIP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AlreadyAssociated {
		t.Error("expected AlreadyAssociated = false")
	}
	if result.Family != IPFamilyIPv6 {
		t.Errorf("Family = %q, want %q", result.Family, IPFamilyIPv6)
	}
	if result.TargetIP != targetIP {
		t.Errorf("TargetIP = %q, want %q", result.TargetIP, targetIP)
	}
	if result.NetworkInterfaceID != "eni-primary" {
		t.Errorf("NetworkInterfaceID = %q, want eni-primary", result.NetworkInterfaceID)
	}
}

func TestBindIPv6_MovesFromOtherENI(t *testing.T) {
	targetIP := "2001:db8::30"
	describeENICalls := 0
	unassigned := false

	ec2Mock := &mockEC2{
		describeNetworkInterfacesFn: func(_ context.Context, _ *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
			describeENICalls++
			switch describeENICalls {
			case 1:
				return &ec2.DescribeNetworkInterfacesOutput{
					NetworkInterfaces: []types.NetworkInterface{primaryENI()},
				}, nil
			case 2:
				return &ec2.DescribeNetworkInterfacesOutput{
					NetworkInterfaces: []types.NetworkInterface{
						{NetworkInterfaceId: new("eni-old")},
					},
				}, nil
			default:
				t.Fatalf("unexpected DescribeNetworkInterfaces call %d", describeENICalls)
				return nil, nil
			}
		},
		describeSubnetsFn: func(_ context.Context, _ *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
			return &ec2.DescribeSubnetsOutput{
				Subnets: []types.Subnet{subnetWithIPv6CIDR("2001:db8::/64")},
			}, nil
		},
		unassignIpv6AddressesFn: func(_ context.Context, in *ec2.UnassignIpv6AddressesInput) (*ec2.UnassignIpv6AddressesOutput, error) {
			if *in.NetworkInterfaceId != "eni-old" {
				t.Errorf("unassign NetworkInterfaceId = %q, want eni-old", *in.NetworkInterfaceId)
			}
			if len(in.Ipv6Addresses) != 1 || in.Ipv6Addresses[0] != targetIP {
				t.Errorf("unassign Ipv6Addresses = %v, want [%s]", in.Ipv6Addresses, targetIP)
			}
			unassigned = true
			return &ec2.UnassignIpv6AddressesOutput{}, nil
		},
		assignIpv6AddressesFn: func(_ context.Context, in *ec2.AssignIpv6AddressesInput) (*ec2.AssignIpv6AddressesOutput, error) {
			if !unassigned {
				t.Error("assign called before unassign")
			}
			if *in.NetworkInterfaceId != "eni-primary" {
				t.Errorf("assign NetworkInterfaceId = %q, want eni-primary", *in.NetworkInterfaceId)
			}
			return &ec2.AssignIpv6AddressesOutput{
				AssignedIpv6Addresses: []string{targetIP},
			}, nil
		},
	}
	imdsMock := &mockIMDS{
		token: "tok",
		metadata: map[string]string{
			"meta-data/instance-id": "i-v6",
		},
	}

	b := NewBinder(ec2Mock, imdsMock, silentLogger())
	result, err := b.Bind(context.Background(), targetIP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AlreadyAssociated {
		t.Error("expected AlreadyAssociated = false")
	}
	if !unassigned {
		t.Error("expected UnassignIpv6Addresses to be called")
	}
}

func TestBindIPv6_TargetOutsideSubnetFailsBeforeUnassign(t *testing.T) {
	targetIP := "2001:db8:2::1"
	describeENICalls := 0

	ec2Mock := &mockEC2{
		describeNetworkInterfacesFn: func(_ context.Context, _ *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
			describeENICalls++
			if describeENICalls != 1 {
				t.Fatalf("unexpected DescribeNetworkInterfaces call %d", describeENICalls)
			}
			return &ec2.DescribeNetworkInterfacesOutput{
				NetworkInterfaces: []types.NetworkInterface{primaryENI()},
			}, nil
		},
		describeSubnetsFn: func(_ context.Context, _ *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
			return &ec2.DescribeSubnetsOutput{
				Subnets: []types.Subnet{subnetWithIPv6CIDR("2001:db8:1::/64")},
			}, nil
		},
		unassignIpv6AddressesFn: func(_ context.Context, _ *ec2.UnassignIpv6AddressesInput) (*ec2.UnassignIpv6AddressesOutput, error) {
			t.Fatal("UnassignIpv6Addresses should not be called when target IPv6 is outside subnet")
			return nil, nil
		},
		assignIpv6AddressesFn: func(_ context.Context, _ *ec2.AssignIpv6AddressesInput) (*ec2.AssignIpv6AddressesOutput, error) {
			t.Fatal("AssignIpv6Addresses should not be called when target IPv6 is outside subnet")
			return nil, nil
		},
	}
	imdsMock := &mockIMDS{
		token: "tok",
		metadata: map[string]string{
			"meta-data/instance-id": "i-v6",
		},
	}

	b := NewBinder(ec2Mock, imdsMock, silentLogger())
	_, err := b.Bind(context.Background(), targetIP)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBindIPv6_NoPrimaryNetworkInterface(t *testing.T) {
	ec2Mock := &mockEC2{
		describeNetworkInterfacesFn: func(_ context.Context, _ *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
			return &ec2.DescribeNetworkInterfacesOutput{}, nil
		},
	}
	imdsMock := &mockIMDS{
		token: "tok",
		metadata: map[string]string{
			"meta-data/instance-id": "i-v6",
		},
	}

	b := NewBinder(ec2Mock, imdsMock, silentLogger())
	_, err := b.Bind(context.Background(), "2001:db8::40")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBindIPv6_AssignError(t *testing.T) {
	targetIP := "2001:db8::50"
	describeENICalls := 0

	ec2Mock := &mockEC2{
		describeNetworkInterfacesFn: func(_ context.Context, _ *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
			describeENICalls++
			switch describeENICalls {
			case 1:
				return &ec2.DescribeNetworkInterfacesOutput{
					NetworkInterfaces: []types.NetworkInterface{primaryENI()},
				}, nil
			case 2:
				return &ec2.DescribeNetworkInterfacesOutput{}, nil
			default:
				t.Fatalf("unexpected DescribeNetworkInterfaces call %d", describeENICalls)
				return nil, nil
			}
		},
		describeSubnetsFn: func(_ context.Context, _ *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
			return &ec2.DescribeSubnetsOutput{
				Subnets: []types.Subnet{subnetWithIPv6CIDR("2001:db8::/64")},
			}, nil
		},
		assignIpv6AddressesFn: func(_ context.Context, _ *ec2.AssignIpv6AddressesInput) (*ec2.AssignIpv6AddressesOutput, error) {
			return nil, errors.New("assign denied")
		},
	}
	imdsMock := &mockIMDS{
		token: "tok",
		metadata: map[string]string{
			"meta-data/instance-id": "i-v6",
		},
	}

	b := NewBinder(ec2Mock, imdsMock, silentLogger())
	_, err := b.Bind(context.Background(), targetIP)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBindIPv6_UnassignError(t *testing.T) {
	targetIP := "2001:db8::60"
	describeENICalls := 0

	ec2Mock := &mockEC2{
		describeNetworkInterfacesFn: func(_ context.Context, _ *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
			describeENICalls++
			switch describeENICalls {
			case 1:
				return &ec2.DescribeNetworkInterfacesOutput{
					NetworkInterfaces: []types.NetworkInterface{primaryENI()},
				}, nil
			case 2:
				return &ec2.DescribeNetworkInterfacesOutput{
					NetworkInterfaces: []types.NetworkInterface{
						{NetworkInterfaceId: new("eni-old")},
					},
				}, nil
			default:
				t.Fatalf("unexpected DescribeNetworkInterfaces call %d", describeENICalls)
				return nil, nil
			}
		},
		describeSubnetsFn: func(_ context.Context, _ *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
			return &ec2.DescribeSubnetsOutput{
				Subnets: []types.Subnet{subnetWithIPv6CIDR("2001:db8::/64")},
			}, nil
		},
		unassignIpv6AddressesFn: func(_ context.Context, _ *ec2.UnassignIpv6AddressesInput) (*ec2.UnassignIpv6AddressesOutput, error) {
			return nil, errors.New("unassign denied")
		},
		assignIpv6AddressesFn: func(_ context.Context, _ *ec2.AssignIpv6AddressesInput) (*ec2.AssignIpv6AddressesOutput, error) {
			t.Fatal("AssignIpv6Addresses should not be called after unassign error")
			return nil, nil
		},
	}
	imdsMock := &mockIMDS{
		token: "tok",
		metadata: map[string]string{
			"meta-data/instance-id": "i-v6",
		},
	}

	b := NewBinder(ec2Mock, imdsMock, silentLogger())
	_, err := b.Bind(context.Background(), targetIP)
	if err == nil {
		t.Fatal("expected error")
	}
}
