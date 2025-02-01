# AWS EIP Binding CLI

This CLI tool associates an Elastic IP (EIP) to the current EC2 instance using AWS SDK for Go.

## Usage

1. Build the application:

   ```
   go build -o aws-eip-binding
   ```

2. Run the tool with your target EIP:

   ```
   ./aws-eip-binding <EIP>
   ```

### Using POD_NAME Argument

If you pass "POD_NAME" as the CLI argument, the program will:

1. Retrieve the environment variable POD_NAME.
2. Replace all hyphens (-) in its value with underscores (\_).
3. Use the resulting string as the key to fetch the actual IP from the environment.

For example, if your environment is configured as follows:

- POD_NAME=app-config
- app_config=192.168.1.100

Running:
aws-eip-binding POD_NAME
will set the target IP to 192.168.1.100.

## Required AWS IAM Policy Permissions

Ensure that the IAM role or user has permissions similar to the following:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ec2:AssociateAddress",
        "ec2:DescribeAddresses",
        "ec2:DescribeNetworkInterfaces",
        "ec2:DescribeTags",
        "ec2:DisassociateAddress"
      ],
      "Resource": "*"
    }
  ]
}
```
