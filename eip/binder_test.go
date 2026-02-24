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
