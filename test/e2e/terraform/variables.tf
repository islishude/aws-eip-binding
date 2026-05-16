variable "aws_region" {
  description = "AWS region for the disposable E2E environment."
  type        = string

  validation {
    condition     = length(var.aws_region) > 0
    error_message = "aws_region must not be empty."
  }
}

variable "binary_path" {
  description = "Path to the Linux amd64 aws-eip-binding binary to upload and execute."
  type        = string

  validation {
    condition     = fileexists(var.binary_path)
    error_message = "binary_path must point to an existing file."
  }
}

variable "name_prefix" {
  description = "Short unique prefix for all E2E resources. Use lowercase letters, numbers, and hyphens."
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$", var.name_prefix))
    error_message = "name_prefix must match ^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$."
  }
}

variable "enable_ipv6" {
  description = "Whether to run the IPv6 binding scenario in addition to IPv4."
  type        = bool
  default     = true
}

variable "instance_type" {
  description = "EC2 instance type for the E2E runner. Must be x86_64 compatible."
  type        = string
  default     = "t3.micro"
}

variable "ssm_timeout_seconds" {
  description = "How long Terraform waits for the SSM association to report Success."
  type        = number
  default     = 900
}

variable "tags" {
  description = "Additional tags to apply to E2E resources."
  type        = map(string)
  default     = {}
}
