locals {
  common_tags = merge(var.tags, {
    Project   = "aws-eip-binding"
    TestSuite = "terraform-e2e"
  })

  bucket_name       = trim(substr("${var.name_prefix}-${data.aws_caller_identity.current.account_id}-${var.aws_region}", 0, 63), "-")
  artifact_key      = "artifacts/${var.name_prefix}/aws-eip-binding-linux-amd64"
  ssm_output_prefix = "ssm-output/${var.name_prefix}"
  target_ipv6       = var.enable_ipv6 ? cidrhost(aws_subnet.public.ipv6_cidr_block, 100) : ""

  e2e_script = templatefile("${path.module}/templates/e2e.sh.tftpl", {
    artifact_bucket            = aws_s3_bucket.artifacts.bucket
    artifact_key               = aws_s3_object.binary.key
    artifact_md5               = filemd5(var.binary_path)
    aws_region                 = var.aws_region
    enable_ipv6                = var.enable_ipv6
    instance_id                = aws_instance.runner.id
    primary_network_interface  = aws_instance.runner.primary_network_interface_id
    previous_network_interface = aws_network_interface.previous_owner.id
    target_ipv4                = aws_eip.target.public_ip
    target_ipv4_allocation_id  = aws_eip.target.allocation_id
    target_ipv6                = local.target_ipv6
  })
}
