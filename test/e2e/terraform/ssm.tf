resource "aws_ssm_association" "e2e" {
  name             = "AWS-RunShellScript"
  association_name = "${var.name_prefix}-e2e"

  targets {
    key    = "InstanceIds"
    values = [aws_instance.runner.id]
  }

  parameters = {
    commands = local.e2e_script
  }

  output_location {
    s3_bucket_name = aws_s3_bucket.artifacts.bucket
    s3_key_prefix  = local.ssm_output_prefix
    s3_region      = var.aws_region
  }

  wait_for_success_timeout_seconds = var.ssm_timeout_seconds

  depends_on = [
    aws_iam_role_policy.runner,
    aws_iam_role_policy_attachment.ssm,
    aws_network_interface.previous_owner,
    aws_s3_object.binary,
    aws_vpc_endpoint.ssm,
  ]
}
