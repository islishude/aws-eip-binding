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

## Execution Flow

```mermaid
sequenceDiagram
    autonumber
    actor User
    participant CLI as aws-eip-binding
    participant Config as eip.ParseConfig
    participant AWSConfig as AWS SDK config
    participant Binder as eip.Binder
    participant IMDS as EC2 IMDSv2
    participant EC2 as EC2 API

    User->>CLI: Run with IP or POD_NAME
    CLI->>Config: Parse CLI args and environment
    alt POD_NAME argument
        Config->>Config: Read POD_NAME, replace '-' with '_'
        Config->>Config: Resolve target IP from matching env var
    end
    Config-->>CLI: Normalized target IP and address family
    CLI->>AWSConfig: Load default AWS config
    alt IPv6 target
        AWSConfig->>AWSConfig: Enable IMDS IPv6 endpoint mode and dual-stack EC2 endpoint
    end
    CLI->>Binder: Bind(target IP)

    alt IPv4 target
        Binder->>EC2: DescribeAddresses(public IP)
        Binder->>IMDS: Get token and instance-id
        Binder->>EC2: DescribeNetworkInterfaces(primary ENI filters)
        alt Target already on primary ENI
            Binder-->>CLI: AlreadyAssociated result
        else Target is elsewhere
            Binder->>EC2: AssociateAddress(allocation, primary ENI, allow reassociation)
            Binder-->>CLI: Association result
        end
    else IPv6 target
        Binder->>IMDS: Get token and instance-id
        Binder->>EC2: DescribeNetworkInterfaces(primary ENI filters)
        alt Target already on primary ENI
            Binder-->>CLI: AlreadyAssociated result
        else Target must move or be assigned
            Binder->>EC2: DescribeSubnets(primary ENI subnet)
            Binder->>Binder: Verify IPv6 is inside subnet CIDR
            Binder->>EC2: DescribeNetworkInterfaces(IPv6 filter)
            opt IPv6 is on another ENI
                Binder->>EC2: UnassignIpv6Addresses(previous ENI)
            end
            Binder->>EC2: AssignIpv6Addresses(primary ENI)
            Binder-->>CLI: Assignment result
        end
    end
    CLI-->>User: Log success or already-associated status
```

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
        "ec2:UnassignIpv6Addresses"
      ],
      "Resource": "*"
    }
  ]
}
```

`ec2:DescribeNetworkInterfaces` should be validated against the
[AWS Service Authorization Reference](https://docs.aws.amazon.com/service-authorization/latest/reference/list_amazonec2.html)
or the Terraform-backed E2E test in a real AWS account.

## Testing

Run unit tests with:

```sh
go test ./...
```

The unit test path stays inside the Go process by using fakes for AWS and IMDS
interfaces, so it does not need AWS credentials or networked EC2 endpoints.

```mermaid
sequenceDiagram
    autonumber
    actor Dev
    participant GoTest as go test ./...
    participant ConfigTests as config/metadata tests
    participant BinderTests as binder tests
    participant Fakes as fake AWS and IMDS clients

    Dev->>GoTest: Run unit test command
    GoTest->>ConfigTests: Validate argument and environment resolution
    GoTest->>BinderTests: Exercise IPv4 and IPv6 binding cases
    BinderTests->>Fakes: Return deterministic EC2 and IMDS responses
    Fakes-->>BinderTests: Simulated addresses, ENIs, subnets, metadata
    BinderTests-->>GoTest: Assert API calls and bind results
    ConfigTests-->>GoTest: Assert config errors and normalized IPs
    GoTest-->>Dev: Test result without real AWS access
```

Check the Terraform E2E harness with:

```sh
terraform -chdir=test/e2e/terraform fmt -recursive -check
terraform -chdir=test/e2e/terraform init
terraform -chdir=test/e2e/terraform validate
```

Run the Terraform-backed AWS E2E suite with:

```sh
AWS_REGION=us-east-1 scripts/e2e-terraform.sh
```

```mermaid
sequenceDiagram
    autonumber
    actor Dev
    participant Script as scripts/e2e-terraform.sh
    participant GoBuild as go build
    participant Terraform as terraform test
    participant AWS as AWS account
    participant S3 as Artifact bucket
    participant SSM as SSM association
    participant EC2Instance as Runner EC2 instance
    participant CLI as aws-eip-binding

    Dev->>Script: Set AWS_REGION and run script
    Script->>GoBuild: Build linux/amd64 binary into .e2e/
    Script->>Terraform: init and test with binary path
    Terraform->>AWS: Create VPC, subnet, IAM, EIP, ENIs, endpoints, runner
    Terraform->>S3: Upload binary artifact
    Terraform->>SSM: Attach AWS-RunShellScript association
    SSM->>EC2Instance: Run generated e2e.sh
    EC2Instance->>S3: Download binary and verify checksum
    EC2Instance->>EC2Instance: Verify IMDSv2 instance identity
    EC2Instance->>AWS: Pre-associate IPv4 EIP to previous-owner ENI
    EC2Instance->>CLI: Bind IPv4 target
    CLI->>AWS: Move EIP to runner primary ENI
    EC2Instance->>AWS: Assert IPv4 target is on primary ENI
    alt IPv6 scenario enabled
        EC2Instance->>AWS: Pre-assign IPv6 target to previous-owner ENI
        EC2Instance->>CLI: Bind IPv6 target
        CLI->>AWS: Unassign from previous ENI and assign to primary ENI
        EC2Instance->>AWS: Assert IPv6 is on primary ENI and absent from previous ENI
    else IPv6 disabled
        EC2Instance->>EC2Instance: Skip IPv6 scenario
    end
    EC2Instance->>AWS: Cleanup test IPv4 and IPv6 associations
    SSM-->>Terraform: Report command status and S3 output location
    Terraform-->>Script: Destroy test infrastructure after test
    Script-->>Dev: Exit with E2E result
```

The E2E harness builds a Linux amd64 binary, uploads it to a temporary S3
bucket, creates a disposable VPC, an Amazon Linux 2023 EC2 instance, and a
standalone ENI used as the previous address owner. SSM runs the CLI inside the
instance after pre-associating the target EIP and, when enabled, pre-assigning
the target IPv6 address to that previous-owner ENI. The test validates real
IMDSv2, the instance IAM role, automatic IPv4 EIP reassociation, and
IPv6 unassign-and-move behavior. Set `E2E_ENABLE_IPV6=false` to run only the
IPv4 scenario.

SSM command output is written to the temporary artifact bucket under
`ssm-output/<name-prefix>/`, which is useful for failed-run debugging before
Terraform destroys the test bucket.

Only run E2E tests in a disposable AWS account or isolated region. Terraform
creates real infrastructure that can incur short-lived EC2, EIP, S3, and VPC
endpoint charges. `terraform test` attempts to destroy test infrastructure when
it finishes, but you should still monitor cleanup and remove leftover resources
manually if a run is interrupted.

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
