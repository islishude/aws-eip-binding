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
