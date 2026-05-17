provider "aws" {
  region = var.aws_region

  default_tags {
    tags = local.common_tags
  }
}

data "aws_caller_identity" "current" {}

data "aws_partition" "current" {}

data "aws_ssm_parameter" "al2023_ami" {
  name = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64"
}

resource "aws_vpc" "main" {
  cidr_block                       = "10.42.0.0/16"
  assign_generated_ipv6_cidr_block = var.enable_ipv6
  enable_dns_hostnames             = true
  enable_dns_support               = true

  tags = {
    Name = "${var.name_prefix}-vpc"
  }
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id

  tags = {
    Name = "${var.name_prefix}-igw"
  }
}

resource "aws_subnet" "public" {
  vpc_id                          = aws_vpc.main.id
  cidr_block                      = "10.42.1.0/24"
  ipv6_cidr_block                 = var.enable_ipv6 ? cidrsubnet(aws_vpc.main.ipv6_cidr_block, 8, 1) : null
  assign_ipv6_address_on_creation = var.enable_ipv6
  map_public_ip_on_launch         = true

  tags = {
    Name = "${var.name_prefix}-public"
  }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  tags = {
    Name = "${var.name_prefix}-public"
  }
}

resource "aws_route" "public_ipv4" {
  route_table_id         = aws_route_table.public.id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.main.id
}

resource "aws_route" "public_ipv6" {
  count = var.enable_ipv6 ? 1 : 0

  route_table_id              = aws_route_table.public.id
  destination_ipv6_cidr_block = "::/0"
  gateway_id                  = aws_internet_gateway.main.id
}

resource "aws_route_table_association" "public" {
  subnet_id      = aws_subnet.public.id
  route_table_id = aws_route_table.public.id
}

resource "aws_security_group" "runner" {
  name        = "${var.name_prefix}-runner"
  description = "E2E runner outbound access only"
  vpc_id      = aws_vpc.main.id

  egress {
    description = "Allow outbound IPv4"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  dynamic "egress" {
    for_each = var.enable_ipv6 ? [1] : []
    content {
      description      = "Allow outbound IPv6"
      from_port        = 0
      to_port          = 0
      protocol         = "-1"
      ipv6_cidr_blocks = ["::/0"]
    }
  }

  tags = {
    Name = "${var.name_prefix}-runner"
  }
}

resource "aws_security_group" "endpoints" {
  name        = "${var.name_prefix}-endpoints"
  description = "Allow the E2E runner to reach private SSM endpoints"
  vpc_id      = aws_vpc.main.id

  ingress {
    description     = "HTTPS from E2E runner"
    from_port       = 443
    to_port         = 443
    protocol        = "tcp"
    security_groups = [aws_security_group.runner.id]
  }

  egress {
    description = "Allow endpoint return traffic"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${var.name_prefix}-endpoints"
  }
}

resource "aws_network_interface" "previous_owner" {
  subnet_id       = aws_subnet.public.id
  security_groups = [aws_security_group.runner.id]

  tags = {
    Name = "${var.name_prefix}-previous-owner"
  }
}

resource "aws_vpc_endpoint" "ssm" {
  for_each = toset(["ssm", "ssmmessages", "ec2messages"])

  vpc_id              = aws_vpc.main.id
  service_name        = "com.amazonaws.${var.aws_region}.${each.key}"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = [aws_subnet.public.id]
  security_group_ids  = [aws_security_group.endpoints.id]
  private_dns_enabled = true

  tags = {
    Name = "${var.name_prefix}-${each.key}"
  }
}

resource "aws_eip" "target" {
  domain = "vpc"

  depends_on = [
    aws_internet_gateway.main,
    aws_network_interface.previous_owner,
  ]

  tags = {
    Name = "${var.name_prefix}-target"
  }
}

resource "aws_instance" "runner" {
  ami                         = data.aws_ssm_parameter.al2023_ami.value
  instance_type               = var.instance_type
  iam_instance_profile        = aws_iam_instance_profile.runner.name
  subnet_id                   = aws_subnet.public.id
  vpc_security_group_ids      = [aws_security_group.runner.id]
  associate_public_ip_address = true
  ipv6_address_count          = var.enable_ipv6 ? 1 : null

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_protocol_ipv6          = var.enable_ipv6 ? "enabled" : "disabled"
    http_put_response_hop_limit = 1
    instance_metadata_tags      = "enabled"
  }

  user_data = <<-USER_DATA
    #!/bin/bash
    set -euxo pipefail
    systemctl enable --now amazon-ssm-agent || true
    if ! command -v aws >/dev/null 2>&1; then
      dnf install -y awscli || dnf install -y awscli-2 || true
    fi
  USER_DATA

  depends_on = [
    aws_route_table_association.public,
    aws_vpc_endpoint.ssm,
  ]

  tags = {
    Name = "${var.name_prefix}-runner"
  }
}
