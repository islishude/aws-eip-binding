# AWS EIP Binding CLI

This CLI tool associates an IPv4 Elastic IP (EIP), or moves a specified IPv6 address, to the current EC2 instance using AWS SDK for Go.

## Usage

1. Build the application:

   ```
   go build -o aws-eip-binding
   ```

2. Run the tool with your target IP:

   ```
   ./aws-eip-binding <IP>
   ```

   IPv4 targets use Elastic IP association APIs. IPv6 targets are assigned to the current instance's primary ENI; if the IPv6 address is already assigned to another ENI, the tool unassigns it first and then assigns it to the current primary ENI. The IPv6 address must belong to the current primary ENI subnet's IPv6 CIDR block.

## Prerequisites

1. You're using IMDSv2

2. For IPv6 targets, the instance must support the IMDS IPv6 endpoint (`http://[fd00:ec2::254]`) and EC2 dual-stack service endpoints. The IMDS endpoint can still be overridden with `AWS_EC2_METADATA_SERVICE_ENDPOINT` for custom environments.

3. Ensure that the IAM role or user has permissions similar to the following:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ec2:AssociateAddress",
        "ec2:AssignIpv6Addresses",
        "ec2:DescribeAddresses",
        "ec2:DescribeNetworkInterfaces",
        "ec2:DescribeSubnets",
        "ec2:DescribeTags",
        "ec2:DisassociateAddress",
        "ec2:UnassignIpv6Addresses"
      ],
      "Resource": "*"
    }
  ]
}
```

`ec2:DescribeNetworkInterfaces` should be validated against the
[AWS Service Authorization Reference](https://docs.aws.amazon.com/service-authorization/latest/reference/list_amazonec2.html)
or an opt-in test in a real AWS account. LocalStack can exercise the EC2 API
state flow, but it is not the authority for this permission: LocalStack
[IAM policy enforcement](https://docs.localstack.cloud/aws/capabilities/security-testing/iam-policy-enforcement/)
is disabled by default and its
[IAM coverage](https://docs.localstack.cloud/aws/capabilities/security-testing/iam-coverage/)
does not verify every EC2 action.

## Testing

Run unit tests with:

```sh
go test ./...
```

Run the LocalStack-backed integration suite with:

```sh
ENABLE_INTEGRATION_TESTS=true AWS_ENDPOINT_URL=http://localhost:4566 go test ./... -run TestIntegration
```

These tests cover IPv4 EIP behavior against LocalStack, including a capability
probe for the EC2 APIs and filters this tool depends on, plus a CLI-level run
against a test IMDS endpoint. They do not validate IAM least-privilege policy or
IPv6 behavior.

Run read-only real AWS E2E checks from an EC2 instance with:

```sh
ENABLE_AWS_E2E_TESTS=true go test ./... -run TestAWSE2E
```

The AWS E2E tests are skipped by default. With only `ENABLE_AWS_E2E_TESTS=true`,
they verify real IMDSv2 access and `DescribeNetworkInterfaces` filters for the
current instance. Mutating bind tests require explicit targets:

```sh
ENABLE_AWS_E2E_TESTS=true AWS_E2E_TARGET_IPV4=203.0.113.10 go test ./eip -run TestAWSE2E_BindTargetIPv4
ENABLE_AWS_E2E_TESTS=true AWS_E2E_TARGET_IPV6=2001:db8::10 go test ./eip -run TestAWSE2E_BindTargetIPv6
```

Only run mutating AWS E2E tests on a disposable EC2 instance or isolated account:
they can move, assign, or unassign addresses and change the instance's public
entry point. IPv6 E2E is skipped when the primary ENI subnet has no IPv6 CIDR
block.

### Using POD_NAME Argument

If you pass "POD_NAME" as the CLI argument, the program will:

1. Retrieve the environment variable POD_NAME.
2. Replace all hyphens (-) in its value with underscores (\_), it's due to hyphens cannot be used in environment variable names.
3. Use the resulting string as the key to fetch the actual IP from the environment.

For example, if your environment is configured as follows:

- POD_NAME=app-config
- app_config=54.162.153.80

Running:
aws-eip-binding POD_NAME
will set the target IP to 54.162.153.80.

The resolved value can also be an IPv6 address, for example `2001:db8::1234`.

For example in Kubernetes, you can use the following snippet to set the environment variable:

```yaml
initContainers:
  - name: eip
    image: ghcr.io/islishude/aws-eip-binding
    imagePullPolicy: Always
    args: ["POD_NAME"]
    env:
      - name: "POD_NAME"
        valueFrom:
          fieldRef:
            apiVersion: v1
            fieldPath: metadata.name
      - name: "test_0"
        value: "54.162.153.80"
```
