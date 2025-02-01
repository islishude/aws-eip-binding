package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	// AWS SDK v2 imports
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// getMetadataToken fetches an IMDSv2 token from the EC2 metadata service.
func getMetadataToken() (string, error) {
	// Create a PUT request to obtain the IMDSv2 token.
	req, err := http.NewRequest("PUT", "http://169.254.169.254/latest/api/token", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Add("X-aws-ec2-metadata-token-ttl-seconds", "300")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get metadata token: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read metadata token: %w", err)
	}
	return string(b), nil
}

// getMetadata retrieves metadata from EC2 instance by providing the token and metadata path.
func getMetadata(token, path string) (string, error) {
	// Create a GET request with the token in header.
	req, err := http.NewRequest("GET", "http://169.254.169.254/latest/"+path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create metadata request: %w", err)
	}
	req.Header.Add("X-aws-ec2-metadata-token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get metadata %s: %w", path, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read metadata %s: %w", path, err)
	}
	return string(b), nil
}

func main() {
	// Step 1: Validate input arguments.
	if len(os.Args) < 2 {
		fmt.Println("Usage: aws-eip-binding <EIP>")
		os.Exit(1)
	}
	targetIP := os.Args[1]
	log.Printf("Received target EIP: %s", targetIP)

	// Step 2: Validate that the provided IP is a valid IPv4 address.
	ip := net.ParseIP(targetIP)
	if ip == nil || ip.To4() == nil {
		fmt.Println("Invalid IPv4 address provided.")
		os.Exit(1)
	}

	// Step 3: Load AWS configuration and create an EC2 client.
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}
	log.Println("AWS configuration loaded successfully", "region", cfg.Region)
	ec2Client := ec2.NewFromConfig(cfg)

	// Step 4: Describe addresses for the provided EIP.
	log.Printf("Describing addresses")
	descOut, err := ec2Client.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{
		PublicIps: []string{targetIP},
	})
	if err != nil || len(descOut.Addresses) == 0 {
		log.Fatalf("failed to get addresses for %s: %v", targetIP, err)
	}
	address := descOut.Addresses[0]

	// Step 5: Obtain IMDSv2 metadata token.
	log.Println("Requesting IMDSv2 metadata token")
	mdToken, err := getMetadataToken()
	if err != nil {
		log.Fatalf("error fetching metadata token: %v", err)
	}

	// Step 6: Retrieve instance's public IPv4.
	log.Println("Fetching instance public IPv4 from metadata")
	instancePublicIP, err := getMetadata(mdToken, "meta-data/public-ipv4")
	if err != nil {
		log.Fatalf("error fetching public-ipv4: %v", err)
	}
	// If the target IP is already assigned to this instance, exit.
	if targetIP == instancePublicIP {
		fmt.Printf("EIP %s is already associated with this instance\n", targetIP)
		os.Exit(0)
	}

	// Step 7: Retrieve instance ID.
	log.Println("Fetching instance ID from metadata")
	instanceID, err := getMetadata(mdToken, "meta-data/instance-id")
	if err != nil {
		log.Fatalf("error fetching instance-id: %v", err)
	}

	// Step 8: If the EIP is associated with another instance, disassociate it.
	if address.AssociationId != nil {
		log.Printf("Disassociating EIP from previous association %s", *address.AssociationId)
		_, err = ec2Client.DisassociateAddress(ctx, &ec2.DisassociateAddressInput{
			AssociationId: address.AssociationId,
		})
		if err != nil {
			log.Fatalf("failed to disassociate EIP %s: %v", targetIP, err)
		}
	}

	// Step 9: Retrieve the network interface ID using the instance's public IP.
	log.Printf("Retrieving network interface for instance public IP %s", instancePublicIP)
	eniOut, err := ec2Client.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("addresses.association.public-ip"),
				Values: []string{instancePublicIP},
			},
		},
	})
	if err != nil || len(eniOut.NetworkInterfaces) == 0 {
		log.Fatalf("failed to get network interface for public IP %s: %v", instancePublicIP, err)
	}
	networkInterfaceID := eniOut.NetworkInterfaces[0].NetworkInterfaceId

	// Step 10: Associate the EIP with the current instance by specifying AllocationId and NetworkInterfaceId.
	log.Printf("Associating EIP %s with instance %s", targetIP, instanceID)
	associateInput := &ec2.AssociateAddressInput{
		AllocationId:       address.AllocationId,
		NetworkInterfaceId: networkInterfaceID,
		InstanceId:         aws.String(instanceID), // Optional for traceability.
	}
	_, err = ec2Client.AssociateAddress(ctx, associateInput)
	if err != nil {
		log.Fatalf("failed to associate EIP %s: %v", targetIP, err)
	}

	// Step 11: Confirm successful association.
	log.Printf("Successfully associated EIP %s with instance %s", targetIP, instanceID)
}
