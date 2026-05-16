package eip

import (
	"fmt"
	"net/netip"
	"os"
	"strings"
)

// Config holds the resolved configuration for EIP binding.
type Config struct {
	// TargetIP is the IPv4 Elastic IP address or IPv6 address to associate.
	TargetIP string
	// Family is the address family: "ipv4" or "ipv6".
	Family string
}

// ParseConfig resolves the target IP from CLI arguments and environment variables.
//
// If the first argument is "POD_NAME", it reads the POD_NAME environment variable,
// replaces hyphens with underscores, and uses the resulting key to look up the
// actual IP from the environment. This is useful when running as a Kubernetes
// init container.
//
// getenv is an injectable function for reading environment variables (typically os.Getenv).
func ParseConfig(args []string, getenv func(string) string) (*Config, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: aws-eip-binding <EIP>")
	}

	targetIP := args[0]

	if targetIP == "POD_NAME" {
		podName := getenv("POD_NAME")
		if podName == "" {
			return nil, fmt.Errorf("environment variable POD_NAME is empty")
		}
		envKey := strings.ReplaceAll(podName, "-", "_")
		targetIP = getenv(envKey)
		if targetIP == "" {
			return nil, fmt.Errorf("environment variable %s (from POD_NAME=%s) is empty", envKey, podName)
		}
	}

	ip, err := netip.ParseAddr(targetIP)
	if err != nil {
		return nil, fmt.Errorf("invalid IP address: %s", targetIP)
	}
	ip = ip.Unmap()
	family := IPFamilyIPv4
	if !ip.Is4() {
		family = IPFamilyIPv6
	}

	return &Config{TargetIP: ip.String(), Family: family}, nil
}

// ParseConfigFromOS is a convenience wrapper that calls ParseConfig with os.Args and os.Getenv.
func ParseConfigFromOS() (*Config, error) {
	if len(os.Args) < 2 {
		return nil, fmt.Errorf("usage: aws-eip-binding <EIP>")
	}
	return ParseConfig(os.Args[1:], os.Getenv)
}
