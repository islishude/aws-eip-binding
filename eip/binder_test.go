package eip

import (
	"context"
	"errors"
	"io"
	"log"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type describeAddressesFunc func(*ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error)
type describeNetworkInterfacesFunc func(*ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error)
type associateAddressFunc func(*ec2.AssociateAddressInput) (*ec2.AssociateAddressOutput, error)
type assignIPv6AddressesFunc func(*ec2.AssignIpv6AddressesInput) (*ec2.AssignIpv6AddressesOutput, error)
type unassignIPv6AddressesFunc func(*ec2.UnassignIpv6AddressesInput) (*ec2.UnassignIpv6AddressesOutput, error)
type describeSubnetsFunc func(*ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error)

type fakeEC2 struct {
	t *testing.T

	calls []string

	describeAddresses         describeAddressesFunc
	describeNetworkInterfaces []describeNetworkInterfacesFunc
	associateAddress          associateAddressFunc
	assignIPv6Addresses       assignIPv6AddressesFunc
	unassignIPv6Addresses     unassignIPv6AddressesFunc
	describeSubnets           describeSubnetsFunc
}

func newFakeEC2(t *testing.T) *fakeEC2 {
	t.Helper()
	return &fakeEC2{t: t}
}

func (f *fakeEC2) DescribeAddresses(_ context.Context, in *ec2.DescribeAddressesInput, _ ...func(*ec2.Options)) (*ec2.DescribeAddressesOutput, error) {
	f.t.Helper()
	f.record("DescribeAddresses")
	if f.describeAddresses == nil {
		f.unexpected("DescribeAddresses")
		return nil, nil
	}
	return f.describeAddresses(in)
}

func (f *fakeEC2) DescribeNetworkInterfaces(_ context.Context, in *ec2.DescribeNetworkInterfacesInput, _ ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
	f.t.Helper()
	f.record("DescribeNetworkInterfaces")
	if len(f.describeNetworkInterfaces) == 0 {
		f.unexpected("DescribeNetworkInterfaces")
		return nil, nil
	}
	fn := f.describeNetworkInterfaces[0]
	f.describeNetworkInterfaces = f.describeNetworkInterfaces[1:]
	return fn(in)
}

func (f *fakeEC2) AssociateAddress(_ context.Context, in *ec2.AssociateAddressInput, _ ...func(*ec2.Options)) (*ec2.AssociateAddressOutput, error) {
	f.t.Helper()
	f.record("AssociateAddress")
	if f.associateAddress == nil {
		f.unexpected("AssociateAddress")
		return nil, nil
	}
	return f.associateAddress(in)
}

func (f *fakeEC2) AssignIpv6Addresses(_ context.Context, in *ec2.AssignIpv6AddressesInput, _ ...func(*ec2.Options)) (*ec2.AssignIpv6AddressesOutput, error) {
	f.t.Helper()
	f.record("AssignIpv6Addresses")
	if f.assignIPv6Addresses == nil {
		f.unexpected("AssignIpv6Addresses")
		return nil, nil
	}
	return f.assignIPv6Addresses(in)
}

func (f *fakeEC2) UnassignIpv6Addresses(_ context.Context, in *ec2.UnassignIpv6AddressesInput, _ ...func(*ec2.Options)) (*ec2.UnassignIpv6AddressesOutput, error) {
	f.t.Helper()
	f.record("UnassignIpv6Addresses")
	if f.unassignIPv6Addresses == nil {
		f.unexpected("UnassignIpv6Addresses")
		return nil, nil
	}
	return f.unassignIPv6Addresses(in)
}

func (f *fakeEC2) DescribeSubnets(_ context.Context, in *ec2.DescribeSubnetsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	f.t.Helper()
	f.record("DescribeSubnets")
	if f.describeSubnets == nil {
		f.unexpected("DescribeSubnets")
		return nil, nil
	}
	return f.describeSubnets(in)
}

func (f *fakeEC2) record(call string) {
	f.t.Helper()
	f.calls = append(f.calls, call)
}

func (f *fakeEC2) unexpected(call string) {
	f.t.Helper()
	f.t.Fatalf("unexpected EC2 call %s", call)
}

func (f *fakeEC2) assertCalls(want []string) {
	f.t.Helper()
	if !slices.Equal(f.calls, want) {
		f.t.Fatalf("EC2 calls = %v, want %v", f.calls, want)
	}
	if len(f.describeNetworkInterfaces) != 0 {
		f.t.Fatalf("unused DescribeNetworkInterfaces handlers: %d", len(f.describeNetworkInterfaces))
	}
}

type fakeIMDS struct {
	t *testing.T

	token       string
	tokenErr    error
	metadata    map[string]string
	metadataErr map[string]error
	calls       []string
}

func newFakeIMDS(t *testing.T, metadata map[string]string) *fakeIMDS {
	t.Helper()
	return &fakeIMDS{
		t:        t,
		token:    "tok",
		metadata: metadata,
	}
}

func (f *fakeIMDS) GetToken(_ context.Context) (string, error) {
	f.t.Helper()
	f.calls = append(f.calls, "GetToken")
	return f.token, f.tokenErr
}

func (f *fakeIMDS) GetMetadata(_ context.Context, token, path string) (string, error) {
	f.t.Helper()
	f.calls = append(f.calls, "GetMetadata:"+path)
	if token != f.token {
		f.t.Errorf("metadata token = %q, want %q", token, f.token)
	}
	if err := f.metadataErr[path]; err != nil {
		return "", err
	}
	value, ok := f.metadata[path]
	if !ok {
		f.t.Fatalf("unexpected metadata path %q", path)
	}
	return value, nil
}

func (f *fakeIMDS) assertCalls(want []string) {
	f.t.Helper()
	if !slices.Equal(f.calls, want) {
		f.t.Fatalf("IMDS calls = %v, want %v", f.calls, want)
	}
}

func silentLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

func elasticAddress(publicIP, allocationID, associationID string) types.Address {
	addr := types.Address{
		PublicIp:     new(publicIP),
		AllocationId: new(allocationID),
	}
	if associationID != "" {
		addr.AssociationId = new(associationID)
	}
	return addr
}

func networkInterface(id string) types.NetworkInterface {
	return types.NetworkInterface{
		NetworkInterfaceId: new(id),
	}
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

func assertBindResult(t *testing.T, got *BindResult, want BindResult) {
	t.Helper()
	if got == nil {
		t.Fatal("result is nil")
	}
	if got.AlreadyAssociated != want.AlreadyAssociated {
		t.Errorf("AlreadyAssociated = %v, want %v", got.AlreadyAssociated, want.AlreadyAssociated)
	}
	if got.AssociationID != want.AssociationID {
		t.Errorf("AssociationID = %q, want %q", got.AssociationID, want.AssociationID)
	}
	if got.InstanceID != want.InstanceID {
		t.Errorf("InstanceID = %q, want %q", got.InstanceID, want.InstanceID)
	}
	if got.Family != want.Family {
		t.Errorf("Family = %q, want %q", got.Family, want.Family)
	}
	if got.TargetIP != want.TargetIP {
		t.Errorf("TargetIP = %q, want %q", got.TargetIP, want.TargetIP)
	}
	if got.NetworkInterfaceID != want.NetworkInterfaceID {
		t.Errorf("NetworkInterfaceID = %q, want %q", got.NetworkInterfaceID, want.NetworkInterfaceID)
	}
}

func requireStrings(t *testing.T, got []string, want []string, label string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}

func requireStringPtr(t *testing.T, got *string, want string, label string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is nil, want %q", label, want)
	}
	if *got != want {
		t.Fatalf("%s = %q, want %q", label, *got, want)
	}
}

func requireBoolPtr(t *testing.T, got *bool, want bool, label string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is nil, want %v", label, want)
	}
	if *got != want {
		t.Fatalf("%s = %v, want %v", label, *got, want)
	}
}

func requireFilter(t *testing.T, filters []types.Filter, name string, values ...string) {
	t.Helper()
	for _, filter := range filters {
		if filter.Name == nil || *filter.Name != name {
			continue
		}
		requireStrings(t, filter.Values, values, "filter "+name)
		return
	}
	t.Fatalf("missing filter %q in %#v", name, filters)
}

func requireDescribeAddressInput(t *testing.T, in *ec2.DescribeAddressesInput, targetIP string) {
	t.Helper()
	requireStrings(t, in.PublicIps, []string{targetIP}, "PublicIps")
}

func requirePrimaryENIFilters(t *testing.T, in *ec2.DescribeNetworkInterfacesInput, instanceID string) {
	t.Helper()
	requireFilter(t, in.Filters, "attachment.instance-id", instanceID)
	requireFilter(t, in.Filters, "attachment.device-index", "0")
	requireFilter(t, in.Filters, "attachment.status", "attached")
}

func requireIPv6ENIFilter(t *testing.T, in *ec2.DescribeNetworkInterfacesInput, targetIP string) {
	t.Helper()
	requireFilter(t, in.Filters, "ipv6-addresses.ipv6-address", targetIP)
}

func requireSubnetInput(t *testing.T, in *ec2.DescribeSubnetsInput, subnetID string) {
	t.Helper()
	requireStrings(t, in.SubnetIds, []string{subnetID}, "SubnetIds")
}

func instanceMetadata(instanceID string) map[string]string {
	return map[string]string{
		"meta-data/instance-id": instanceID,
	}
}

func TestBindIPv4Scenarios(t *testing.T) {
	const (
		targetIP   = "54.162.153.80"
		instanceID = "i-ipv4"
		allocation = "eipalloc-111"
	)

	tests := []struct {
		name          string
		setup         func(t *testing.T) (*fakeEC2, *fakeIMDS)
		wantResult    *BindResult
		wantErr       bool
		wantEC2Calls  []string
		wantIMDSCalls []string
	}{
		{
			name: "already associated",
			setup: func(t *testing.T) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeAddresses = func(in *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
					requireDescribeAddressInput(t, in, targetIP)
					address := elasticAddress(targetIP, allocation, "")
					address.NetworkInterfaceId = new("eni-primary")
					return &ec2.DescribeAddressesOutput{
						Addresses: []types.Address{address},
					}, nil
				}
				ec2Fake.describeNetworkInterfaces = []describeNetworkInterfacesFunc{
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requirePrimaryENIFilters(t, in, instanceID)
						return &ec2.DescribeNetworkInterfacesOutput{
							NetworkInterfaces: []types.NetworkInterface{networkInterface("eni-primary")},
						}, nil
					},
				}
				return ec2Fake, newFakeIMDS(t, instanceMetadata(instanceID))
			},
			wantResult: &BindResult{
				AlreadyAssociated:  true,
				InstanceID:         instanceID,
				Family:             IPFamilyIPv4,
				TargetIP:           targetIP,
				NetworkInterfaceID: "eni-primary",
			},
			wantEC2Calls: []string{"DescribeAddresses", "DescribeNetworkInterfaces"},
			wantIMDSCalls: []string{
				"GetToken",
				"GetMetadata:meta-data/instance-id",
			},
		},
		{
			name: "reassociates from secondary ENI on same instance",
			setup: func(t *testing.T) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeAddresses = func(in *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
					requireDescribeAddressInput(t, in, targetIP)
					address := elasticAddress(targetIP, allocation, "eipassoc-secondary")
					address.InstanceId = new(instanceID)
					address.NetworkInterfaceId = new("eni-secondary")
					return &ec2.DescribeAddressesOutput{
						Addresses: []types.Address{address},
					}, nil
				}
				ec2Fake.describeNetworkInterfaces = []describeNetworkInterfacesFunc{
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requirePrimaryENIFilters(t, in, instanceID)
						return &ec2.DescribeNetworkInterfacesOutput{
							NetworkInterfaces: []types.NetworkInterface{networkInterface("eni-primary")},
						}, nil
					},
				}
				ec2Fake.associateAddress = func(in *ec2.AssociateAddressInput) (*ec2.AssociateAddressOutput, error) {
					requireStringPtr(t, in.AllocationId, allocation, "AllocationId")
					requireBoolPtr(t, in.AllowReassociation, true, "AllowReassociation")
					requireStringPtr(t, in.NetworkInterfaceId, "eni-primary", "NetworkInterfaceId")
					return &ec2.AssociateAddressOutput{AssociationId: new("eipassoc-primary")}, nil
				}
				return ec2Fake, newFakeIMDS(t, instanceMetadata(instanceID))
			},
			wantResult: &BindResult{
				AssociationID:      "eipassoc-primary",
				InstanceID:         instanceID,
				Family:             IPFamilyIPv4,
				TargetIP:           targetIP,
				NetworkInterfaceID: "eni-primary",
			},
			wantEC2Calls: []string{"DescribeAddresses", "DescribeNetworkInterfaces", "AssociateAddress"},
			wantIMDSCalls: []string{
				"GetToken",
				"GetMetadata:meta-data/instance-id",
			},
		},
		{
			name: "new association",
			setup: func(t *testing.T) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeAddresses = func(in *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
					requireDescribeAddressInput(t, in, targetIP)
					return &ec2.DescribeAddressesOutput{
						Addresses: []types.Address{elasticAddress(targetIP, allocation, "")},
					}, nil
				}
				ec2Fake.describeNetworkInterfaces = []describeNetworkInterfacesFunc{
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requirePrimaryENIFilters(t, in, instanceID)
						return &ec2.DescribeNetworkInterfacesOutput{
							NetworkInterfaces: []types.NetworkInterface{networkInterface("eni-primary")},
						}, nil
					},
				}
				ec2Fake.associateAddress = func(in *ec2.AssociateAddressInput) (*ec2.AssociateAddressOutput, error) {
					requireStringPtr(t, in.AllocationId, allocation, "AllocationId")
					requireBoolPtr(t, in.AllowReassociation, true, "AllowReassociation")
					requireStringPtr(t, in.NetworkInterfaceId, "eni-primary", "NetworkInterfaceId")
					return &ec2.AssociateAddressOutput{AssociationId: new("eipassoc-new")}, nil
				}
				return ec2Fake, newFakeIMDS(t, instanceMetadata(instanceID))
			},
			wantResult: &BindResult{
				AssociationID:      "eipassoc-new",
				InstanceID:         instanceID,
				Family:             IPFamilyIPv4,
				TargetIP:           targetIP,
				NetworkInterfaceID: "eni-primary",
			},
			wantEC2Calls: []string{"DescribeAddresses", "DescribeNetworkInterfaces", "AssociateAddress"},
			wantIMDSCalls: []string{
				"GetToken",
				"GetMetadata:meta-data/instance-id",
			},
		},
		{
			name: "reassociates existing address without explicit disassociate",
			setup: func(t *testing.T) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeAddresses = func(in *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
					requireDescribeAddressInput(t, in, targetIP)
					address := elasticAddress(targetIP, allocation, "eipassoc-old")
					address.NetworkInterfaceId = new("eni-old")
					return &ec2.DescribeAddressesOutput{
						Addresses: []types.Address{address},
					}, nil
				}
				ec2Fake.describeNetworkInterfaces = []describeNetworkInterfacesFunc{
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requirePrimaryENIFilters(t, in, instanceID)
						return &ec2.DescribeNetworkInterfacesOutput{
							NetworkInterfaces: []types.NetworkInterface{networkInterface("eni-primary")},
						}, nil
					},
				}
				ec2Fake.associateAddress = func(in *ec2.AssociateAddressInput) (*ec2.AssociateAddressOutput, error) {
					requireStringPtr(t, in.AllocationId, allocation, "AllocationId")
					requireBoolPtr(t, in.AllowReassociation, true, "AllowReassociation")
					requireStringPtr(t, in.NetworkInterfaceId, "eni-primary", "NetworkInterfaceId")
					return &ec2.AssociateAddressOutput{AssociationId: new("eipassoc-new")}, nil
				}
				return ec2Fake, newFakeIMDS(t, instanceMetadata(instanceID))
			},
			wantResult: &BindResult{
				AssociationID:      "eipassoc-new",
				InstanceID:         instanceID,
				Family:             IPFamilyIPv4,
				TargetIP:           targetIP,
				NetworkInterfaceID: "eni-primary",
			},
			wantEC2Calls: []string{"DescribeAddresses", "DescribeNetworkInterfaces", "AssociateAddress"},
			wantIMDSCalls: []string{
				"GetToken",
				"GetMetadata:meta-data/instance-id",
			},
		},
		{
			name: "describe addresses error",
			setup: func(t *testing.T) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeAddresses = func(in *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
					requireDescribeAddressInput(t, in, targetIP)
					return nil, errors.New("aws error")
				}
				return ec2Fake, newFakeIMDS(t, nil)
			},
			wantErr:       true,
			wantEC2Calls:  []string{"DescribeAddresses"},
			wantIMDSCalls: nil,
		},
		{
			name: "address not found",
			setup: func(t *testing.T) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeAddresses = func(in *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
					requireDescribeAddressInput(t, in, targetIP)
					return &ec2.DescribeAddressesOutput{}, nil
				}
				return ec2Fake, newFakeIMDS(t, nil)
			},
			wantErr:       true,
			wantEC2Calls:  []string{"DescribeAddresses"},
			wantIMDSCalls: nil,
		},
		{
			name: "metadata token error",
			setup: func(t *testing.T) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeAddresses = func(in *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
					requireDescribeAddressInput(t, in, targetIP)
					return &ec2.DescribeAddressesOutput{
						Addresses: []types.Address{elasticAddress(targetIP, allocation, "")},
					}, nil
				}
				imdsFake := newFakeIMDS(t, nil)
				imdsFake.tokenErr = errors.New("imds down")
				return ec2Fake, imdsFake
			},
			wantErr:       true,
			wantEC2Calls:  []string{"DescribeAddresses"},
			wantIMDSCalls: []string{"GetToken"},
		},
		{
			name: "instance metadata error",
			setup: func(t *testing.T) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeAddresses = func(in *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
					requireDescribeAddressInput(t, in, targetIP)
					return &ec2.DescribeAddressesOutput{
						Addresses: []types.Address{elasticAddress(targetIP, allocation, "")},
					}, nil
				}
				imdsFake := newFakeIMDS(t, nil)
				imdsFake.metadataErr = map[string]error{
					"meta-data/instance-id": errors.New("instance id unavailable"),
				}
				return ec2Fake, imdsFake
			},
			wantErr:      true,
			wantEC2Calls: []string{"DescribeAddresses"},
			wantIMDSCalls: []string{
				"GetToken",
				"GetMetadata:meta-data/instance-id",
			},
		},
		{
			name: "no primary network interface",
			setup: func(t *testing.T) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeAddresses = func(in *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
					requireDescribeAddressInput(t, in, targetIP)
					return &ec2.DescribeAddressesOutput{
						Addresses: []types.Address{elasticAddress(targetIP, allocation, "")},
					}, nil
				}
				ec2Fake.describeNetworkInterfaces = []describeNetworkInterfacesFunc{
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requirePrimaryENIFilters(t, in, instanceID)
						return &ec2.DescribeNetworkInterfacesOutput{}, nil
					},
				}
				return ec2Fake, newFakeIMDS(t, instanceMetadata(instanceID))
			},
			wantErr:      true,
			wantEC2Calls: []string{"DescribeAddresses", "DescribeNetworkInterfaces"},
			wantIMDSCalls: []string{
				"GetToken",
				"GetMetadata:meta-data/instance-id",
			},
		},
		{
			name: "address has no allocation ID",
			setup: func(t *testing.T) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeAddresses = func(in *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
					requireDescribeAddressInput(t, in, targetIP)
					return &ec2.DescribeAddressesOutput{
						Addresses: []types.Address{{PublicIp: new(targetIP)}},
					}, nil
				}
				ec2Fake.describeNetworkInterfaces = []describeNetworkInterfacesFunc{
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requirePrimaryENIFilters(t, in, instanceID)
						return &ec2.DescribeNetworkInterfacesOutput{
							NetworkInterfaces: []types.NetworkInterface{networkInterface("eni-primary")},
						}, nil
					},
				}
				return ec2Fake, newFakeIMDS(t, instanceMetadata(instanceID))
			},
			wantErr:      true,
			wantEC2Calls: []string{"DescribeAddresses", "DescribeNetworkInterfaces"},
			wantIMDSCalls: []string{
				"GetToken",
				"GetMetadata:meta-data/instance-id",
			},
		},
		{
			name: "associate error",
			setup: func(t *testing.T) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeAddresses = func(in *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
					requireDescribeAddressInput(t, in, targetIP)
					return &ec2.DescribeAddressesOutput{
						Addresses: []types.Address{elasticAddress(targetIP, allocation, "")},
					}, nil
				}
				ec2Fake.describeNetworkInterfaces = []describeNetworkInterfacesFunc{
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requirePrimaryENIFilters(t, in, instanceID)
						return &ec2.DescribeNetworkInterfacesOutput{
							NetworkInterfaces: []types.NetworkInterface{networkInterface("eni-primary")},
						}, nil
					},
				}
				ec2Fake.associateAddress = func(in *ec2.AssociateAddressInput) (*ec2.AssociateAddressOutput, error) {
					requireStringPtr(t, in.AllocationId, allocation, "AllocationId")
					requireBoolPtr(t, in.AllowReassociation, true, "AllowReassociation")
					requireStringPtr(t, in.NetworkInterfaceId, "eni-primary", "NetworkInterfaceId")
					return nil, errors.New("permission denied")
				}
				return ec2Fake, newFakeIMDS(t, instanceMetadata(instanceID))
			},
			wantErr:      true,
			wantEC2Calls: []string{"DescribeAddresses", "DescribeNetworkInterfaces", "AssociateAddress"},
			wantIMDSCalls: []string{
				"GetToken",
				"GetMetadata:meta-data/instance-id",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ec2Fake, imdsFake := tt.setup(t)
			result, err := NewBinder(ec2Fake, imdsFake, silentLogger()).Bind(context.Background(), targetIP)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			} else {
				assertBindResult(t, result, *tt.wantResult)
			}

			ec2Fake.assertCalls(tt.wantEC2Calls)
			imdsFake.assertCalls(tt.wantIMDSCalls)
		})
	}
}

func TestBindIPv6Scenarios(t *testing.T) {
	const instanceID = "i-ipv6"

	tests := []struct {
		name          string
		targetIP      string
		setup         func(t *testing.T, targetIP string) (*fakeEC2, *fakeIMDS)
		wantResult    *BindResult
		wantErr       bool
		wantEC2Calls  []string
		wantIMDSCalls []string
	}{
		{
			name:     "already assigned to primary ENI",
			targetIP: "2001:db8::10",
			setup: func(t *testing.T, targetIP string) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeNetworkInterfaces = []describeNetworkInterfacesFunc{
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requirePrimaryENIFilters(t, in, instanceID)
						return &ec2.DescribeNetworkInterfacesOutput{
							NetworkInterfaces: []types.NetworkInterface{primaryENI(targetIP)},
						}, nil
					},
				}
				return ec2Fake, newFakeIMDS(t, instanceMetadata(instanceID))
			},
			wantResult: &BindResult{
				AlreadyAssociated:  true,
				InstanceID:         instanceID,
				Family:             IPFamilyIPv6,
				TargetIP:           "2001:db8::10",
				NetworkInterfaceID: "eni-primary",
			},
			wantEC2Calls: []string{"DescribeNetworkInterfaces"},
			wantIMDSCalls: []string{
				"GetToken",
				"GetMetadata:meta-data/instance-id",
			},
		},
		{
			name:     "free assignment",
			targetIP: "2001:db8::20",
			setup: func(t *testing.T, targetIP string) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeNetworkInterfaces = []describeNetworkInterfacesFunc{
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requirePrimaryENIFilters(t, in, instanceID)
						return &ec2.DescribeNetworkInterfacesOutput{
							NetworkInterfaces: []types.NetworkInterface{primaryENI()},
						}, nil
					},
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requireIPv6ENIFilter(t, in, targetIP)
						return &ec2.DescribeNetworkInterfacesOutput{}, nil
					},
				}
				ec2Fake.describeSubnets = func(in *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
					requireSubnetInput(t, in, "subnet-1")
					return &ec2.DescribeSubnetsOutput{
						Subnets: []types.Subnet{subnetWithIPv6CIDR("2001:db8::/64")},
					}, nil
				}
				ec2Fake.assignIPv6Addresses = func(in *ec2.AssignIpv6AddressesInput) (*ec2.AssignIpv6AddressesOutput, error) {
					requireStringPtr(t, in.NetworkInterfaceId, "eni-primary", "NetworkInterfaceId")
					requireStrings(t, in.Ipv6Addresses, []string{targetIP}, "Ipv6Addresses")
					return &ec2.AssignIpv6AddressesOutput{
						AssignedIpv6Addresses: []string{targetIP},
					}, nil
				}
				return ec2Fake, newFakeIMDS(t, instanceMetadata(instanceID))
			},
			wantResult: &BindResult{
				InstanceID:         instanceID,
				Family:             IPFamilyIPv6,
				TargetIP:           "2001:db8::20",
				NetworkInterfaceID: "eni-primary",
			},
			wantEC2Calls: []string{"DescribeNetworkInterfaces", "DescribeSubnets", "DescribeNetworkInterfaces", "AssignIpv6Addresses"},
			wantIMDSCalls: []string{
				"GetToken",
				"GetMetadata:meta-data/instance-id",
			},
		},
		{
			name:     "moves from another ENI",
			targetIP: "2001:db8::30",
			setup: func(t *testing.T, targetIP string) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeNetworkInterfaces = []describeNetworkInterfacesFunc{
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requirePrimaryENIFilters(t, in, instanceID)
						return &ec2.DescribeNetworkInterfacesOutput{
							NetworkInterfaces: []types.NetworkInterface{primaryENI()},
						}, nil
					},
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requireIPv6ENIFilter(t, in, targetIP)
						return &ec2.DescribeNetworkInterfacesOutput{
							NetworkInterfaces: []types.NetworkInterface{networkInterface("eni-old")},
						}, nil
					},
				}
				ec2Fake.describeSubnets = func(in *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
					requireSubnetInput(t, in, "subnet-1")
					return &ec2.DescribeSubnetsOutput{
						Subnets: []types.Subnet{subnetWithIPv6CIDR("2001:db8::/64")},
					}, nil
				}
				ec2Fake.unassignIPv6Addresses = func(in *ec2.UnassignIpv6AddressesInput) (*ec2.UnassignIpv6AddressesOutput, error) {
					requireStringPtr(t, in.NetworkInterfaceId, "eni-old", "NetworkInterfaceId")
					requireStrings(t, in.Ipv6Addresses, []string{targetIP}, "Ipv6Addresses")
					return &ec2.UnassignIpv6AddressesOutput{}, nil
				}
				ec2Fake.assignIPv6Addresses = func(in *ec2.AssignIpv6AddressesInput) (*ec2.AssignIpv6AddressesOutput, error) {
					requireStringPtr(t, in.NetworkInterfaceId, "eni-primary", "NetworkInterfaceId")
					requireStrings(t, in.Ipv6Addresses, []string{targetIP}, "Ipv6Addresses")
					return &ec2.AssignIpv6AddressesOutput{
						AssignedIpv6Addresses: []string{targetIP},
					}, nil
				}
				return ec2Fake, newFakeIMDS(t, instanceMetadata(instanceID))
			},
			wantResult: &BindResult{
				InstanceID:         instanceID,
				Family:             IPFamilyIPv6,
				TargetIP:           "2001:db8::30",
				NetworkInterfaceID: "eni-primary",
			},
			wantEC2Calls: []string{"DescribeNetworkInterfaces", "DescribeSubnets", "DescribeNetworkInterfaces", "UnassignIpv6Addresses", "AssignIpv6Addresses"},
			wantIMDSCalls: []string{
				"GetToken",
				"GetMetadata:meta-data/instance-id",
			},
		},
		{
			name:     "target outside primary subnet fails before unassign",
			targetIP: "2001:db8:2::1",
			setup: func(t *testing.T, _ string) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeNetworkInterfaces = []describeNetworkInterfacesFunc{
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requirePrimaryENIFilters(t, in, instanceID)
						return &ec2.DescribeNetworkInterfacesOutput{
							NetworkInterfaces: []types.NetworkInterface{primaryENI()},
						}, nil
					},
				}
				ec2Fake.describeSubnets = func(in *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
					requireSubnetInput(t, in, "subnet-1")
					return &ec2.DescribeSubnetsOutput{
						Subnets: []types.Subnet{subnetWithIPv6CIDR("2001:db8:1::/64")},
					}, nil
				}
				return ec2Fake, newFakeIMDS(t, instanceMetadata(instanceID))
			},
			wantErr:      true,
			wantEC2Calls: []string{"DescribeNetworkInterfaces", "DescribeSubnets"},
			wantIMDSCalls: []string{
				"GetToken",
				"GetMetadata:meta-data/instance-id",
			},
		},
		{
			name:     "no primary network interface",
			targetIP: "2001:db8::40",
			setup: func(t *testing.T, _ string) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeNetworkInterfaces = []describeNetworkInterfacesFunc{
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requirePrimaryENIFilters(t, in, instanceID)
						return &ec2.DescribeNetworkInterfacesOutput{}, nil
					},
				}
				return ec2Fake, newFakeIMDS(t, instanceMetadata(instanceID))
			},
			wantErr:      true,
			wantEC2Calls: []string{"DescribeNetworkInterfaces"},
			wantIMDSCalls: []string{
				"GetToken",
				"GetMetadata:meta-data/instance-id",
			},
		},
		{
			name:     "assign error",
			targetIP: "2001:db8::50",
			setup: func(t *testing.T, targetIP string) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeNetworkInterfaces = []describeNetworkInterfacesFunc{
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requirePrimaryENIFilters(t, in, instanceID)
						return &ec2.DescribeNetworkInterfacesOutput{
							NetworkInterfaces: []types.NetworkInterface{primaryENI()},
						}, nil
					},
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requireIPv6ENIFilter(t, in, targetIP)
						return &ec2.DescribeNetworkInterfacesOutput{}, nil
					},
				}
				ec2Fake.describeSubnets = func(in *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
					requireSubnetInput(t, in, "subnet-1")
					return &ec2.DescribeSubnetsOutput{
						Subnets: []types.Subnet{subnetWithIPv6CIDR("2001:db8::/64")},
					}, nil
				}
				ec2Fake.assignIPv6Addresses = func(in *ec2.AssignIpv6AddressesInput) (*ec2.AssignIpv6AddressesOutput, error) {
					requireStringPtr(t, in.NetworkInterfaceId, "eni-primary", "NetworkInterfaceId")
					requireStrings(t, in.Ipv6Addresses, []string{targetIP}, "Ipv6Addresses")
					return nil, errors.New("assign denied")
				}
				return ec2Fake, newFakeIMDS(t, instanceMetadata(instanceID))
			},
			wantErr:      true,
			wantEC2Calls: []string{"DescribeNetworkInterfaces", "DescribeSubnets", "DescribeNetworkInterfaces", "AssignIpv6Addresses"},
			wantIMDSCalls: []string{
				"GetToken",
				"GetMetadata:meta-data/instance-id",
			},
		},
		{
			name:     "unassign error",
			targetIP: "2001:db8::60",
			setup: func(t *testing.T, targetIP string) (*fakeEC2, *fakeIMDS) {
				ec2Fake := newFakeEC2(t)
				ec2Fake.describeNetworkInterfaces = []describeNetworkInterfacesFunc{
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requirePrimaryENIFilters(t, in, instanceID)
						return &ec2.DescribeNetworkInterfacesOutput{
							NetworkInterfaces: []types.NetworkInterface{primaryENI()},
						}, nil
					},
					func(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
						requireIPv6ENIFilter(t, in, targetIP)
						return &ec2.DescribeNetworkInterfacesOutput{
							NetworkInterfaces: []types.NetworkInterface{networkInterface("eni-old")},
						}, nil
					},
				}
				ec2Fake.describeSubnets = func(in *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
					requireSubnetInput(t, in, "subnet-1")
					return &ec2.DescribeSubnetsOutput{
						Subnets: []types.Subnet{subnetWithIPv6CIDR("2001:db8::/64")},
					}, nil
				}
				ec2Fake.unassignIPv6Addresses = func(in *ec2.UnassignIpv6AddressesInput) (*ec2.UnassignIpv6AddressesOutput, error) {
					requireStringPtr(t, in.NetworkInterfaceId, "eni-old", "NetworkInterfaceId")
					requireStrings(t, in.Ipv6Addresses, []string{targetIP}, "Ipv6Addresses")
					return nil, errors.New("unassign denied")
				}
				return ec2Fake, newFakeIMDS(t, instanceMetadata(instanceID))
			},
			wantErr:      true,
			wantEC2Calls: []string{"DescribeNetworkInterfaces", "DescribeSubnets", "DescribeNetworkInterfaces", "UnassignIpv6Addresses"},
			wantIMDSCalls: []string{
				"GetToken",
				"GetMetadata:meta-data/instance-id",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ec2Fake, imdsFake := tt.setup(t, tt.targetIP)
			result, err := NewBinder(ec2Fake, imdsFake, silentLogger()).Bind(context.Background(), tt.targetIP)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			} else {
				assertBindResult(t, result, *tt.wantResult)
			}

			ec2Fake.assertCalls(tt.wantEC2Calls)
			imdsFake.assertCalls(tt.wantIMDSCalls)
		})
	}
}
