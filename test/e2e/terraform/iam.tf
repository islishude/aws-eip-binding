resource "aws_iam_role" "runner" {
  name = "${var.name_prefix}-runner"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "ec2.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })

  tags = {
    Name = "${var.name_prefix}-runner"
  }
}

resource "aws_iam_role_policy_attachment" "ssm" {
  role       = aws_iam_role.runner.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_role_policy" "runner" {
  name = "${var.name_prefix}-runner"
  role = aws_iam_role.runner.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "BindAddresses"
        Effect = "Allow"
        Action = [
          "ec2:AssociateAddress",
          "ec2:AssignIpv6Addresses",
          "ec2:DescribeAddresses",
          "ec2:DescribeNetworkInterfaces",
          "ec2:DescribeSubnets",
          "ec2:DisassociateAddress",
          "ec2:UnassignIpv6Addresses",
        ]
        Resource = "*"
      },
      {
        Sid    = "ReadArtifactBucket"
        Effect = "Allow"
        Action = [
          "s3:GetBucketLocation",
          "s3:ListBucket",
        ]
        Resource = aws_s3_bucket.artifacts.arn
      },
      {
        Sid      = "ReadArtifactObject"
        Effect   = "Allow"
        Action   = "s3:GetObject"
        Resource = "${aws_s3_bucket.artifacts.arn}/${aws_s3_object.binary.key}"
      },
      {
        Sid      = "WriteSSMOutput"
        Effect   = "Allow"
        Action   = "s3:PutObject"
        Resource = "${aws_s3_bucket.artifacts.arn}/${local.ssm_output_prefix}/*"
      }
    ]
  })
}

resource "aws_iam_instance_profile" "runner" {
  name = "${var.name_prefix}-runner"
  role = aws_iam_role.runner.name
}
