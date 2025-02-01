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
2. Replace all hyphens (-) in its value with underscores (\_), it's due to hyphens cannot be used in environment variable names.
3. Use the resulting string as the key to fetch the actual IP from the environment.

For example, if your environment is configured as follows:

- POD_NAME=app-config
- app_config=54.162.153.80

Running:
aws-eip-binding POD_NAME
will set the target IP to 54.162.153.80.

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
